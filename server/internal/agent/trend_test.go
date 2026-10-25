package agent

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"blastingroom/internal/qloo"
)

func TestParseTrendAcceptsKnownShapes(t *testing.T) {
	for name, raw := range map[string]string{
		"bare array":   `[{"date":"2026-08-08","population_percentile":0.7},{"date":"2026-08-01","population_percentile":0.6,"velocity_fold_change":1.1}]`,
		"object field": `{"trending":[{"date":"2026-08-01","percentile":0.6,"velocity":1.1},{"date":"2026-08-08","percentile":0.7}]}`,
	} {
		pts := parseTrend(json.RawMessage(raw))
		if len(pts) != 2 || pts[0].Date != "2026-08-01" || pts[1].Percentile != 0.7 {
			t.Fatalf("%s: %+v", name, pts)
		}
		if pts[0].Velocity == nil || *pts[0].Velocity != 1.1 || pts[1].Velocity != nil {
			t.Fatalf("%s: velocity %+v", name, pts)
		}
	}
	for _, raw := range []string{`{}`, `[]`, `{"trending":[{"when":"x","score":3}]}`, `"nope"`} {
		if pts := parseTrend(json.RawMessage(raw)); len(pts) != 0 {
			t.Fatalf("%s: got %+v, want none", raw, pts)
		}
	}
}

func TestTrendSummaryDirection(t *testing.T) {
	pts := func(a, b float64) []TrendPoint {
		return []TrendPoint{{Date: "2026-08-01", Percentile: a}, {Date: "2026-08-08", Percentile: (a + b) / 2}, {Date: "2026-08-15", Percentile: b}}
	}
	for want, p := range map[string][]TrendPoint{"rising": pts(0.6, 0.7), "flat": pts(0.6, 0.61), "falling": pts(0.7, 0.6)} {
		if got := trendSummary("a", p)["direction"]; got != want {
			t.Errorf("%v: direction %v, want %s", p, got, want)
		}
	}
	if s := trendSummary("a", nil); s["note"] == nil {
		t.Errorf("empty series summary = %v, want a note", s)
	}
}

func TestMomentumEmitsTrendAndSummarizesForTheModel(t *testing.T) {
	fake := &fakeClaude{replies: []string{
		reply("tool_use", toolUse("m1", "momentum", `{"entity_ids":["mock-artist-01","mock-artist-02"]}`)),
		reply("tool_use", toolUse("s1", memoToolName, memoInput)),
	}}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	a := New(anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test")), qloo.NewMock(), Config{})
	var mu sync.Mutex
	var trends []*Trend
	if _, err := a.Run(context.Background(), Request{VenueID: "lantern"}, func(e Event) {
		if e.Type == "trend" {
			mu.Lock()
			trends = append(trends, e.Data.(*Trend))
			mu.Unlock()
		}
	}); err != nil {
		t.Fatal(err)
	}
	if len(trends) != 1 || trends[0].ToolCallID != "m1" || len(trends[0].Series) != 2 {
		t.Fatalf("trend events = %+v", trends)
	}
	for i, s := range trends[0].Series {
		if s.EntityID != []string{"mock-artist-01", "mock-artist-02"}[i] || len(s.Points) < 12 {
			t.Fatalf("series %d = %+v", i, s)
		}
		for j := 1; j < len(s.Points); j++ {
			if s.Points[j].Date <= s.Points[j-1].Date {
				t.Fatalf("series %d not sorted oldest first", i)
			}
		}
	}

	msgs := fake.requests[1]["messages"].([]any)
	block := msgs[len(msgs)-1].(map[string]any)["content"].([]any)[0].(map[string]any)
	content := block["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(content, `"direction"`) || strings.Contains(content, "population_percentile") || strings.Contains(content, "tool_call_id") {
		t.Fatalf("model got %s, want the compact summary", content)
	}
}

func TestEmptyResultsCarryAHint(t *testing.T) {
	fake := &fakeClaude{replies: []string{
		reply("tool_use",
			toolUse("d1", "discover_artists", `{"trend_bias":"high"}`),
			toolUse("m1", "momentum", `{"entity_ids":["mock-artist-01"]}`),
			toolUse("h1", "local_heatmap", `{"entity_id":"mock-artist-01"}`),
		),
		reply("tool_use", toolUse("s1", memoToolName, memoInput)),
	}}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	a := New(anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test")), qloo.NewMockScenario(qloo.MockEmpty), Config{})
	var side int
	if _, err := a.Run(context.Background(), Request{VenueID: "copperline"}, func(e Event) {
		if e.Type == "trend" || e.Type == "geo" {
			side++
		}
	}); err != nil {
		t.Fatal(err)
	}
	if side != 0 {
		t.Fatalf("%d trend/geo events for empty data, want none", side)
	}
	msgs := fake.requests[1]["messages"].([]any)
	blocks := msgs[len(msgs)-1].(map[string]any)["content"].([]any)
	for i, want := range []string{"popularity_max", "no trending data", "no heatmap data"} {
		b := blocks[i].(map[string]any)
		content := b["content"].([]any)[0].(map[string]any)["text"].(string)
		if b["is_error"] == true || !strings.Contains(content, want) {
			t.Fatalf("block %d = %s, want a note mentioning %q", i, content, want)
		}
	}
}
