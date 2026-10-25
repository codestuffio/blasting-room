// Package qloo is a small client for the Qloo Insights API (hackathon host).
//
// Every endpoint is a GET with query-string parameters and the key in the
// X-Api-Key header. Responses are cached in memory; Qloo's hackathon terms
// allow private server-side caching but not publishing the data.
package qloo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultBaseURL = "https://hackathon.api.qloo.com"

type Client struct {
	baseURL   string
	apiKey    string
	http      *http.Client
	cache     *ttlCache
	retryWait time.Duration
	scenario  MockScenario // empty for the live API
}

type Option func(*Client)

func WithBaseURL(u string) Option          { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }
func WithCacheTTL(d time.Duration) Option  { return func(c *Client) { c.cache = newTTLCache(d) } }

// WithRetryWait sets the pause before the one retry of a transient failure.
func WithRetryWait(d time.Duration) Option { return func(c *Client) { c.retryWait = d } }

// New returns a client for the real API.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		baseURL:   DefaultBaseURL,
		apiKey:    apiKey,
		http:      &http.Client{Timeout: 20 * time.Second},
		cache:     newTTLCache(6 * time.Hour),
		retryWait: 500 * time.Millisecond,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// NewMock returns a client whose transport serves synthetic, Qloo-shaped data.
// It lets the whole app run before a hackathon key is issued.
func NewMock() *Client { return NewMockScenario(MockFull) }

// NewMockScenario is NewMock with thin, failing or empty data instead.
func NewMockScenario(s MockScenario) *Client {
	c := New("mock-key", WithBaseURL("https://mock.qloo.local"),
		WithHTTPClient(&http.Client{Transport: &mockTransport{scenario: s, seen: map[string]bool{}}}),
		WithRetryWait(50*time.Millisecond))
	c.scenario = s
	return c
}

func (c *Client) IsMock() bool { return c.scenario != "" }

// MockScenario is the mock's scenario, or empty for the live API.
func (c *Client) MockScenario() MockScenario { return c.scenario }

// APIError is a non-2xx response from Qloo.
type APIError struct {
	Status int
	Path   string
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("qloo %s: HTTP %d: %s", e.Path, e.Status, truncate(e.Body, 300))
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	u := c.baseURL + path
	if enc := q.Encode(); enc != "" {
		u += "?" + enc
	}
	if body, ok := c.cache.get(u); ok {
		return json.Unmarshal(body, out)
	}

	body, err := c.fetch(ctx, path, u)
	if err != nil && transient(err) && ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return err
		case <-time.After(c.retryWait):
		}
		body, err = c.fetch(ctx, path, u)
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("qloo %s: decoding: %w", path, err)
	}
	c.cache.set(u, body)
	return nil
}

func (c *Client) fetch(ctx context.Context, path, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qloo %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("qloo %s: reading body: %w", path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &APIError{Status: resp.StatusCode, Path: path, Body: string(body)}
	}
	return body, nil
}

// transient reports whether a failed request is worth one retry: rate
// limiting, a gateway error, or no response at all.
func transient(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Status {
		case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		}
		return false
	}
	return true
}

// Raw calls any GET path and returns the whole response body undecoded. The
// probe command uses it to inspect response shapes the typed methods assume.
func (c *Client) Raw(ctx context.Context, path string, q url.Values) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.get(ctx, path, q, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Search resolves a free-text name to entities (GET /search).
func (c *Client) Search(ctx context.Context, query string, types []string, take int) ([]Entity, error) {
	q := url.Values{"query": {query}}
	if len(types) > 0 {
		q.Set("types", strings.Join(types, ","))
	}
	if take > 0 {
		q.Set("take", fmt.Sprint(take))
	}
	var out struct {
		Results []Entity `json:"results"`
	}
	if err := c.get(ctx, "/search", q, &out); err != nil {
		return nil, err
	}
	return out.Results, nil
}

// FindTags resolves free text to tag URNs (GET /v2/tags).
func (c *Client) FindTags(ctx context.Context, query string, tagTypes []string, take int) ([]Tag, error) {
	q := url.Values{"filter.query": {query}, "feature.typo_tolerance": {"true"}}
	if len(tagTypes) > 0 {
		q.Set("filter.tag.types", strings.Join(tagTypes, ","))
	}
	if take > 0 {
		q.Set("take", fmt.Sprint(take))
	}
	var out struct {
		Results struct {
			Tags []Tag `json:"tags"`
		} `json:"results"`
	}
	if err := c.get(ctx, "/v2/tags", q, &out); err != nil {
		return nil, err
	}
	return out.Results.Tags, nil
}

// Insights calls GET /v2/insights. The filter.type parameter decides which
// field of the result is populated (entities, heatmap, demographics or tags).
func (c *Client) Insights(ctx context.Context, q url.Values) (*InsightsResults, error) {
	if q.Get("filter.type") == "" {
		return nil, fmt.Errorf("qloo insights: filter.type is required")
	}
	var out struct {
		Results InsightsResults `json:"results"`
	}
	if err := c.get(ctx, "/v2/insights", q, &out); err != nil {
		return nil, err
	}
	return &out.Results, nil
}

// Trending calls GET /v2/trending for popularity over time.
func (c *Client) Trending(ctx context.Context, entityType string, entityIDs []string, start, end time.Time) (json.RawMessage, error) {
	q := url.Values{
		"filter.type":       {entityType},
		"filter.start_date": {start.Format("2006-01-02")},
		"filter.end_date":   {end.Format("2006-01-02")},
	}
	if len(entityIDs) > 0 {
		q.Set("signal.interests.entities", strings.Join(entityIDs, ","))
	}
	var out struct {
		Results json.RawMessage `json:"results"`
	}
	if err := c.get(ctx, "/v2/trending", q, &out); err != nil {
		return nil, err
	}
	return out.Results, nil
}

// Compare calls GET /v2/analysis/compare for two groups of entities.
func (c *Client) Compare(ctx context.Context, groupA, groupB []string, extra url.Values) (json.RawMessage, error) {
	q := url.Values{
		"a.signal.interests.entities": {strings.Join(groupA, ",")},
		"b.signal.interests.entities": {strings.Join(groupB, ",")},
	}
	for k, v := range extra {
		q[k] = v
	}
	var out struct {
		Results json.RawMessage `json:"results"`
	}
	if err := c.get(ctx, "/v2/analysis/compare", q, &out); err != nil {
		return nil, err
	}
	return out.Results, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
