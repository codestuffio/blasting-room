package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
  "summary": "Book Mock Artist 01 with Mock Artist 02.",
  "confidence": "low",
  "headliner": {"name": "Mock Artist 01", "entity_id": "mock-artist-01", "rationale": ["high local affinity"]},
  "support": {"name": "Mock Artist 02", "entity_id": "mock-artist-02", "rationale": ["overlapping fans"]},
  "local_fit": "strong", "momentum": "rising",
  "audience": {"summary": "25-29", "age_skew": "25_to_29 +0.34", "ticket_tier": "standard"},
  "co_promo": [{"kind": "podcast", "name": "Mock Podcast 03", "idea": "ad read"}],
  "risks": ["synthetic data"],
  "alternates": [],
  "evidence": [{"claim": "high local affinity", "tool": "discover_artists"}]
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
			toolUse("t1", "discover_artists", `{"trend_bias":"high"}`),
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
	if memo.Headliner.EntityID != "mock-artist-01" || memo.Audience.TicketTier != "standard" {
		t.Fatalf("memo = %+v", memo)
	}

	counts := map[string]int{}
	for _, e := range events {
		counts[e.Type]++
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
