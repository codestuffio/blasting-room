package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDealMathPricesAPlusDeal(t *testing.T) {
	v, _ := FindVenue("lantern") // 450 cap, $1,800 house costs, $9 bar a head
	m := dealMath(v, Deal{Guarantee: 1400, BackendPct: 80, SupportFee: 300, TicketAdvance: 20, ExpectedTickets: 300})
	if m.SplitPoint != 3500 || m.BreakEvenTickets != 175 || m.BreakEvenWithBar != 121 {
		t.Fatalf("split %v, break-even %d / %d with bar", m.SplitPoint, m.BreakEvenTickets, m.BreakEvenWithBar)
	}
	want := []DealScenario{
		{Label: "Half house", Tickets: 225, Gross: 4500, ArtistTotal: 2200, VenueDoor: 200, VenueBar: 2025, VenueTotal: 2225},
		{Label: "Expected", Tickets: 300, Gross: 6000, ArtistTotal: 3400, VenueDoor: 500, VenueBar: 2700, VenueTotal: 3200},
		{Label: "Three-quarters", Tickets: 337, Gross: 6740, ArtistTotal: 3992, VenueDoor: 648, VenueBar: 3033, VenueTotal: 3681},
		{Label: "Sellout", Tickets: 450, Gross: 9000, ArtistTotal: 5800, VenueDoor: 1100, VenueBar: 4050, VenueTotal: 5150},
	}
	if len(m.Scenarios) != len(want) {
		t.Fatalf("scenarios = %+v", m.Scenarios)
	}
	for i := range want {
		if m.Scenarios[i] != want[i] {
			t.Errorf("scenario %d = %+v, want %+v", i, m.Scenarios[i], want[i])
		}
	}
	if dealMath(v, Deal{Guarantee: 0, TicketAdvance: 0}) != nil {
		t.Fatal("a pass with no ticket price should have no math")
	}
}

func TestPriceDealWarnsWhenTheRoomLoses(t *testing.T) {
	var price tool
	for _, tl := range toolset(nil, nil, DemoVenues[0]) {
		if tl.name == "price_deal" {
			price = tl
		}
	}
	out, err := price.run(context.Background(), json.RawMessage(`{"guarantee":5000,"ticket_price_advance":18}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(out)
	for _, w := range []string{"break-even needs", "loses $"} {
		if !strings.Contains(string(b), w) {
			t.Errorf("missing warning %q in %s", w, b)
		}
	}
	if _, err := price.run(context.Background(), json.RawMessage(`{"guarantee":1000,"ticket_price_advance":0}`)); err == nil {
		t.Fatal("expected an error for a zero ticket price")
	}
}
