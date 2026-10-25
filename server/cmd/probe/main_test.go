package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"blastingroom/internal/agent"
	"blastingroom/internal/qloo"
)

func TestRunWithoutKeyExplainsAndExits(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run(context.Background(), nil, func(string) string { return "" }, &out, &errOut)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "QLOO_API_KEY is not set") {
		t.Fatalf("stderr = %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want nothing", out.String())
	}
}

func TestRunRejectsBadFlags(t *testing.T) {
	env := func(k string) string { return map[string]string{"QLOO_API_KEY": "k"}[k] }
	for _, args := range [][]string{{"-only", "9"}, {"-compare", "just one"}} {
		var out, errOut bytes.Buffer
		if code := run(context.Background(), args, env, &out, &errOut); code != 2 {
			t.Errorf("%v: exit = %d, want 2", args, code)
		}
	}
}

func mockConfig() config {
	return config{
		Artist:  "Mock Seed",
		Compare: [2]string{"Mock A", "Mock B"},
		Genre:   "indie rock",
		PopMax:  0.9,
		City:    "Chicago",
		Venue:   agent.DemoVenues[0],
		APIKey:  "mock-key",
		BaseURL: "https://mock.qloo.local",
	}
}

func TestProbeAllAgainstMock(t *testing.T) {
	var out bytes.Buffer
	code := probeAll(context.Background(), qloo.NewMock(), mockConfig(), &out)
	got := out.String()
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, got)
	}
	for _, want := range []string{
		"1. Heatmap", "2. Location-signaled", "3. Trending", "4. Compare", "5. Demographics", "6. Cross-domain", "7. Harness",
		"boundary=urn:entity:locality",
		"10/10 artists have a 6-month series",
		"strongest skew",
		"location{lat,lon} in 8/8; qloo.Entity.Coordinates() parsed 8/8",
		"7  SKIP",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "ERROR") {
		t.Errorf("a probe errored:\n%s", got)
	}
}

func TestProbeAllOnlyRunsSelected(t *testing.T) {
	cfg := mockConfig()
	cfg.Only = map[int]bool{3: true}
	var out bytes.Buffer
	if code := probeAll(context.Background(), qloo.NewMock(), cfg, &out); code != 0 {
		t.Fatalf("exit = %d\n%s", code, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "3. Trending") || strings.Contains(got, "1. Heatmap") {
		t.Fatalf("output:\n%s", got)
	}
}

// The three shapes Coordinates() accepts, plus one it doesn't, so the probe's
// report can tell the parser and Qloo's real shape apart. Synthetic, not Qloo data.
func TestCoordPathsAndSummary(t *testing.T) {
	cases := []struct {
		json string
		want string
	}{
		{`{"location":{"lat":41.9,"lon":-87.7}}`, "location{lat,lon} in 1/1; qloo.Entity.Coordinates() parsed 1/1"},
		{`{"properties":{"geocode":{"latitude":41.9,"longitude":-87.7}}}`, "properties.geocode{latitude,longitude} in 1/1; qloo.Entity.Coordinates() parsed 1/1"},
		{`{"properties":{"latitude":41.9,"longitude":-87.7}}`, "properties{latitude,longitude} in 1/1; qloo.Entity.Coordinates() parsed 1/1"},
		{`{"geo":{"point":{"lat":41.9,"lng":-87.7}}}`, "geo.point{lat,lng} in 1/1; qloo.Entity.Coordinates() parsed 0/1 (PARSER MISSES A SHAPE: fix Coordinates)"},
		{`{"properties":{"address":"1 Main St"}}`, "neither location nor properties.geocode: no lat/lon pair in 1 places; qloo.Entity.Coordinates() parsed 0"},
	}
	for _, c := range cases {
		var m map[string]any
		var e qloo.Entity
		json.Unmarshal([]byte(c.json), &m)
		json.Unmarshal([]byte(c.json), &e)
		shapes := map[string]int{}
		for _, s := range coordPaths(m, "", 0) {
			shapes[s]++
		}
		parsed := 0
		if _, _, ok := e.Coordinates(); ok {
			parsed = 1
		}
		if got := coordSummary(1, shapes, parsed); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.json, got, c.want)
		}
	}
}

func TestSeries(t *testing.T) {
	cases := []struct {
		raw   string
		n     int
		where string
	}{
		{`[{"date":"2026-10-01"},{"date":"2026-10-08"}]`, 2, "results"},
		{`{"entities":[{"date":"2026-10-01"}]}`, 1, "results.entities"},
		{`{"note":"x"}`, 0, "results with no array (fields [note])"},
	}
	for _, c := range cases {
		n, where, _ := series(json.RawMessage(c.raw))
		if n != c.n || where != c.where {
			t.Errorf("%s: got %d %q, want %d %q", c.raw, n, where, c.n, c.where)
		}
	}
}
