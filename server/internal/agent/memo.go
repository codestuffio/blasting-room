package agent

import "sort"

// Memo is the booking memo the agent submits through the strict
// submit_booking_memo tool. Its JSON schema (memoSchema) must stay in sync.
type Memo struct {
	Summary    string       `json:"summary"`
	Confidence string       `json:"confidence"`
	Headliner  Pick         `json:"headliner"`
	Support    Pick         `json:"support"`
	LocalFit   string       `json:"local_fit"`
	Momentum   string       `json:"momentum"`
	Audience   Audience     `json:"audience"`
	CoPromo    []Partner    `json:"co_promo"`
	Risks      []string     `json:"risks"`
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

type Alternate struct {
	Name     string `json:"name"`
	EntityID string `json:"entity_id"`
	Reason   string `json:"reason"`
}

// Provenance ties a memo claim to the tool call that produced the evidence.
type Provenance struct {
	Claim string `json:"claim"`
	Tool  string `json:"tool"`
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
	"alternates": arr(obj(map[string]any{
		"name":      str("Artist name"),
		"entity_id": str("Qloo entity_id"),
		"reason":    str("Why this is a backup"),
	})),
	"evidence": arr(obj(map[string]any{
		"claim": str("A factual claim made in the memo"),
		"tool":  str("Name of the tool whose result supports it"),
	})),
}
