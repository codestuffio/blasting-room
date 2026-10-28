// Package agent runs the Blasting Room booking agent: a Claude tool-use loop
// over Qloo tools that ends with a structured booking memo.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"

	"blastingroom/internal/qloo"
	"blastingroom/internal/ticketmaster"
)

// ErrClaude wraps a failed Messages API call: a network error, an outage or
// a rejected key. Its details are for the server log, not the visitor.
var ErrClaude = errors.New("claude request failed")

type Config struct {
	Model    string
	Effort   string // low | medium | high | xhigh | max
	MaxTurns int
	// Shows looks up competing shows; nil when no Ticketmaster key is set.
	Shows *ticketmaster.Client
}

type Agent struct {
	claude anthropic.Client
	qloo   *qloo.Client
	cfg    Config
}

func New(claude anthropic.Client, q *qloo.Client, cfg Config) *Agent {
	if cfg.Model == "" {
		cfg.Model = "claude-opus-5-5"
	}
	if cfg.Effort == "" {
		cfg.Effort = "medium"
	}
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 20
	}
	return &Agent{claude: claude, qloo: q, cfg: cfg}
}

// Event is streamed to the UI as the agent works.
type Event struct {
	Type string `json:"type"` // start | progress | text | tool_call | tool_result | geo | trend | route | memo | error | done
	Data any    `json:"data,omitempty"`
}

type ToolCall struct {
	ID    string          `json:"id"`
	Ref   string          `json:"ref"` // Q1, Q2, … in call order; memo evidence cites it
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type ToolResult struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	Bytes      int    `json:"bytes"`
}

// Run executes one booking task, calling emit for every step. It returns the
// memo, or an error if the agent could not produce one.
func (a *Agent) Run(ctx context.Context, req Request, emit func(Event)) (*Memo, error) {
	venue, ok := FindVenue(req.VenueID)
	if !ok {
		return nil, fmt.Errorf("unknown venue %q", req.VenueID)
	}
	tools := toolset(a.qloo, a.cfg.Shows, venue)
	byName := make(map[string]tool, len(tools))
	for _, t := range tools {
		byName[t.name] = t
	}

	params := anthropic.BetaMessageNewParams{
		Model:     a.cfg.Model,
		MaxTokens: 16000,
		System: []anthropic.BetaTextBlockParam{{
			Text:         systemPrompt,
			CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
		}},
		Tools:        toolParams(tools),
		Thinking:     anthropic.BetaThinkingConfigParamUnion{OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{Display: anthropic.BetaThinkingConfigAdaptiveDisplayUpdates}},
		OutputConfig: anthropic.BetaOutputConfigParam{Effort: anthropic.BetaOutputConfigEffort(a.cfg.Effort)},
		// Server-side fallback re-serves a safety-classifier refusal on another model.
		Fallbacks: anthropic.BetaFallbacksParamUnion{OfDefault: constant.ValueOf[constant.Default]()},
		Betas: []anthropic.AnthropicBeta{
			anthropic.AnthropicBetaServerSideFallback2026_07_01,
			anthropic.AnthropicBetaThinkingDisplayUpdates2026_08_18,
		},
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(userPrompt(venue, req, a.qloo.IsMock()))),
		},
	}

	emit(Event{Type: "start", Data: map[string]any{"venue": venue, "model": a.cfg.Model, "qloo_mock": a.qloo.IsMock()}})
	refs := map[string]string{} // call ref → tool name, for checking memo evidence

	for turn := 1; turn <= a.cfg.MaxTurns; turn++ {
		resp, err := a.claude.Beta.Messages.New(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrClaude, err)
		}
		// History is append-only: the assistant turn goes back exactly as returned.
		params.Messages = append(params.Messages, resp.ToParam())

		var calls []anthropic.BetaToolUseBlock
		for _, block := range resp.Content {
			switch b := block.AsAny().(type) {
			case anthropic.BetaThinkingBlock:
				if b.Thinking != "" {
					emit(Event{Type: "progress", Data: b.Thinking})
				}
			case anthropic.BetaTextBlock:
				if b.Text != "" {
					emit(Event{Type: "text", Data: b.Text})
				}
			case anthropic.BetaToolUseBlock:
				calls = append(calls, b)
			}
		}

		switch resp.StopReason {
		case anthropic.BetaStopReasonToolUse:
		case anthropic.BetaStopReasonRefusal:
			return nil, errors.New("the model declined this request")
		case anthropic.BetaStopReasonMaxTokens:
			return nil, errors.New("the model hit its output limit before finishing")
		default:
			return nil, errors.New("the agent stopped without submitting a memo")
		}

		// The memo tool ends the run; any other calls in the same turn are moot.
		for _, c := range calls {
			if c.Name != memoToolName {
				continue
			}
			var memo Memo
			if err := json.Unmarshal([]byte(c.JSON.Input.Raw()), &memo); err != nil {
				return nil, fmt.Errorf("memo did not match schema: %w", err)
			}
			checkMemo(&memo, refs)
			memo.Deal.Math = dealMath(venue, memo.Deal)
			emit(Event{Type: "memo", Data: memo})
			emit(Event{Type: "done", Data: map[string]any{"turns": turn}})
			return &memo, nil
		}

		results := a.runTools(ctx, calls, byName, refs, emit)
		params.Messages = append(params.Messages, anthropic.NewBetaUserMessage(results...))
	}
	return nil, fmt.Errorf("no memo after %d turns", a.cfg.MaxTurns)
}

