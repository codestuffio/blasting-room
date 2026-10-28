package agent

import (
	"encoding/json"
	"testing"
)

func TestDescribeCallUsesLearnedNames(t *testing.T) {
	n := newNames()
	fresh := n.learn(map[string]any{"results": []map[string]any{
		{"entity_id": "a1", "name": "Mock Artist 01", "popularity": 0.8},
		{"entity_id": "a2", "name": "Mock Artist 02"},
	}})
	if len(fresh) != 2 || fresh["a1"] != "Mock Artist 01" {
		t.Fatalf("learned %v", fresh)
	}
	if again := n.learn([]map[string]string{{"entity_id": "a1", "name": "Mock Artist 01"}}); again != nil {
		t.Fatalf("re-learned %v", again)
	}
	for _, c := range []struct{ tool, input, want string }{
		{"local_heatmap", `{"entity_id":"a1"}`, "Mock Artist 01"},
		{"compare_audiences", `{"group_a":["a1"],"group_b":["a2","zz"]}`, "Mock Artist 01 vs Mock Artist 02 +1 more"},
		{"score_shortlist", `{"candidate_ids":["x","y"]}`, "2 artists"},
		{"co_promo_partners", `{"entity_ids":["a1"],"kind":"podcast"}`, "Podcasts for Mock Artist 01"},
		{"competing_shows", `{"date":"2026-11-19"}`, "Nov 19 ± 1 night"},
		{"tour_dates", `{"artist_name":"Mock Day","date":"2026-11-19"}`, "Mock Day · Nov 19"},
		{"price_deal", `{"guarantee":2500,"backend_pct":70,"ticket_price_advance":27,"expected_tickets":400}`, "$2,500 + 70% · $27 adv · ~400 expected"},
		{"resolve_entity", `{"name":"Dehd","entity_type":"artist"}`, "“Dehd” (artist)"},
		{memoToolName, `{}`, ""},
	} {
		if got := describeCall(c.tool, json.RawMessage(c.input), n); got != c.want {
			t.Errorf("%s: %q, want %q", c.tool, got, c.want)
		}
	}
}
