package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"blastingroom/internal/ticketmaster"
)

// Synthetic stops around Copperline (East Austin, 30.2622,-97.7223).
func mockStop(date, city string, lat, lon float64) ticketmaster.Show {
	return ticketmaster.Show{Date: date, Venue: "Mock " + city + " Hall", City: city, Region: "TX", Lat: lat, Lon: lon}
}

var (
	houston = mockStop("2026-11-12", "Houston", 29.76, -95.37)
	dallas  = mockStop("2026-11-13", "Dallas", 32.78, -96.80)
	austin  = mockStop("2026-11-14", "Austin", 30.26, -97.73)
	okc     = mockStop("2026-11-16", "Oklahoma City", 35.47, -97.52)
	denver  = mockStop("2026-11-20", "Denver", 39.74, -104.99)
)

func fitOn(shows []ticketmaster.Show, date string) string {
	v, _ := FindVenue("copperline")
	d, _ := time.Parse(time.DateOnly, date)
	fit, _ := routeFit(tourStops(v, shows), d)
	return fit
}

func TestRouteFit(t *testing.T) {
	far := mockStop("2026-03-01", "Austin", 30.26, -97.73) // months away: no radius clause
	for _, c := range []struct {
		name  string
		shows []ticketmaster.Show
		date  string
		want  string
	}{
		{"playing elsewhere that night", []ticketmaster.Show{dallas}, "2026-11-13", "already_booked"},
		{"a show in town within the clause", []ticketmaster.Show{austin, okc}, "2026-12-01", "already_in_market"},
		{"between two stops a day's drive away", []ticketmaster.Show{houston, dallas}, "2026-11-12", "already_booked"},
		{"open night between Houston and Dallas", []ticketmaster.Show{houston, mockStop("2026-11-15", "Dallas", 32.78, -96.80)}, "2026-11-13", "on_the_way"},
		{"the night after the last date", []ticketmaster.Show{houston}, "2026-11-13", "on_the_way"},
		{"Denver is a long haul", []ticketmaster.Show{denver}, "2026-11-19", "detour"},
		{"nothing near the date", []ticketmaster.Show{far, denver}, "2026-07-01", "off_route"},
		{"no dates", nil, "2026-11-13", "no_tour_data"},
		{"no booking date", []ticketmaster.Show{dallas}, "", "not_checked"},
	} {
		if got := fitOn(c.shows, c.date); got != c.want {
			t.Errorf("%s: fit = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestRouteWindowsFindOpenNightsNearby(t *testing.T) {
	v, _ := FindVenue("copperline")
	today := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	w := routeWindows(tourStops(v, []ticketmaster.Show{houston, mockStop("2026-11-15", "Dallas", 32.78, -96.80), denver}), today)
	// The nights before Houston are open, and Houston → Dallas leaves 13-14 open.
	// Dallas → Denver is too far apart, and Denver too far to tail.
	var between bool
	for _, x := range w {
		between = between || x.From == "2026-11-13" && x.To == "2026-11-14" && x.After == "Houston, TX" && x.Before == "Dallas, TX"
	}
	if !between || len(w) != 2 || w[0].Before != "Houston, TX" {
		t.Fatalf("windows = %+v", w)
	}
	for _, x := range w {
		if strings.Contains(x.After+x.Before, "Denver") {
			t.Fatalf("Denver is beyond a day's drive: %+v", x)
		}
	}
}

func TestTourDatesToolStreamsTheRoute(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/attractions.json":
			io.WriteString(w, `{"_embedded":{"attractions":[{"id":"a1","name":"Mock Day","upcomingEvents":{"_total":2}}]}}`)
		case "/events.json":
			io.WriteString(w, `{"_embedded":{"events":[
			  {"id":"1","name":"Mock Day","dates":{"start":{"localDate":"2026-11-12"}},"priceRanges":[{"min":22,"max":27,"currency":"USD"}],
			   "_embedded":{"venues":[{"name":"Mock Houston Hall","city":{"name":"Houston"},"state":{"stateCode":"TX"},"location":{"latitude":"29.76","longitude":"-95.37"}}]}},
			  {"id":"2","name":"Mock Day","dates":{"start":{"localDate":"2026-11-15"}},
			   "_embedded":{"venues":[{"name":"Mock Dallas Hall","city":{"name":"Dallas"},"state":{"stateCode":"TX"},"location":{"latitude":"32.78","longitude":"-96.80"}}]}}
			]},"page":{"totalElements":2}}`)
		}
	}))
	defer srv.Close()
	shows := ticketmaster.New("k", ticketmaster.WithBaseURL(srv.URL), ticketmaster.WithRequestGap(0))
	v, _ := FindVenue("copperline")
	out, err := tourDatesTool(shows, v).run(context.Background(), json.RawMessage(`{"artist_name":"Mock Day","entity_id":"q-1","date":"2026-11-13"}`))
	if err != nil {
		t.Fatal(err)
	}
	r, ok := out.(withRoute)
	if !ok || r.route == nil {
		t.Fatalf("result is %T", out)
	}
	if r.route.Fit != "on_the_way" || r.route.EntityID != "q-1" || len(r.route.Stops) != 2 || r.route.Stops[0].Miles < 100 || len(r.route.Windows) == 0 {
		t.Fatalf("route = %+v", r.route)
	}
	b, _ := json.Marshal(r.model)
	if !strings.Contains(string(b), `"ticket_prices_elsewhere":"22-27 USD"`) || strings.Contains(string(b), "29.76") {
		t.Fatalf("model summary = %s", b)
	}

	// No key: say routing wasn't checked.
	out, _ = tourDatesTool(nil, v).run(context.Background(), json.RawMessage(`{"artist_name":"Mock Day"}`))
	if m := out.(map[string]any); m["checked"] != false {
		t.Fatalf("no-key result = %v", m)
	}
}
