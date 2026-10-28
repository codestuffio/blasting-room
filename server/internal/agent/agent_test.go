package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"blastingroom/internal/qloo"
)

// fakeClaude replays scripted Messages API responses and records requests.
type fakeClaude struct {
	mu       sync.Mutex
	replies  []string
	requests []map[string]any
}

func (f *fakeClaude) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req map[string]any
	json.Unmarshal(body, &req)
	f.mu.Lock()
	f.requests = append(f.requests, req)
	reply := f.replies[0]
	f.replies = f.replies[1:]
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, reply)
}

const memoInput = `{
  "verdict": "book",
  "summary": "Book Mock Artist 01 with Mock Artist 02.",
  "confidence": "low",
  "headliner": {"name": "Mock Artist 01", "entity_id": "mock-artist-01", "rationale": ["high local affinity"]},
  "support": {"name": "Mock Artist 02", "entity_id": "mock-artist-02", "rationale": ["overlapping fans"]},
  "local_fit": "strong", "momentum": "rising",
  "audience": {"summary": "25-29", "age_skew": "25_to_29 +0.34", "ticket_tier": "standard"},
  "co_promo": [{"kind": "podcast", "name": "Mock Podcast 03", "idea": "ad read"}],
  "deal": {"guarantee": 1400, "backend_pct": 80, "support_fee": 300, "ticket_price_advance": 20, "ticket_price_door": 25,
    "expected_tickets": 300, "terms": "$1,400 plus 80%", "walk_away": "$1,600", "rationale": "comparable tickets"},
  "routing": {"fit": "on_the_way", "summary": "between two nearby dates"},
  "risks": ["synthetic data"],
  "alternates": [],
  "evidence": [
    {"claim": "high local affinity", "tool": "discover_artists", "ref": "q1"},
    {"claim": "fans cluster nearby", "tool": "local_heatmap", "ref": "Q2"},
    {"claim": "wrong tool for the ref", "tool": "momentum", "ref": "Q1"},
    {"claim": "ref that names no call", "tool": "discover_artists", "ref": "Q9"}
  ]
}`

func reply(stop string, content ...string) string {
	b, _ := json.Marshal(map[string]any{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
		"stop_reason": stop, "content": json.RawMessage("[" + join(content) + "]"),
		"usage": map[string]any{"input_tokens": 10, "output_tokens": 10},
	})
	return string(b)
}

func toolUse(id, name, input string) string {
	return `{"type":"tool_use","id":"` + id + `","name":"` + name + `","input":` + input + `}`
}

func join(parts []string) string {
	s := ""
	for i, p := range parts {
		if i > 0 {
			s += ","
		}
		s += p
	}
	return s
}

