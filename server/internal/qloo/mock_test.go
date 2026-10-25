package qloo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestParseMockScenario(t *testing.T) {
	for in, want := range map[string]MockScenario{"": MockFull, "sparse": MockSparse, " Degraded ": MockDegraded, "empty": MockEmpty} {
		if got, err := ParseMockScenario(in); err != nil || got != want {
			t.Errorf("%q: got %q, %v", in, got, err)
		}
	}
	if _, err := ParseMockScenario("chaos"); err == nil {
		t.Error("expected error for unknown scenario")
	}
	if NewMock().MockScenario() != MockFull || New("k").IsMock() {
		t.Error("NewMock must be the full scenario and New must be live")
	}
}

func TestMockSparseThinsCoverage(t *testing.T) {
	c := NewMockScenario(MockSparse)
	ctx := context.Background()
	if ents, _ := c.Search(ctx, "anything", []string{TypeArtist}, 5); len(ents) != 2 {
		t.Fatalf("search returned %d, want 2", len(ents))
	}
	if res, _ := c.Insights(ctx, url.Values{"filter.type": {TypeBrand}, "signal.interests.entities": {"x"}}); len(res.Entities) != 0 {
		t.Fatalf("brands = %d, want none", len(res.Entities))
	}

	// About 40% of artists have no heatmap, trend or score; the rest do.
	var thinIDs, fullIDs []string
	for i := 0; i < 40; i++ {
		id := fmt.Sprintf("mock-artist-%02d", i)
		res, err := c.Insights(ctx, url.Values{"filter.type": {TypeHeatmap}, "signal.interests.entities": {id}, "filter.location.query": {"Chicago"}})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := c.Trending(ctx, TypeArtist, []string{id}, time.Now().AddDate(0, 0, -90), time.Now())
		var tr struct{ Trending []json.RawMessage }
		json.Unmarshal(raw, &tr)
		if (len(res.Heatmap) == 0) != (len(tr.Trending) == 0) {
			t.Fatalf("%s: heatmap and trend coverage disagree", id)
		}
		if len(res.Heatmap) == 0 {
			thinIDs = append(thinIDs, id)
		} else {
			fullIDs = append(fullIDs, id)
		}
	}
	if len(thinIDs) < 8 || len(fullIDs) < 8 {
		t.Fatalf("thin %d / full %d, want a mix", len(thinIDs), len(fullIDs))
	}

	res, err := c.Insights(ctx, url.Values{"filter.type": {TypeArtist}, "filter.results.entities": {thinIDs[0] + "," + fullIDs[0]}})
	if err != nil || len(res.Entities) != 1 || res.Entities[0].EntityID != fullIDs[0] || res.Entities[0].Name == fullIDs[0] {
		t.Fatalf("scored shortlist = %+v, %v; want only %s, with a name", res.Entities, err, fullIDs[0])
	}
}

func TestMockDegradedFailsSomeCalls(t *testing.T) {
	c := NewMockScenario(MockDegraded)
	ctx := context.Background()
	_, err := c.Insights(ctx, url.Values{"filter.type": {TypeHeatmap}, "signal.interests.entities": {"x"}})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusServiceUnavailable {
		t.Fatalf("heatmap err = %v, want 503", err)
	}
	ok, failed := 0, 0
	for i := 0; i < 30; i++ {
		if _, err := c.Search(ctx, fmt.Sprint("artist ", i), []string{TypeArtist}, 3); err != nil {
			failed++
		} else {
			ok++
		}
	}
	// One URL in five fails for good; the once-only 429s are absorbed by the retry.
	if failed == 0 || ok < 15 {
		t.Fatalf("ok %d / failed %d, want mostly ok with some failures", ok, failed)
	}
}

func TestMockEmptyReturnsNothing(t *testing.T) {
	c := NewMockScenario(MockEmpty)
	ctx := context.Background()
	if ents, err := c.Search(ctx, "anything", nil, 5); err != nil || len(ents) != 0 {
		t.Fatalf("search: %v %d", err, len(ents))
	}
	if tags, err := c.FindTags(ctx, "indie", nil, 5); err != nil || len(tags) != 0 {
		t.Fatalf("tags: %v %d", err, len(tags))
	}
	for _, ft := range []string{TypeArtist, TypePlace, TypeHeatmap, TypeDemographics} {
		res, err := c.Insights(ctx, url.Values{"filter.type": {ft}, "signal.interests.entities": {"x"}})
		if err != nil || len(res.Entities)+len(res.Heatmap) != 0 || string(res.Demographics) != "" && string(res.Demographics) != "[]" {
			t.Fatalf("insights %s: %v %+v", ft, err, res)
		}
	}
}

func TestMockTrendIsWeeklyOverTheRange(t *testing.T) {
	end := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	raw, err := NewMock().Trending(context.Background(), TypeArtist, []string{"mock-artist-07"}, end.AddDate(0, 0, -90), end)
	if err != nil {
		t.Fatal(err)
	}
	var tr struct {
		Trending []struct {
			Date       string  `json:"date"`
			Percentile float64 `json:"population_percentile"`
		}
	}
	if err := json.Unmarshal(raw, &tr); err != nil || len(tr.Trending) != 13 {
		t.Fatalf("trend = %s, %v", raw, err)
	}
	if tr.Trending[0].Date != "2026-10-18" || tr.Trending[12].Date != "2026-07-26" {
		t.Fatalf("dates %s..%s, want newest first from 2026-10-18 to 2026-07-26", tr.Trending[0].Date, tr.Trending[12].Date)
	}
	for _, p := range tr.Trending {
		if p.Percentile <= 0 || p.Percentile >= 1 {
			t.Fatalf("percentile %v out of range", p.Percentile)
		}
	}
}
