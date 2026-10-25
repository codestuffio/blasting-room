package qloo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// MockScenario picks how much data the mock serves, so the agent's critic and
// the UI's degraded paths can be exercised before a live key arrives.
type MockScenario string

const (
	MockFull     MockScenario = "full"     // every call returns data
	MockSparse   MockScenario = "sparse"   // thin coverage: short lists, and many artists with no heatmap, trend or demographics
	MockDegraded MockScenario = "degraded" // the heatmap is down, some calls fail with 503, some are rate-limited once
	MockEmpty    MockScenario = "empty"    // every call succeeds with nothing in it, like Qloo's silent invalid-parameter failures
)

var MockScenarios = []MockScenario{MockFull, MockSparse, MockDegraded, MockEmpty}

func ParseMockScenario(s string) (MockScenario, error) {
	if s == "" {
		return MockFull, nil
	}
	for _, m := range MockScenarios {
		if string(m) == strings.ToLower(strings.TrimSpace(s)) {
			return m, nil
		}
	}
	return "", fmt.Errorf("unknown mock scenario %q (want one of %v)", s, MockScenarios)
}

// mockTransport answers Qloo API paths with deterministic synthetic data.
// Names are obviously fake ("Mock Artist 07") so mock output is never
// mistaken for real Qloo data in a demo.
type mockTransport struct {
	scenario MockScenario
	mu       sync.Mutex
	seen     map[string]bool // URLs already answered, for once-only failures
}

func (m *mockTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	q := r.URL.Query()
	if resp := m.failure(r); resp != nil {
		return resp, nil
	}
	if m.scenario == MockEmpty {
		return jsonResponse(r, http.StatusOK, emptyBody(r.URL.Path, q)), nil
	}
	sparse := m.scenario == MockSparse
	var body any
	switch r.URL.Path {
	case "/search":
		n := atoi(q.Get("take"), 5)
		if sparse {
			n = min(n, 2)
		}
		body = map[string]any{"results": mockEntities(q.Get("types"), q.Get("query"), n)}
	case "/v2/tags":
		body = map[string]any{"results": map[string]any{"tags": mockTags(q.Get("filter.query"))}}
	case "/v2/insights":
		body = map[string]any{"results": mockInsights(q, sparse)}
	case "/v2/trending":
		if strings.Contains(q.Get("signal.interests.entities"), ",") {
			return jsonResponse(r, http.StatusBadRequest, map[string]any{"errors": []map[string]any{{"message": "signal.interests.entities.ids must contain only one non-empty string"}}}), nil
		}
		trend := []map[string]any{}
		if id := q.Get("signal.interests.entities"); !(sparse && thin(id)) {
			trend = mockTrend(id, q.Get("filter.start_date"), q.Get("filter.end_date"))
		}
		body = map[string]any{"results": map[string]any{"trending": trend}}
	case "/v2/analysis/compare":
		body = map[string]any{"results": mockCompare(q.Get("a.signal.interests.entities"), q.Get("b.signal.interests.entities"))}
	default:
		return jsonResponse(r, http.StatusNotFound, map[string]any{"error": "no mock for " + r.URL.Path}), nil
	}
	return jsonResponse(r, http.StatusOK, body), nil
}

// failure returns the degraded scenario's error responses: the heatmap is
// down for good, one URL in five always times out upstream, and another one
// in five is rate-limited on its first request only (the client's retry
// absorbs it).
func (m *mockTransport) failure(r *http.Request) *http.Response {
	if m.scenario != MockDegraded {
		return nil
	}
	if r.URL.Query().Get("filter.type") == TypeHeatmap {
		return jsonResponse(r, http.StatusServiceUnavailable, map[string]any{"error": "mock: heatmap service unavailable"})
	}
	u := r.URL.String()
	switch hash(u) % 5 {
	case 0:
		return jsonResponse(r, http.StatusServiceUnavailable, map[string]any{"error": "mock: upstream timeout"})
	case 1:
		m.mu.Lock()
		first := !m.seen[u]
		m.seen[u] = true
		m.mu.Unlock()
		if first {
			return jsonResponse(r, http.StatusTooManyRequests, map[string]any{"error": "mock: rate limited"})
		}
	}
	return nil
}

// emptyBody is each endpoint's success shape with nothing in it.
func emptyBody(path string, q url.Values) any {
	none := []any{}
	switch path {
	case "/search":
		return map[string]any{"results": none}
	case "/v2/tags":
		return map[string]any{"results": map[string]any{"tags": none}}
	case "/v2/trending":
		return map[string]any{"results": map[string]any{"trending": none}}
	case "/v2/analysis/compare":
		return map[string]any{"results": map[string]any{}}
	}
	switch q.Get("filter.type") {
	case TypeHeatmap:
		return map[string]any{"results": map[string]any{"heatmap": none}}
	case TypeDemographics:
		return map[string]any{"results": map[string]any{"demographics": none}}
	}
	return map[string]any{"results": map[string]any{"entities": none}}
}

// thin marks about 40% of entities as having no data in the sparse scenario.
func thin(entityIDs string) bool { return hash(entityIDs)%5 < 2 }

