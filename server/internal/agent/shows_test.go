package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"blastingroom/internal/ticketmaster"
)

// Synthetic Discovery API events: two at one venue, one elsewhere.
const showsBody = `{"_embedded":{"events":[
  {"id":"e1","name":"Mock Show 01","dates":{"start":{"localDate":"2026-11-11","localTime":"19:00:00"},"status":{"code":"onsale"}},
   "_embedded":{"venues":[{"id":"v1","name":"Mock Club","distance":0.4,"location":{"latitude":"41.925","longitude":"-87.71"}}]}},
  {"id":"e2","name":"Mock Show 02","dates":{"start":{"localDate":"2026-11-12","localTime":"20:00:00"},"status":{"code":"onsale"}},
   "classifications":[{"primary":true,"genre":{"name":"Rock"},"subGenre":{"name":"Indie Rock"}}],
   "_embedded":{"venues":[{"id":"v2","name":"Mock Hall","distance":3.1,"location":{"latitude":"41.94","longitude":"-87.69"}}],"attractions":[{"name":"Mock Band"}]}},
  {"id":"e3","name":"Mock Show 03","dates":{"start":{"localDate":"2026-11-12","localTime":"21:00:00"},"status":{"code":"postponed"}},
   "_embedded":{"venues":[{"id":"v1","name":"Mock Club","distance":0.4,"location":{"latitude":"41.925","longitude":"-87.71"}}]}}
]},"page":{"totalElements":3}}`

func runShows(t *testing.T, shows *ticketmaster.Client, input string) (any, error) {
	t.Helper()
	for _, tl := range toolset(nil, shows, DemoVenues[0]) {
		if tl.name == "competing_shows" {
			return tl.run(context.Background(), json.RawMessage(input))
		}
	}
	t.Fatal("no competing_shows tool")
	return nil, nil
}

func TestCompetingShowsSearchesAroundTheDate(t *testing.T) {
	var q url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q = r.URL.Query()
		io.WriteString(w, showsBody)
	}))
	defer srv.Close()

	out, err := runShows(t, ticketmaster.New("k", ticketmaster.WithBaseURL(srv.URL)), `{"date":"2026-11-12"}`)
	if err != nil {
		t.Fatal(err)
	}
	if q.Get("localStartDateTime") != "2026-11-11T00:00:00,2026-11-13T23:59:59" || q.Get("radius") != "5" {
		t.Fatalf("query = %v", q)
	}
	g, ok := out.(withGeo)
	if !ok {
		t.Fatalf("result is %T, want withGeo", out)
	}

	// The model reads the booking night first, nearest venue first.
	b, _ := json.Marshal(g.model)
	var sum struct {
		Checked bool
		Window  string
		Shows   []map[string]any
	}
	json.Unmarshal(b, &sum)
	var names []string
	for _, s := range sum.Shows {
		names = append(names, s["name"].(string))
	}
	if !sum.Checked || sum.Window != "2026-11-11 to 2026-11-13" || strings.Join(names, ",") != "Mock Show 03,Mock Show 02,Mock Show 01" {
		t.Fatalf("summary = %s", b)
	}
	if sum.Shows[0]["status"] != "postponed" || sum.Shows[1]["genre"] != "Rock / Indie Rock" || sum.Shows[1]["time"] != "20:00" {
		t.Fatalf("show fields = %s", b)
	}

	// The map gets one pin per venue, listing its shows.
	if g.geo == nil || g.geo.Kind != "shows" || len(g.geo.Places) != 2 {
		t.Fatalf("geo = %+v", g.geo)
	}
	club := g.geo.Places[0]
	if club.Name != "Mock Club" || club.Address != "Mock Show 01 (2026-11-11); Mock Show 03 (2026-11-12)" {
		t.Fatalf("venue pin = %+v", club)
	}
	if strings.Contains(string(b), "41.925") {
		t.Fatalf("coordinates leaked to the model: %s", b)
	}
}

func TestCompetingShowsWithoutAKeySaysSo(t *testing.T) {
	out, err := runShows(t, nil, `{"date":"2026-11-12"}`)
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]any)
	if m["checked"] != false || !strings.Contains(m["note"].(string), "not configured") {
		t.Fatalf("result = %v", m)
	}
	if _, err := runShows(t, nil, `{"date":"next Thursday"}`); err == nil {
		t.Fatal("expected a date error")
	}
}
