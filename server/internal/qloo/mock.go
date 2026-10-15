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
	"strings"
	"time"
)

// mockTransport answers Qloo API paths with deterministic synthetic data.
// Names are obviously fake ("Mock Artist 07") so mock output is never
// mistaken for real Qloo data in a demo.
type mockTransport struct{}

func (mockTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	q := r.URL.Query()
	var body any
	switch r.URL.Path {
	case "/search":
		body = map[string]any{"results": mockEntities(q.Get("types"), q.Get("query"), atoi(q.Get("take"), 5))}
	case "/v2/tags":
		body = map[string]any{"results": map[string]any{"tags": mockTags(q.Get("filter.query"))}}
	case "/v2/insights":
		body = map[string]any{"results": mockInsights(q)}
	case "/v2/trending":
		body = map[string]any{"results": map[string]any{"trending": mockTrend(q.Get("signal.interests.entities"))}}
	case "/v2/analysis/compare":
		body = map[string]any{"results": map[string]any{
			"note": "mock compare output",
			"tags": mockTags(q.Get("a.signal.interests.entities") + q.Get("b.signal.interests.entities")),
		}}
	default:
		return jsonResponse(r, http.StatusNotFound, map[string]any{"error": "no mock for " + r.URL.Path}), nil
	}
	return jsonResponse(r, http.StatusOK, body), nil
}

func jsonResponse(r *http.Request, status int, v any) *http.Response {
	b, _ := json.Marshal(v)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(b)),
		Request:    r,
	}
}

var mockGenres = []string{"indie rock", "post-punk", "dream pop", "alt-country", "garage rock", "synthpop", "shoegaze", "emo"}

func mockEntities(entityType, seed string, n int) []map[string]any {
	if entityType == "" {
		entityType = TypeArtist
	}
	label := map[string]string{
		TypeArtist: "Mock Artist", TypePlace: "Mock Bar", TypePodcast: "Mock Podcast", TypeBrand: "Mock Brand",
	}[entityType]
	if label == "" {
		label = "Mock Entity"
	}
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

func mockInsights(q url.Values) map[string]any {
	seed := q.Encode()
	switch t := q.Get("filter.type"); t {
	case TypeHeatmap:
		h := hash(seed)
		pts := []map[string]any{}
		for i := 0; i < 12; i++ {
			k := (h + uint32(i)*104729) % 1000
			pts = append(pts, map[string]any{
				"location": map[string]any{"latitude": 41.92 + float64(k%40)/1000, "longitude": -87.70 + float64(k%55)/1000, "geohash": fmt.Sprintf("dp3wm%02d", i)},
				"query":    map[string]any{"affinity": float64(k%100) / 100, "affinity_rank": float64(i + 1), "popularity": float64(k%80) / 100},
			})
		}
		return map[string]any{"heatmap": pts}
	case TypeDemographics:
		return map[string]any{"demographics": []map[string]any{{
			"entity_id": q.Get("signal.interests.entities"),
			"query": map[string]any{
				"age":    map[string]float64{"24_and_younger": 0.21, "25_to_29": 0.34, "30_to_34": 0.12, "35_to_44": -0.08, "45_to_54": -0.27, "55_and_older": -0.41},
				"gender": map[string]float64{"male": 0.04, "female": -0.04},
			},
		}}}
	case TypeTag:
		return map[string]any{"tags": mockTags(seed)}
	default:
		return map[string]any{"entities": mockEntities(t, seed, atoi(q.Get("take"), 10))}
	}
}

func mockTrend(seed string) []map[string]any {
	h := hash(seed)
	out := []map[string]any{}
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 8; i++ {
		out = append(out, map[string]any{
			"date":                  day.AddDate(0, 0, -7*i).Format("2006-01-02"),
			"population_percentile": 0.6 + float64((h+uint32(i))%30)/100,
			"velocity_fold_change":  1 + float64((h>>uint(i))%40)/100,
		})
	}
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