func jsonResponse(r *http.Request, status int, v any) *http.Response {
	b, _ := json.Marshal(v)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(b)),
		Request:    r,
	}
}

var mockLabels = map[string]string{
	TypeArtist: "Mock Artist", TypePlace: "Mock Bar", TypePodcast: "Mock Podcast", TypeBrand: "Mock Brand",
}

func mockLabel(entityType string) string {
	if l, ok := mockLabels[entityType]; ok {
		return l
	}
	return "Mock Entity"
}

var mockGenres = []string{"indie rock", "post-punk", "dream pop", "alt-country", "garage rock", "synthpop", "shoegaze", "emo"}

func mockEntities(entityType, seed string, n int) []map[string]any {
	if entityType == "" {
		entityType = TypeArtist
	}
	label := mockLabel(entityType)
	if n <= 0 || n > 50 {
		n = 10
	}
	h := hash(seed)
	out := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		k := (h + uint32(i)*7919) % 97
		pop := 0.35 + float64(k%60)/100
		aff := math.Round((0.95-float64(i)*0.04)*1000) / 1000
		genre := mockGenres[int(k)%len(mockGenres)]
		out = append(out, map[string]any{
			"entity_id":  fmt.Sprintf("mock-%s-%02d", strings.TrimPrefix(entityType, "urn:entity:"), k),
			"name":       fmt.Sprintf("%s %02d", label, k),
			"type":       entityType,
			"popularity": pop,
			"tags":       []map[string]any{{"id": "urn:tag:genre:music:" + strings.ReplaceAll(genre, " ", "_"), "name": genre, "type": "urn:tag:genre"}},
			"query":      map[string]any{"affinity": aff},
		})
	}
	return out
}

func mockTags(seed string) []map[string]any {
	h := hash(seed)
	out := []map[string]any{}
	for i := 0; i < 5; i++ {
		g := mockGenres[(int(h)+i)%len(mockGenres)]
		out = append(out, map[string]any{
			"tag_id": "urn:tag:genre:music:" + strings.ReplaceAll(g, " ", "_"),
			"name":   g,
			"type":   "urn:tag:genre",
		})
	}
	return out
}

// mockCompare mirrors the live compare shape: shared tags with a score that
// repeat both groups' entities, tags only one group has, and shared entities.
func mockCompare(a, b string) map[string]any {
	group := func(ids string) []map[string]any {
		out := []map[string]any{}
		for _, id := range strings.Split(ids, ",") {
			out = append(out, map[string]any{"entity_id": id, "name": mockName(id), "subtype": TypeArtist})
		}
		return out
	}
	shared := mockTags(a + b)
	for i, t := range shared {
		t["subtype"] = "urn:tag:genre:music"
		t["query"] = map[string]any{"score": math.Round((0.6+float64(i)*0.08)*1000) / 1000, "a.signal.interests.entities": group(a), "b.signal.interests.entities": group(b)}
	}
	return map[string]any{"tags": shared, "a": mockTags(a), "b": mockTags(b), "matchEntities": []any{}}
}

func mockInsights(q url.Values, sparse bool) map[string]any {
	seed := q.Encode()
	signal := q.Get("signal.interests.entities")
	switch t := q.Get("filter.type"); t {
	case TypeHeatmap:
		if sparse && thin(signal) {
			return map[string]any{"heatmap": []any{}}
		}
		return map[string]any{"heatmap": mockHeatmap(q.Get("filter.location.query"), signal)}
	case TypeDemographics:
		if sparse && thin(signal) {
			return map[string]any{"demographics": []any{}}
		}
		return map[string]any{"demographics": []map[string]any{{
			"entity_id": signal,
			"query": map[string]any{
				"age":    map[string]float64{"24_and_younger": 0.21, "25_to_29": 0.34, "30_to_34": 0.12, "35_to_44": -0.08, "45_to_54": -0.27, "55_and_older": -0.41},
				"gender": map[string]float64{"male": 0.04, "female": -0.04},
			},
		}}}
	case TypeTag:
		return map[string]any{"tags": mockTags(seed)}
	default:
		if ids := q.Get("filter.results.entities"); ids != "" {
			return map[string]any{"entities": mockRanked(strings.Split(ids, ","), sparse)}
		}
		n := atoi(q.Get("take"), 10)
		if sparse {
			if t == TypeBrand {
				return map[string]any{"entities": []any{}}
			}
			n = min(n, 3)
		}
		ents := mockEntities(t, seed, n)
		if t == TypePlace {
			placeAround(ents, q.Get("filter.location"))
		}
		return map[string]any{"entities": ents}
	}
}

// mockRanked scores the requested entities, as filter.results.entities does.
// In the sparse scenario thin entities come back unscored, so they drop out.
func mockRanked(ids []string, sparse bool) []map[string]any {
	out := []map[string]any{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || sparse && thin(id) {
			continue
		}
		k := hash(id)
		out = append(out, map[string]any{
			"entity_id":  id,
			"name":       mockName(id),
			"type":       TypeArtist,
			"popularity": 0.35 + float64(k%60)/100,
			"query": map[string]any{
				"affinity":       math.Round((0.5+float64(k%45)/100)*1000) / 1000,
				"explainability": map[string]any{"signal.location": map[string]any{"affinity": math.Round(float64(k%90)/100*1000) / 1000}},
			},
		})
	}
	sort.Slice(out, func(a, b int) bool {
		return out[a]["query"].(map[string]any)["affinity"].(float64) > out[b]["query"].(map[string]any)["affinity"].(float64)
	})
	return out
}

