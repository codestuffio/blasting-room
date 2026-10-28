package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// names remembers the entity names a run's tool results mention, so the
// trace can read "Neighborhood heatmap · Wednesday" instead of an id.
type names struct {
	mu sync.Mutex
	m  map[string]string
}

func newNames() *names { return &names{m: map[string]string{}} }

// learn records every {entity_id, name} pair in a tool result and returns
// the ones it hadn't seen.
func (n *names) learn(out any) map[string]string {
	b, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	var v any
	if json.Unmarshal(b, &v) != nil {
		return nil
	}
	found := map[string]string{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			id, _ := x["entity_id"].(string)
			name, _ := x["name"].(string)
			if id != "" && name != "" {
				found[id] = name
			}
			for _, c := range x {
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(v)
	n.mu.Lock()
	defer n.mu.Unlock()
	fresh := map[string]string{}
	for id, name := range found {
		if _, ok := n.m[id]; !ok {
			n.m[id] = name
			fresh[id] = name
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	return fresh
}

// list names ids, the first few by name and the rest as a count.
func (n *names) list(ids []string) string {
	n.mu.Lock()
	defer n.mu.Unlock()
	var shown []string
	unknown := 0
	for _, id := range ids {
		if name, ok := n.m[id]; ok && len(shown) < 3 {
			shown = append(shown, name)
		} else {
			unknown++
		}
	}
	switch {
	case len(shown) == 0 && unknown == 1:
		return "1 artist"
	case len(shown) == 0:
		return fmt.Sprintf("%d artists", unknown)
	case unknown > 0:
		return fmt.Sprintf("%s +%d more", strings.Join(shown, ", "), unknown)
	default:
		return strings.Join(shown, ", ")
	}
}

// describeCall is a one-line, human summary of a tool call for the trace.
// Empty when there's nothing useful to say.
func describeCall(tool string, raw json.RawMessage, n *names) string {
	var in struct {
		Name          string   `json:"name"`
		EntityType    string   `json:"entity_type"`
		Query         string   `json:"query"`
		SeedIDs       []string `json:"seed_ids"`
		GenreTags     []string `json:"genre_tags"`
		PopularityMax float64  `json:"popularity_max"`
		CandidateIDs  []string `json:"candidate_ids"`
		EntityID      string   `json:"entity_id"`
		EntityIDs     []string `json:"entity_ids"`
		GroupA        []string `json:"group_a"`
		GroupB        []string `json:"group_b"`
		Kind          string   `json:"kind"`
		Date          string   `json:"date"`
		Nights        *int     `json:"nights_either_side"`
		ArtistName    string   `json:"artist_name"`
		Guarantee     float64  `json:"guarantee"`
		BackendPct    float64  `json:"backend_pct"`
		Ticket        float64  `json:"ticket_price_advance"`
		Expected      int      `json:"expected_tickets"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return ""
	}
	ids := in.EntityIDs
	if in.EntityID != "" {
		ids = append([]string{in.EntityID}, ids...)
	}
	switch tool {
	case "resolve_entity":
		return fmt.Sprintf("“%s” (%s)", in.Name, in.EntityType)
	case "find_genre_tags":
		return fmt.Sprintf("“%s”", in.Query)
	case "discover_artists":
		s := "Seeds: " + n.list(in.SeedIDs)
		if len(in.GenreTags) > 0 {
			s += fmt.Sprintf(" · %d genre tags", len(in.GenreTags))
		}
		if in.PopularityMax > 0 {
			s += fmt.Sprintf(" · popularity ≤ %.2f", in.PopularityMax)
		}
		return s
	case "score_shortlist":
		return n.list(in.CandidateIDs)
	case "local_heatmap", "momentum", "audience_demographics":
		return n.list(ids)
	case "compare_audiences":
		return n.list(in.GroupA) + " vs " + n.list(in.GroupB)
	case "co_promo_partners":
		kinds := map[string]string{"place": "Nearby spots", "podcast": "Podcasts", "brand": "Brands"}
		return kinds[in.Kind] + " for " + n.list(ids)
	case "competing_shows":
		nights := 1
		if in.Nights != nil {
			nights = *in.Nights
		}
		return fmt.Sprintf("%s ± %d %s", shortDate(in.Date), nights, map[bool]string{true: "night", false: "nights"}[nights == 1])
	case "tour_dates":
		if in.Date != "" {
			return in.ArtistName + " · " + shortDate(in.Date)
		}
		return in.ArtistName
	case "price_deal":
		s := fmt.Sprintf("$%s", thousands(in.Guarantee))
		if in.BackendPct > 0 {
			s += fmt.Sprintf(" + %.0f%%", in.BackendPct)
		}
		s += fmt.Sprintf(" · $%.0f adv", in.Ticket)
		if in.Expected > 0 {
			s += fmt.Sprintf(" · ~%d expected", in.Expected)
		}
		return s
	}
	return ""
}

func shortDate(d string) string {
	t, err := time.Parse(time.DateOnly, d)
	if err != nil {
		return d
	}
	return t.Format("Jan 2")
}

func thousands(f float64) string {
	s := fmt.Sprintf("%.0f", f)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
