package qloo

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

func TestInsightsSendsKeyAndParams(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if got := r.Header.Get("X-Api-Key"); got != "k" {
			t.Errorf("X-Api-Key = %q", got)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		if got := r.URL.Query().Get("signal.location.query"); got != "Logan Square" {
			t.Errorf("signal.location.query = %q", got)
		}
		w.Write([]byte(`{"success":true,"results":{"entities":[{"entity_id":"e1","name":"A","popularity":0.5,"query":{"affinity":0.9}}]}}`))
	}))
	defer srv.Close()

	c := New("k", WithBaseURL(srv.URL))
	q := url.Values{"filter.type": {TypeArtist}, "signal.location.query": {"Logan Square"}}
	res, err := c.Insights(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entities) != 1 || res.Entities[0].Name != "A" {
		t.Fatalf("entities = %+v", res.Entities)
	}
	if aff, ok := res.Entities[0].Affinity(); !ok || aff != 0.9 {
		t.Fatalf("affinity = %v %v", aff, ok)
	}

	// Second identical call is served from cache.
	if _, err := c.Insights(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("calls = %d, want 1 (cached)", n)
	}
}

func TestInsightsRequiresFilterType(t *testing.T) {
	c := New("k", WithBaseURL("http://unused"))
	if _, err := c.Insights(context.Background(), url.Values{}); err == nil {
		t.Fatal("expected error for missing filter.type")
	}
}

func TestAPIErrorIsReturnedAndNotCached(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, `{"error":"bad key"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := New("bad", WithBaseURL(srv.URL))
	for i := 0; i < 2; i++ {
		_, err := c.Search(context.Background(), "Wilco", []string{TypeArtist}, 3)
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized {
			t.Fatalf("err = %v", err)
		}
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("calls = %d, errors must not be cached", n)
	}
}

func TestMockServesEveryEndpoint(t *testing.T) {
	c := NewMock()
	ctx := context.Background()
	if ents, err := c.Search(ctx, "anything", []string{TypeArtist}, 3); err != nil || len(ents) != 3 {
		t.Fatalf("search: %v %d", err, len(ents))
	}
	if tags, err := c.FindTags(ctx, "indie", nil, 5); err != nil || len(tags) == 0 {
		t.Fatalf("tags: %v", err)
	}
	for _, ft := range []string{TypeArtist, TypeHeatmap, TypeDemographics, TypeTag} {
		res, err := c.Insights(ctx, url.Values{"filter.type": {ft}, "signal.interests.entities": {"x"}})
		if err != nil {
			t.Fatalf("insights %s: %v", ft, err)
		}
		if len(res.Entities)+len(res.Heatmap)+len(res.Tags)+len(res.Demographics) == 0 {
			t.Fatalf("insights %s: empty", ft)
		}
	}
}

func TestEntityCoordinatesAcceptsKnownShapes(t *testing.T) {
	for name, e := range map[string]Entity{
		"location":   {Location: map[string]any{"lat": 41.9, "lon": -87.7}},
		"geocode":    {Properties: map[string]any{"geocode": map[string]any{"latitude": 41.9, "longitude": -87.7}}},
		"properties": {Properties: map[string]any{"latitude": 41.9, "longitude": -87.7}},
	} {
		if lat, lon, ok := e.Coordinates(); !ok || lat != 41.9 || lon != -87.7 {
			t.Errorf("%s: got %v %v %v", name, lat, lon, ok)
		}
	}
	if _, _, ok := (Entity{Properties: map[string]any{"address": "1 Main St"}}).Coordinates(); ok {
		t.Error("entity without coordinates reported ok")
	}
}

func TestMockGeoIsLocal(t *testing.T) {
	c := NewMock()
	ctx := context.Background()
	res, err := c.Insights(ctx, url.Values{
		"filter.type": {TypePlace}, "signal.interests.entities": {"x"},
		"filter.location": {"POINT(-122.384600 47.668700)"}, "take": {"8"},
	})
	if err != nil || len(res.Entities) != 8 {
		t.Fatalf("places: %v %d", err, len(res.Entities))
	}
	for _, e := range res.Entities {
		lat, lon, ok := e.Coordinates()
		if !ok || math.Abs(lat-47.6687) > 0.03 || math.Abs(lon+122.3846) > 0.03 {
			t.Fatalf("%s at %v,%v (ok=%v), want near Ballard", e.Name, lat, lon, ok)
		}
	}

	res, err = c.Insights(ctx, url.Values{"filter.type": {TypeHeatmap}, "signal.interests.entities": {"x"}, "filter.location.query": {"Seattle"}})
	if err != nil || len(res.Heatmap) != 81 {
		t.Fatalf("heatmap: %v %d", err, len(res.Heatmap))
	}
	for _, p := range res.Heatmap {
		if math.Abs(p.Location.Latitude-47.63) > 0.11 || p.Query.Affinity < 0 || p.Query.Affinity > 1 {
			t.Fatalf("cell %+v outside Seattle or affinity out of range", p)
		}
	}
}