// mockName turns "mock-artist-07" back into "Mock Artist 07".
func mockName(id string) string {
	parts := strings.Split(id, "-")
	if len(parts) != 3 || parts[0] != "mock" {
		return id
	}
	return mockLabel("urn:entity:"+parts[1]) + " " + parts[2]
}

// mockCities centres the synthetic heatmap on the demo venues' cities.
var mockCities = map[string][2]float64{
	"chicago": {41.895, -87.68},
	"austin":  {30.267, -97.743},
	"seattle": {47.63, -122.35},
}

// mockHeatmap returns a 9x9 grid of cells around the city with one smooth
// hotspot whose position depends on the artist, so different artists draw
// visibly different maps.
func mockHeatmap(city, entity string) []map[string]any {
	c, ok := mockCities[strings.ToLower(strings.TrimSpace(city))]
	if !ok {
		c = mockCities["chicago"]
	}
	h := hash(entity)
	hotLat := c[0] + float64(int(h%121)-60)/1000
	hotLon := c[1] + float64(int((h/121)%121)-60)/1000
	pts := []map[string]any{}
	for i := 0; i < 9; i++ {
		for j := 0; j < 9; j++ {
			lat := c[0] + float64(i-4)*0.025
			lon := c[1] + float64(j-4)*0.025
			d2 := (math.Pow(lat-hotLat, 2) + math.Pow(lon-hotLon, 2)) / 0.003
			noise := float64(hash(fmt.Sprint(entity, i, j))%100) / 100
			aff := math.Round((0.8*math.Exp(-d2)+0.2*noise)*1000) / 1000
			pts = append(pts, map[string]any{
				"location": map[string]any{"latitude": lat, "longitude": lon, "geohash": fmt.Sprintf("mock%d%d", i, j)},
				"query":    map[string]any{"affinity": aff, "popularity": math.Round(noise*800) / 1000},
			})
		}
	}
	sort.Slice(pts, func(a, b int) bool {
		return pts[a]["query"].(map[string]any)["affinity"].(float64) > pts[b]["query"].(map[string]any)["affinity"].(float64)
	})
	for i, p := range pts {
		p["query"].(map[string]any)["affinity_rank"] = float64(i + 1)
	}
	return pts
}

// placeAround gives mock places coordinates within about 2 km of the
// filter.location point ("POINT(lon lat)"), in Qloo's top-level location shape.
func placeAround(ents []map[string]any, point string) {
	var lon, lat float64
	if _, err := fmt.Sscanf(point, "POINT(%f %f)", &lon, &lat); err != nil {
		return
	}
	for i, e := range ents {
		k := hash(e["entity_id"].(string))
		e["location"] = map[string]any{
			"lat": lat + float64(int(k%41)-20)/1000,
			"lon": lon + float64(int((k/41)%41)-20)/1000,
		}
		e["properties"] = map[string]any{"address": fmt.Sprintf("%d Mock Street", 100+i*10)}
	}
}

// mockTrend returns a weekly percentile series between start and end (dates
// as YYYY-MM-DD) that rises, holds or falls depending on the entity, newest
// first so the parser's sort is exercised.
func mockTrend(entity, start, end string) []map[string]any {
	to, err := time.Parse("2006-01-02", end)
	if err != nil {
		to = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	}
	from, err := time.Parse("2006-01-02", start)
	if err != nil || !from.Before(to) {
		from = to.AddDate(0, -6, 0)
	}
	h := hash(entity)
	slope := []float64{0.012, 0.001, -0.009}[h%3] // per week: rising, flat, falling
	base := 0.5 + float64(h%25)/100
	weeks := int(to.Sub(from).Hours()/(24*7)) + 1
	out := []map[string]any{}
	prev := 0.0
	for i := 0; i < weeks; i++ {
		noise := (float64(hash(fmt.Sprint(entity, i))%100)/100 - 0.5) * 0.03
		p := math.Max(0.05, math.Min(0.99, base+slope*float64(i)+noise))
		vel := 1.0
		if prev > 0 {
			vel = p / prev
		}
		prev = p
		out = append(out, map[string]any{
			"date":                  from.AddDate(0, 0, 7*i).Format("2006-01-02"),
			"population_percentile": math.Round(p*1000) / 1000,
			"velocity_fold_change":  math.Round(vel*1000) / 1000,
		})
	}
	slices.Reverse(out)
	return out
}

func hash(s string) uint32 {
	f := fnv.New32a()
	f.Write([]byte(s))
	return f.Sum32()
}

func atoi(s string, def int) int {
	var n int
	if _, err := fmt.Sscan(s, &n); err != nil {
		return def
	}
	return n
}
