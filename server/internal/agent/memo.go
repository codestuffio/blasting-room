package agent

import (
	"sort"
	"strings"
)

// Memo is the booking memo the agent submits through the strict
// submit_booking_memo tool. Its JSON schema (memoSchema) must stay in sync.
type Memo struct {
	Verdict    string       `json:"verdict"`
	Summary    string       `json:"summary"`
	Confidence string       `json:"confidence"`
	Headliner  Pick         `json:"headliner"`
	Support    Pick         `json:"support"`
	LocalFit   string       `json:"local_fit"`
	Momentum   string       `json:"momentum"`
	Audience   Audience     `json:"audience"`
	CoPromo    []Partner    `json:"co_promo"`
	Risks      []string     `json:"risks"`
	Shows      []Clash      `json:"competing_shows"`
	Alternates []Alternate  `json:"alternates"`
	Evidence   []Provenance `json:"evidence"`
}

type Pick struct {
	Name      string   `json:"name"`
	EntityID  string   `json:"entity_id"`
	Rationale []string `json:"rationale"`
}

type Audience struct {
	Summary    string `json:"summary"`
	AgeSkew    string `json:"age_skew"`
	TicketTier string `json:"ticket_tier"`
}

type Partner struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	Idea string `json:"idea"`
}

// Clash is a nearby show the agent judged against the booking.
type Clash struct {
	Name   string `json:"name"`
	Venue  string `json:"venue"`
	Date   string `json:"date"`
	Threat string `json:"threat"`
	Note   string `json:"note"`
}

type Alternate struct {
	Name     string `json:"name"`
	EntityID string `json:"entity_id"`
	Reason   string `json:"reason"`
}

// Provenance ties a memo claim to the tool call that produced the evidence.
// Ref is the call's run-local ref ("Q3"), shown in the trace.
type Provenance struct {
	Claim string `json:"claim"`
	Tool  string `json:"tool"`
	Ref   string `json:"ref"`
}

// checkMemo holds the memo to what the run actually did. refs maps each call
// ref to its tool name: an evidence ref that names no call, or a call of a
// different tool, is cleared so the UI never links a claim to the wrong
// result. A memo with no headliner cannot recommend booking one.
func checkMemo(m *Memo, refs map[string]string) {
	for i := range m.Evidence {
		ev := &m.Evidence[i]
		ev.Ref = strings.ToUpper(strings.TrimSpace(ev.Ref))
		if tool, ok := refs[ev.Ref]; !ok || tool != ev.Tool {
			ev.Ref = ""
		}
	}
	if strings.TrimSpace(m.Headliner.Name) == "" {
		m.Verdict = "pass"
	}
}

func obj(props map[string]any) map[string]any {
	req := make([]string, 0, len(props))
	for k := range props {
		req = append(req, k)
	}
	sort.Strings(req) // deterministic tool schema keeps the prompt cache warm
	return map[string]any{"type": "object", "properties": props, "required": req, "additionalProperties": false}
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

func enum(desc string, vals ...string) map[string]any {
	return map[string]any{"type": "string", "enum": vals, "description": desc}
}

func arr(items map[string]any) map[string]any { return map[string]any{"type": "array", "items": items} }

var pickSchema = obj(map[string]any{
	"name":      str("Artist name exactly as returned by Qloo"),
	"entity_id": str("Qloo entity_id"),
	"rationale": arr(str("One evidence-backed reason")),
})

// memoProperties is the top-level property map of the memo tool's input schema.
var memoProperties = map[string]any{
	"verdict": enum("The call. Vetting an offer: book the offered act as pitched, book_with_changes (a different support, date or ticket tier), or pass. Filling a date: book, or pass when nothing survived your checks",
		"book", "book_with_changes", "pass"),
	"summary":    str("Two-sentence recommendation a talent buyer can act on"),
	"confidence": enum("Overall confidence given data coverage", "low", "medium", "high"),
	"headliner":  pickSchema,
	"support":    pickSchema,
	"local_fit":  str("How strongly this act's fan base over-indexes around the venue, citing heatmap or ranking numbers"),
	"momentum":   str("Trend direction and velocity, citing trending numbers"),
	"audience": obj(map[string]any{
		"summary":     str("Who is likely to buy tickets"),
		"age_skew":    str("Age buckets that over/under-index"),
		"ticket_tier": enum("Suggested pricing tier", "budget", "standard", "premium"),
	}),
	"co_promo": arr(obj(map[string]any{
		"kind": enum("Partner category", "place", "podcast", "brand"),
		"name": str("Partner name as returned by Qloo"),
		"idea": str("One concrete activation idea"),
	})),
	"risks": arr(str("A risk or data gap")),
	"competing_shows": arr(obj(map[string]any{
		"name":   str("Show name exactly as returned by competing_shows"),
		"venue":  str("Its venue"),
		"date":   str("Its local date, YYYY-MM-DD"),
		"threat": enum("How hard it competes for this booking's crowd, from genre overlap, distance and night", "high", "medium", "low"),
		"note":   str("One line on why"),
	})),
	"alternates": arr(obj(map[string]any{
		"name":      str("Artist name"),
		"entity_id": str("Qloo entity_id"),
		"reason":    str("Why this is a backup"),
	})),
	"evidence": arr(obj(map[string]any{
		"claim": str("A factual claim made in the memo"),
		"tool":  str("Name of the tool whose result supports it"),
		"ref":   str("The ref line of that tool result, e.g. Q4"),
	})),
}
