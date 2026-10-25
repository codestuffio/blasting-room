package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"blastingroom/internal/qloo"
)

// fakeQloo answers every request with body and records each query string.
type fakeQloo struct {
	body    string
	mu      sync.Mutex
	queries []url.Values
}

func (f *fakeQloo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.queries = append(f.queries, r.URL.Query())
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(f.body))
}

func runTool(t *testing.T, f *fakeQloo, name, input string) (any, error) {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	q := qloo.New("test", qloo.WithBaseURL(srv.URL))
	for _, tl := range toolset(q, DemoVenues[0]) {
		if tl.name == name {
			return tl.run(context.Background(), json.RawMessage(input))
		}
	}
	t.Fatalf("no tool %q", name)
	return nil, nil
}

// The live API returns nothing for filter.tags on artists and only superstars
// for a bare location, so discovery must send seeds, tags and the venue point
// as signals.
func TestDiscoverArtistsSendsSeedsTagsAndVenuePoint(t *testing.T) {
	f := &fakeQloo{body: `{"results":{"entities":[{"entity_id":"a1","name":"Act One","popularity":0.85}]}}`}
	if _, err := runTool(t, f, "discover_artists", `{"seed_ids":["s1","s2"],"genre_tags":["urn:tag:genre:music:indie_rock"],"popularity_max":0.9}`); err != nil {
		t.Fatal(err)
	}
	q := f.queries[0]
	for k, want := range map[string]string{
		"signal.interests.entities": "s1,s2",
		"signal.interests.tags":     "urn:tag:genre:music:indie_rock",
		"signal.location":           "POINT(-87.709300 41.923100)",
		"signal.location.radius":    signalRadius,
		"filter.popularity.max":     "0.9",
	} {
		if got := q.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	for _, k := range []string{"filter.tags", "signal.location.query", "bias.trends"} {
		if q.Has(k) {
			t.Errorf("query sends %s, which the live API ignores or fails on", k)
		}
	}
}

func TestDiscoverArtistsRequiresSeeds(t *testing.T) {
	f := &fakeQloo{body: `{"results":{"entities":[]}}`}
	if _, err := runTool(t, f, "discover_artists", `{"genre_tags":["urn:tag:genre:music:indie_rock"]}`); err == nil {
		t.Fatal("want an error without seed_ids")
	}
	if len(f.queries) != 0 {
		t.Fatalf("called Qloo %d times without seeds", len(f.queries))
	}
}

func TestScoreShortlistListsDroppedCandidates(t *testing.T) {
	f := &fakeQloo{body: `{"results":{"entities":[{"entity_id":"c1","name":"Kept","query":{"affinity":0.97}}]}}`}
	out, err := runTool(t, f, "score_shortlist", `{"candidate_ids":["c1","c2"],"seed_ids":["s1"]}`)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := out.(map[string]any)
	if !ok || !slices.Equal(m["no_local_signal"].([]string), []string{"c2"}) {
		t.Fatalf("result = %#v, want c2 under no_local_signal", out)
	}
	if got := f.queries[0].Get("signal.location"); got != DemoVenues[0].Point() {
		t.Fatalf("signal.location = %q", got)
	}

	// When every candidate is scored, the result stays a plain list.
	f = &fakeQloo{body: f.body}
	out, _ = runTool(t, f, "score_shortlist", `{"candidate_ids":["c1"]}`)
	if _, isList := out.([]map[string]any); !isList {
		t.Fatalf("result = %#v, want a plain list", out)
	}
}

func TestMomentumAsksForSixMonthsPerArtist(t *testing.T) {
	f := &fakeQloo{body: `{"results":{"trending":[]}}`}
	if _, err := runTool(t, f, "momentum", `{"entity_ids":["a1","a2"]}`); err != nil {
		t.Fatal(err)
	}
	if len(f.queries) != 2 {
		t.Fatalf("%d trending calls, want one per artist", len(f.queries))
	}
	for _, q := range f.queries {
		if q.Get("signal.interests.entities") != "a1" && q.Get("signal.interests.entities") != "a2" {
			t.Errorf("entities = %q, want a single id", q.Get("signal.interests.entities"))
		}
		start, _ := time.Parse("2006-01-02", q.Get("filter.start_date"))
		end, _ := time.Parse("2006-01-02", q.Get("filter.end_date"))
		if days := end.Sub(start).Hours() / 24; days < 180 || days > 186 {
			t.Errorf("window %s..%s is %.0f days, want about six months", q.Get("filter.start_date"), q.Get("filter.end_date"), days)
		}
	}
}

// Live compare responses repeat both groups' entities on every shared tag and
// ran past the old 6,000-byte clip; the summary keeps only the top tags.
func TestCompareAudiencesSummarizesTopTags(t *testing.T) {
	var tags []string
	for i := 0; i < 60; i++ {
		tags = append(tags, fmt.Sprintf(`{"name":"Tag %02d","tag_id":"urn:tag:genre:music:t%02d","subtype":"urn:tag:genre:music","query":{"score":%.3f,`+
			`"a.signal.interests.entities":[{"entity_id":"a1","name":"Artist A","subtype":"urn:entity:artist","disambiguation":"Artist A"}],`+
			`"b.signal.interests.entities":[{"entity_id":"b1","name":"Artist B","subtype":"urn:entity:artist","disambiguation":"Artist B"}]}}`, i, i, float64(i)/100))
	}
	// A repeated name, as Qloo sends for tags in several subtypes.
	tags = append(tags, tags[59])
	only := []string{`{"name":"Only 00","subtype":"urn:tag:genre:music"}`}
	for i := 0; i < 30; i++ {
		only = append(only, fmt.Sprintf(`{"name":"Only %02d","subtype":"urn:tag:keyword:media","query":{"count":"1"}}`, i))
	}
	body := fmt.Sprintf(`{"results":{"tags":[%s],"a":[%s],"b":[%s],"matchEntities":[{"name":"Shared Bar"}]}}`,
		strings.Join(tags, ","), strings.Join(only, ","), strings.Join(only[1:4], ","))
	f := &fakeQloo{body: body}

	out, err := runTool(t, f, "compare_audiences", `{"group_a":["a1"],"group_b":["b1"]}`)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(out)
	if len(body) < 6000 || len(b) > 2500 || strings.Contains(string(b), "truncated") {
		t.Fatalf("raw %d bytes → summary %d bytes: %s", len(body), len(b), b)
	}
	var got struct {
		Shared []struct {
			Name  string  `json:"name"`
			Kind  string  `json:"kind"`
			Score float64 `json:"score"`
		} `json:"shared_tags"`
		SharedCount int      `json:"shared_tag_count"`
		OnlyA       []string `json:"only_group_a"`
		OnlyACount  int      `json:"only_group_a_count"`
		OnlyB       []string `json:"only_group_b"`
		Entities    []string `json:"shared_entities"`
	}
	json.Unmarshal(b, &got)
	if len(got.Shared) != compareSharedMax || got.Shared[0].Name != "Tag 59" || got.Shared[1].Name != "Tag 58" || got.Shared[0].Score != 0.59 || got.Shared[0].Kind != "genre" || got.SharedCount != 61 {
		t.Errorf("shared = %+v (count %d), want the %d highest-scored first", got.Shared, got.SharedCount, compareSharedMax)
	}
	if len(got.OnlyA) != compareDistinctMax || got.OnlyA[0] != "Only 00" || got.OnlyA[1] != "Only 01" || got.OnlyACount != 31 || len(got.OnlyB) != 3 {
		t.Errorf("only_a = %v (count %d), only_b = %v", got.OnlyA, got.OnlyACount, got.OnlyB)
	}
	if len(got.Entities) != 1 || got.Entities[0] != "Shared Bar" {
		t.Errorf("shared_entities = %v", got.Entities)
	}
}

func TestCompareAudiencesEmptyCarriesANote(t *testing.T) {
	out, err := runTool(t, &fakeQloo{body: `{"results":{"tags":[],"a":[],"b":[],"matchEntities":[]}}`}, "compare_audiences", `{"group_a":["a1"],"group_b":["b1"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if m, ok := out.(map[string]any); !ok || m["note"] == nil {
		t.Fatalf("result = %#v, want a note", out)
	}
}
