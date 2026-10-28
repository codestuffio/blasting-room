// Package ticketmaster is a small client for Ticketmaster's Discovery API,
// used to find shows that compete with a booking for the same crowd. It
// only reads, and nothing it fetches is stored.
package ticketmaster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const DefaultBaseURL = "https://app.ticketmaster.com/discovery/v2"

type Client struct {
	key       string
	baseURL   string
	http      *http.Client
	retryWait time.Duration

	// The API allows 5 requests a second and the agent runs tools in
	// parallel, so requests are spaced out. next is the earliest free slot.
	gap  time.Duration
	mu   sync.Mutex
	next time.Time

	cache *cache
}

type Option func(*Client)

func WithBaseURL(u string) Option          { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithRetryWait sets the pause before retrying a rate-limited call.
func WithRetryWait(d time.Duration) Option { return func(c *Client) { c.retryWait = d } }

// WithRequestGap sets the minimum spacing between requests.
func WithRequestGap(d time.Duration) Option { return func(c *Client) { c.gap = d } }

// New returns a client for the Discovery API. Only the consumer key is
// needed; the consumer secret is for OAuth flows this app doesn't use.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		key: apiKey, baseURL: DefaultBaseURL, http: &http.Client{Timeout: 15 * time.Second},
		retryWait: time.Second, gap: 220 * time.Millisecond, cache: newCache(30*time.Minute, 256),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Search is a music-show search around a point over a range of local dates.
type Search struct {
	Lat, Lon    float64
	RadiusMiles int
	From, To    string // local dates, YYYY-MM-DD, inclusive
}

// Show is one ticketed music event, trimmed to what a talent buyer weighs.
type Show struct {
	ID            string
	Name          string
	Date          string // local date
	Time          string // local time, HH:MM:SS, empty when TBA
	Status        string // onsale, offsale, cancelled, postponed, rescheduled
	Genre         string
	Subgenre      string
	Acts          []string
	VenueID       string
	Venue         string
	Address       string
	City          string
	Region        string // state or province code
	Country       string // ISO country code
	Lat, Lon      float64
	DistanceMiles float64
	PriceMin      float64
	PriceMax      float64
	Currency      string
}

// Results is a page of shows plus how many matched in all.
type Results struct {
	Shows []Show
	Total int
}

// MusicShows lists ticketed music events within the radius, by date.
func (c *Client) MusicShows(ctx context.Context, s Search) (*Results, error) {
	if _, err := time.Parse(time.DateOnly, s.From); err != nil {
		return nil, fmt.Errorf("from must be YYYY-MM-DD: %q", s.From)
	}
	if _, err := time.Parse(time.DateOnly, s.To); err != nil {
		return nil, fmt.Errorf("to must be YYYY-MM-DD: %q", s.To)
	}
	q := url.Values{
		"geoPoint":           {Geohash(s.Lat, s.Lon, 9)},
		"radius":             {strconv.Itoa(s.RadiusMiles)},
		"unit":               {"miles"},
		"segmentName":        {"Music"},
		"localStartDateTime": {s.From + "T00:00:00," + s.To + "T23:59:59"},
		"sort":               {"date,asc"},
		"size":               {"100"},
	}
	body, err := c.getRetry(ctx, "/events.json", q)
	if err != nil {
		return nil, err
	}
	return parseEvents(body)
}

// Attraction is a Ticketmaster artist listing.
type Attraction struct {
	ID       string
	Name     string
	Upcoming int // upcoming events Ticketmaster counts for the act
}

// FindArtist returns the music attraction whose name matches exactly,
// ignoring case, or nil. Keyword search is loose: "Wednesday" also finds
// "Wednesday 13".
func (c *Client) FindArtist(ctx context.Context, name string) (*Attraction, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("artist name is required")
	}
	body, err := c.getRetry(ctx, "/attractions.json", url.Values{"keyword": {name}, "segmentName": {"Music"}, "size": {"20"}})
	if err != nil {
		return nil, err
	}
	var r struct {
		Embedded struct {
			Attractions []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Test     bool   `json:"test"`
				Upcoming struct {
					Total int `json:"_total"`
				} `json:"upcomingEvents"`
			} `json:"attractions"`
		} `json:"_embedded"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("ticketmaster: decoding attractions: %w", err)
	}
	for _, a := range r.Embedded.Attractions {
		if !a.Test && strings.EqualFold(strings.TrimSpace(a.Name), name) {
			return &Attraction{ID: a.ID, Name: a.Name, Upcoming: a.Upcoming.Total}, nil
		}
	}
	return nil, nil
}

// ArtistShows lists an act's upcoming events, by date, with the same show
// listed twice (a second listing for the same night and city) dropped.
func (c *Client) ArtistShows(ctx context.Context, attractionID string) (*Results, error) {
	body, err := c.getRetry(ctx, "/events.json", url.Values{
		"attractionId": {attractionID}, "sort": {"date,asc"}, "size": {"100"}, "locale": {"*"},
	})
	if err != nil {
		return nil, err
	}
	res, err := parseEvents(body)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	kept := res.Shows[:0]
	for _, s := range res.Shows {
		k := s.Date + "|" + strings.ToLower(s.City)
		if !seen[k] {
			seen[k] = true
			kept = append(kept, s)
		}
	}
	res.Shows = kept
	return res, nil
}

// getRetry is get with one retry after a rate limit.
func (c *Client) getRetry(ctx context.Context, path string, q url.Values) ([]byte, error) {
	body, err := c.get(ctx, path, q)
	if errors.Is(err, errRateLimited) && ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(c.retryWait):
		}
		body, err = c.get(ctx, path, q)
	}
	return body, err
}

// wait blocks until this request's slot in the request schedule.
func (c *Client) wait(ctx context.Context) error {
	c.mu.Lock()
	now := time.Now()
	at := c.next
	if at.Before(now) {
		at = now
	}
	c.next = at.Add(c.gap)
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Until(at)):
		return nil
	}
}

var errRateLimited = errors.New("ticketmaster: rate limited")

// get never lets the API key into an error: net/http errors quote the
// request URL, and the key rides in the query string.
func (c *Client) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	key := path + "?" + q.Encode() // before the API key is added
	if body, ok := c.cache.get(key); ok {
		return body, nil
	}
	if err := c.wait(ctx); err != nil {
		return nil, fmt.Errorf("ticketmaster %s: %w", path, err)
	}
	q.Set("apikey", c.key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("ticketmaster %s: building request", path)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("ticketmaster %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("ticketmaster %s: reading body: %w", path, err)
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, errRateLimited
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("ticketmaster %s: HTTP %d: the API key was rejected", path, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("ticketmaster %s: HTTP %d", path, resp.StatusCode)
	}
	c.cache.set(key, body)
	return body, nil
}

type eventsResponse struct {
	Embedded struct {
		Events []event `json:"events"`
	} `json:"_embedded"`
	Page struct {
		TotalElements int `json:"totalElements"`
	} `json:"page"`
}

type event struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Test  bool   `json:"test"` // Ticketmaster's own test listings
	Dates struct {
		Start struct {
			LocalDate string `json:"localDate"`
			LocalTime string `json:"localTime"`
		} `json:"start"`
		Status struct {
			Code string `json:"code"`
		} `json:"status"`
	} `json:"dates"`
	Classifications []struct {
		Primary  bool `json:"primary"`
		Genre    name `json:"genre"`
		SubGenre name `json:"subGenre"`
	} `json:"classifications"`
	PriceRanges []struct {
		Min      float64 `json:"min"`
		Max      float64 `json:"max"`
		Currency string  `json:"currency"`
	} `json:"priceRanges"`
	Embedded struct {
		Venues []struct {
			ID       string  `json:"id"`
			Name     string  `json:"name"`
			Distance float64 `json:"distance"`
			Address  struct {
				Line1 string `json:"line1"`
			} `json:"address"`
			City  name `json:"city"`
			State struct {
				StateCode string `json:"stateCode"`
			} `json:"state"`
			Country struct {
				CountryCode string `json:"countryCode"`
			} `json:"country"`
			Location struct {
				Latitude  string `json:"latitude"`
				Longitude string `json:"longitude"`
			} `json:"location"`
		} `json:"venues"`
		Attractions []name `json:"attractions"`
	} `json:"_embedded"`
}

type name struct {
	Name string `json:"name"`
}

func parseEvents(body []byte) (*Results, error) {
	var r eventsResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("ticketmaster: decoding events: %w", err)
	}
	out := &Results{Total: r.Page.TotalElements, Shows: make([]Show, 0, len(r.Embedded.Events))}
	for _, e := range r.Embedded.Events {
		if e.Test {
			continue
		}
		s := Show{ID: e.ID, Name: e.Name, Date: e.Dates.Start.LocalDate, Time: e.Dates.Start.LocalTime, Status: e.Dates.Status.Code}
		for _, c := range e.Classifications {
			if c.Primary || s.Genre == "" {
				s.Genre, s.Subgenre = undefinedToEmpty(c.Genre.Name), undefinedToEmpty(c.SubGenre.Name)
			}
		}
		for _, a := range e.Embedded.Attractions {
			s.Acts = append(s.Acts, a.Name)
		}
		if len(e.Embedded.Venues) > 0 {
			v := e.Embedded.Venues[0]
			s.VenueID, s.Venue, s.Address, s.DistanceMiles = v.ID, v.Name, v.Address.Line1, v.Distance
			s.City, s.Region, s.Country = v.City.Name, v.State.StateCode, v.Country.CountryCode
			s.Lat, _ = strconv.ParseFloat(v.Location.Latitude, 64)
			s.Lon, _ = strconv.ParseFloat(v.Location.Longitude, 64)
		}
		if len(e.PriceRanges) > 0 {
			p := e.PriceRanges[0]
			s.PriceMin, s.PriceMax, s.Currency = p.Min, p.Max, p.Currency
		}
		out.Shows = append(out.Shows, s)
	}
	sort.SliceStable(out.Shows, func(i, j int) bool { return out.Shows[i].Date < out.Shows[j].Date })
	return out, nil
}

// Ticketmaster labels a missing genre "Undefined".
func undefinedToEmpty(s string) string {
	if strings.EqualFold(s, "undefined") {
		return ""
	}
	return s
}

// Geohash encodes a point at the given precision, as geoPoint expects.
func Geohash(lat, lon float64, precision int) string {
	const base32 = "0123456789bcdefghjkmnpqrstuvwxyz"
	latR, lonR := [2]float64{-90, 90}, [2]float64{-180, 180}
	var b strings.Builder
	bit, ch, even := 0, 0, true
	for b.Len() < precision {
		r, v := &latR, lat
		if even {
			r, v = &lonR, lon
		}
		mid := (r[0] + r[1]) / 2
		if v >= mid {
			ch |= 1 << (4 - bit)
			r[0] = mid
		} else {
			r[1] = mid
		}
		even = !even
		if bit++; bit == 5 {
			b.WriteByte(base32[ch])
			bit, ch = 0, 0
		}
	}
	return b.String()
}