// runTools executes one turn's tool calls concurrently and returns their
// results in call order, all destined for a single user message. Each call
// gets the next ref in refs, and its result opens with a "ref Qn" line so
// the memo's evidence can cite the exact call.
func (a *Agent) runTools(ctx context.Context, calls []anthropic.BetaToolUseBlock, byName map[string]tool, refs map[string]string, emit func(Event)) []anthropic.BetaContentBlockParamUnion {
	results := make([]anthropic.BetaContentBlockParamUnion, len(calls))
	var wg sync.WaitGroup
	for i, c := range calls {
		raw := json.RawMessage(c.JSON.Input.Raw())
		ref := fmt.Sprintf("Q%d", len(refs)+1)
		refs[ref] = c.Name
		emit(Event{Type: "tool_call", Data: ToolCall{ID: c.ID, Ref: ref, Name: c.Name, Input: raw}})
		wg.Add(1)
		go func(i int, c anthropic.BetaToolUseBlock, ref string) {
			defer wg.Done()
			start := time.Now()
			out, err := a.exec(ctx, byName, c.Name, raw)
			res := ToolResult{ID: c.ID, Name: c.Name, OK: err == nil, DurationMS: time.Since(start).Milliseconds()}
			switch g := out.(type) {
			case withGeo:
				out = g.model
				if g.geo != nil {
					g.geo.ToolCallID = c.ID
					emit(Event{Type: "geo", Data: g.geo})
				}
			case withTrend:
				out = g.model
				if g.trend != nil {
					g.trend.ToolCallID = c.ID
					emit(Event{Type: "trend", Data: g.trend})
				}
			case withRoute:
				out = g.model
				if g.route != nil {
					g.route.ToolCallID = c.ID
					emit(Event{Type: "route", Data: g.route})
				}
			}
			var content string
			if err != nil {
				res.Error = err.Error()
				content = "error: " + err.Error()
			} else {
				b, _ := json.Marshal(out)
				content = string(b)
			}
			res.Bytes = len(content)
			emit(Event{Type: "tool_result", Data: res})
			results[i] = anthropic.NewBetaToolResultBlock(c.ID, "ref "+ref+"\n"+content, err != nil)
		}(i, c, ref)
	}
	wg.Wait()
	return results
}

func (a *Agent) exec(ctx context.Context, byName map[string]tool, name string, raw json.RawMessage) (any, error) {
	t, ok := byName[name]
	if !ok || t.run == nil {
		return nil, fmt.Errorf("unknown tool %q", name)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return t.run(ctx, raw)
}

func toolParams(tools []tool) []anthropic.BetaToolUnionParam {
	out := make([]anthropic.BetaToolUnionParam, 0, len(tools))
	for _, t := range tools {
		req := t.required
		if t.strict {
			req = make([]string, 0, len(t.properties))
			for k := range t.properties {
				req = append(req, k)
			}
			sort.Strings(req)
		}
		p := anthropic.BetaToolParam{
			Name:        t.name,
			Description: anthropic.String(t.description),
			InputSchema: anthropic.BetaToolInputSchemaParam{
				Properties:  t.properties,
				Required:    req,
				ExtraFields: map[string]any{"additionalProperties": false},
			},
		}
		if t.strict {
			p.Strict = anthropic.Bool(true)
		}
		out = append(out, anthropic.BetaToolUnionParam{OfTool: &p})
	}
	return out
}