func TestRunLoopsThroughToolsToMemo(t *testing.T) {
	fake := &fakeClaude{replies: []string{
		reply("tool_use",
			`{"type":"text","text":"Looking for local acts."}`,
			toolUse("t1", "discover_artists", `{"seed_ids":["mock-artist-01"]}`),
			toolUse("t2", "local_heatmap", `{"entity_id":"mock-artist-01"}`),
		),
		reply("tool_use", toolUse("t3", memoToolName, memoInput)),
	}}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	claude := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test"))
	a := New(claude, qloo.NewMock(), Config{})

	var mu sync.Mutex
	var events []Event
	memo, err := a.Run(context.Background(), Request{VenueID: "lantern", Mode: "fill_date"}, func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	if memo.Headliner.EntityID != "mock-artist-01" || memo.Audience.TicketTier != "standard" || memo.Verdict != "book" {
		t.Fatalf("memo = %+v", memo)
	}
	// The server prices the deal from the terms and the venue's costs.
	if m := memo.Deal.Math; m == nil || m.Capacity != 450 || m.BreakEvenTickets != 175 || len(m.Scenarios) != 4 {
		t.Fatalf("deal math = %+v", memo.Deal.Math)
	}
	// Only refs that name a call of the cited tool survive.
	var gotRefs []string
	for _, ev := range memo.Evidence {
		gotRefs = append(gotRefs, ev.Ref)
	}
	if strings.Join(gotRefs, ",") != "Q1,Q2,," {
		t.Fatalf("evidence refs = %q", gotRefs)
	}

	counts := map[string]int{}
	callRefs := map[string]string{}
	for _, e := range events {
		counts[e.Type]++
		if c, ok := e.Data.(ToolCall); ok {
			callRefs[c.ID] = c.Ref
		}
	}
	if callRefs["t1"] != "Q1" || callRefs["t2"] != "Q2" {
		t.Fatalf("call refs = %v", callRefs)
	}
	if counts["tool_call"] != 2 || counts["tool_result"] != 2 || counts["memo"] != 1 || counts["done"] != 1 {
		t.Fatalf("event counts = %v", counts)
	}

	// Second request must carry both tool results in one user message, in call order.
	if len(fake.requests) != 2 {
		t.Fatalf("requests = %d", len(fake.requests))
	}
	msgs := fake.requests[1]["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)
	blocks := last["content"].([]any)
	if last["role"] != "user" || len(blocks) != 2 {
		t.Fatalf("last message = %v", last)
	}
	for i, id := range []string{"t1", "t2"} {
		b := blocks[i].(map[string]any)
		if b["type"] != "tool_result" || b["tool_use_id"] != id || b["is_error"] == true {
			t.Fatalf("block %d = %v", i, b)
		}
		// The model reads each result's ref so it can cite the call.
		text := b["content"].([]any)[0].(map[string]any)["text"].(string)
		if want := fmt.Sprintf("ref Q%d\n", i+1); !strings.HasPrefix(text, want) || !json.Valid([]byte(text[len(want):])) {
			t.Fatalf("block %d = %q, want %q then JSON", i, text[:min(len(text), 40)], want)
		}
	}
	first := fake.requests[0]
	if first["model"] != "claude-opus-5-5" || first["fallbacks"] != "default" {
		t.Fatalf("model/fallbacks = %v / %v", first["model"], first["fallbacks"])
	}
	if th := first["thinking"].(map[string]any); th["type"] != "adaptive" || th["display"] != "updates" {
		t.Fatalf("thinking = %v", th)
	}
	if oc := first["output_config"].(map[string]any); oc["effort"] != "medium" {
		t.Fatalf("output_config = %v", oc)
	}
	var memoTool map[string]any
	for _, tl := range first["tools"].([]any) {
		if m := tl.(map[string]any); m["name"] == memoToolName {
			memoTool = m
		}
	}
	schema := memoTool["input_schema"].(map[string]any)
	if memoTool["strict"] != true || schema["additionalProperties"] != false || len(schema["required"].([]any)) != len(memoProperties) {
		t.Fatalf("memo tool = %v", memoTool)
	}
}

func TestRunRejectsUnknownVenue(t *testing.T) {
	a := New(anthropic.NewClient(option.WithAPIKey("x")), qloo.NewMock(), Config{})
	if _, err := a.Run(context.Background(), Request{VenueID: "nope"}, func(Event) {}); err == nil {
		t.Fatal("expected error")
	}
}

func TestToolErrorsAreReportedNotFatal(t *testing.T) {
	fake := &fakeClaude{replies: []string{
		reply("tool_use", toolUse("t1", "local_heatmap", `{}`)),
		reply("tool_use", toolUse("t2", memoToolName, memoInput)),
	}}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	a := New(anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test")), qloo.NewMock(), Config{})
	if _, err := a.Run(context.Background(), Request{VenueID: "harbor"}, func(Event) {}); err != nil {
		t.Fatal(err)
	}
	msgs := fake.requests[1]["messages"].([]any)
	block := msgs[len(msgs)-1].(map[string]any)["content"].([]any)[0].(map[string]any)
	if block["is_error"] != true {
		t.Fatalf("expected is_error tool_result, got %v", block)
	}
}

func TestMapToolsEmitGeoButKeepModelResultsCompact(t *testing.T) {
	fake := &fakeClaude{replies: []string{
		reply("tool_use",
			toolUse("h1", "local_heatmap", `{"entity_id":"mock-artist-01"}`),
			toolUse("p1", "co_promo_partners", `{"entity_ids":["mock-artist-01"],"kind":"place"}`),
			toolUse("p2", "co_promo_partners", `{"entity_ids":["mock-artist-01"],"kind":"podcast"}`),
		),
		reply("tool_use", toolUse("m1", memoToolName, memoInput)),
	}}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	a := New(anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test")), qloo.NewMock(), Config{})
	var mu sync.Mutex
	geos := map[string]*Geo{}
	_, err := a.Run(context.Background(), Request{VenueID: "harbor"}, func(e Event) {
		if e.Type == "geo" {
			mu.Lock()
			g := e.Data.(*Geo)
			geos[g.ToolCallID] = g
			mu.Unlock()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	// Podcasts have no coordinates, so only the heatmap and the place call draw on the map.
	if len(geos) != 2 || geos["h1"] == nil || geos["p1"] == nil {
		t.Fatalf("geo events = %v", geos)
	}
	venue, _ := FindVenue("harbor")
	heat, places := geos["h1"], geos["p1"]
	if heat.Kind != "heatmap" || heat.EntityIDs[0] != "mock-artist-01" || len(heat.Cells) != 81 || heat.Venue.Lat != venue.Lat {
		t.Fatalf("heatmap geo = %+v", heat)
	}
	if places.Kind != "partners" || len(places.Places) == 0 || places.Venue.Name != venue.Name {
		t.Fatalf("partners geo = %+v", places)
	}
	for _, p := range places.Places {
		if p.Name == "" || math.Abs(p.Lat-venue.Lat) > 0.03 || math.Abs(p.Lon-venue.Lon) > 0.03 {
			t.Fatalf("place %+v not near the venue", p)
		}
	}

	// The model sees the compact summary, not every cell or coordinate.
	msgs := fake.requests[1]["messages"].([]any)
	blocks := msgs[len(msgs)-1].(map[string]any)["content"].([]any)
	for i, id := range []string{"h1", "p1", "p2"} {
		b := blocks[i].(map[string]any)
		content := b["content"].([]any)[0].(map[string]any)["text"].(string)
		if b["tool_use_id"] != id || b["is_error"] == true {
			t.Fatalf("block %d = %v", i, b)
		}
		if strings.Contains(content, "tool_call_id") || strings.Contains(content, `"lon"`) && id != "h1" {
			t.Fatalf("%s result leaked map data: %s", id, content)
		}
	}
	heatResult := blocks[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(heatResult, "venue_percentile") || strings.Count(heatResult, `"lat"`) > 5 {
		t.Fatalf("heatmap result is not the compact summary: %s", heatResult)
	}
}

func TestMemoWithoutHeadlinerIsAPass(t *testing.T) {
	m := Memo{Verdict: "book"}
	checkMemo(&m, nil)
	if m.Verdict != "pass" {
		t.Fatalf("verdict = %q", m.Verdict)
	}
	m = Memo{Verdict: "book_with_changes", Headliner: Pick{Name: "Mock Artist 01"}}
	checkMemo(&m, nil)
	if m.Verdict != "book_with_changes" {
		t.Fatalf("verdict = %q", m.Verdict)
	}
}
