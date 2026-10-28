package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// Deal is the offer the memo recommends. The model sets the terms; Math is
// filled in by the server from the terms and the venue's costs, so the
// numbers a buyer reads never come from the model's arithmetic. Math is not
// part of the memo tool's schema.
type Deal struct {
	Guarantee       float64   `json:"guarantee"`
	BackendPct      float64   `json:"backend_pct"`
	SupportFee      float64   `json:"support_fee"`
	TicketAdvance   float64   `json:"ticket_price_advance"`
	TicketDoor      float64   `json:"ticket_price_door"`
	ExpectedTickets int       `json:"expected_tickets"`
	Terms           string    `json:"terms"`
	WalkAway        string    `json:"walk_away"`
	Rationale       string    `json:"rationale"`
	Math            *DealMath `json:"math,omitempty"`
}

// DealMath is the deal priced against the room. A "plus" deal: the artist
// gets the guarantee, plus BackendPct of the door above the split point
// (house costs + support fee + guarantee). Tickets are counted at the
// advance price, which is the conservative case. Fees and taxes are left out.
type DealMath struct {
	Capacity         int            `json:"capacity"`
	HouseCosts       float64        `json:"house_costs"`
	BarNetPerHead    float64        `json:"bar_net_per_head"`
	SplitPoint       float64        `json:"split_point"`
	BreakEvenTickets int            `json:"break_even_tickets"`  // the door covers costs and the guarantee
	BreakEvenWithBar int            `json:"break_even_with_bar"` // counting the bar's take too
	Scenarios        []DealScenario `json:"scenarios"`
}

type DealScenario struct {
	Label       string  `json:"label"`
	Tickets     int     `json:"tickets"`
	Gross       float64 `json:"gross"`
	ArtistTotal float64 `json:"artist_total"`
	VenueDoor   float64 `json:"venue_door"`
	VenueBar    float64 `json:"venue_bar"`
	VenueTotal  float64 `json:"venue_total"`
}

// dealMath prices d at half, three-quarters and a full house, plus the
// expected turnout when it differs. Nil when there is no ticket price.
func dealMath(v Venue, d Deal) *DealMath {
	if d.TicketAdvance <= 0 || v.Capacity <= 0 {
		return nil
	}
	split := v.HouseCosts + d.SupportFee + d.Guarantee
	m := &DealMath{
		Capacity: v.Capacity, HouseCosts: v.HouseCosts, BarNetPerHead: v.BarNetPerHead, SplitPoint: split,
		BreakEvenTickets: int(math.Ceil(split / d.TicketAdvance)),
		BreakEvenWithBar: int(math.Ceil(split / (d.TicketAdvance + v.BarNetPerHead))),
	}
	at := func(label string, tickets int) DealScenario {
		gross := float64(tickets) * d.TicketAdvance
		artist := d.Guarantee + d.BackendPct/100*math.Max(0, gross-split)
		door := gross - artist - v.HouseCosts - d.SupportFee
		bar := float64(tickets) * v.BarNetPerHead
		return DealScenario{Label: label, Tickets: tickets, Gross: math.Round(gross), ArtistTotal: math.Round(artist),
			VenueDoor: math.Round(door), VenueBar: math.Round(bar), VenueTotal: math.Round(door + bar)}
	}
	half, most := v.Capacity/2, v.Capacity*3/4
	m.Scenarios = []DealScenario{at("Half house", half), at("Three-quarters", most), at("Sellout", v.Capacity)}
	if e := min(d.ExpectedTickets, v.Capacity); e > 0 && e != half && e != most && e != v.Capacity {
		m.Scenarios = append(m.Scenarios, at("Expected", e))
		sort.SliceStable(m.Scenarios, func(i, j int) bool { return m.Scenarios[i].Tickets < m.Scenarios[j].Tickets })
	}
	return m
}

// priceDealTool lets the agent try terms against the room's costs before it
// commits to an offer.
func priceDealTool(v Venue) tool {
	return tool{
		name: "price_deal",
		description: fmt.Sprintf("Price an offer against this room (capacity %d, house costs $%.0f a night, bar nets about $%.0f a head). ", v.Capacity, v.HouseCosts, v.BarNetPerHead) +
			"The artist gets the guarantee plus backend_pct of the door above the split point (house costs + support fee + guarantee). " +
			"Returns break-even tickets and what the artist and the room take home at half, three-quarters and a full house. Try a few terms before you settle.",
		properties: map[string]any{
			"guarantee":            map[string]any{"type": "number", "description": "Guaranteed artist fee, USD"},
			"backend_pct":          map[string]any{"type": "number", "description": "Artist's share of the door above the split point, 0-100 (0 for a flat fee)"},
			"support_fee":          map[string]any{"type": "number", "description": "Flat fee for the support act, USD"},
			"ticket_price_advance": map[string]any{"type": "number", "description": "Advance ticket price, USD"},
			"expected_tickets":     map[string]any{"type": "integer", "description": "Your estimate of tickets sold"},
		},
		required: []string{"guarantee", "ticket_price_advance"},
		run: func(_ context.Context, raw json.RawMessage) (any, error) {
			var d Deal
			if err := json.Unmarshal(raw, &d); err != nil {
				return nil, fmt.Errorf("invalid input JSON: %w", err)
			}
			if d.TicketAdvance <= 0 || d.Guarantee < 0 || d.BackendPct < 0 || d.BackendPct > 100 || d.SupportFee < 0 {
				return nil, fmt.Errorf("need ticket_price_advance > 0, guarantee and support_fee >= 0, backend_pct 0-100")
			}
			m := dealMath(v, d)
			out := map[string]any{"split_point": m.SplitPoint, "break_even_tickets": m.BreakEvenTickets, "break_even_with_bar": m.BreakEvenWithBar, "scenarios": m.Scenarios}
			var warn []string
			if pct := float64(m.BreakEvenTickets) / float64(v.Capacity); pct > 0.7 {
				warn = append(warn, fmt.Sprintf("break-even needs %.0f%% of capacity on the door", pct*100))
			}
			if m.BreakEvenTickets > v.Capacity {
				warn = append(warn, "even a sellout doesn't cover the guarantee and costs on the door")
			}
			for _, s := range m.Scenarios {
				if s.Label == "Half house" && s.VenueTotal < 0 {
					warn = append(warn, fmt.Sprintf("the room loses $%.0f at half a house, bar included", -s.VenueTotal))
				}
			}
			if len(warn) > 0 {
				out["warnings"] = warn
			}
			return out, nil
		},
	}
}
