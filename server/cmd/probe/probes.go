package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"

	"blastingroom/internal/qloo"
)

// Verdict levels. PASS means the machine-checkable part held up; names still
// need a human eye for plausibility.
const (
	pass   = "PASS"
	weak   = "WEAK"
	fail   = "FAIL"
	errLvl = "ERROR"
	skip   = "SKIP"
)

type verdict struct {
	level string
	text  string
}

type prober struct {
	c   *qloo.Client
	cfg config
	w   io.Writer

	seed   qloo.Entity   // cfg.Artist, resolved once
	tag    *qloo.Tag     // cfg.Genre, resolved once
	recs   []qloo.Entity // mid-tier recs from probe 2, reused by probe 3
	coords string        // place coordinate shape found by probe 6
}

// probeAll runs the selected probes and prints a summary. It returns 1 if any
// probe errored, so a failed run is visible in scripts.
func probeAll(ctx context.Context, c *qloo.Client, cfg config, w io.Writer) int {
	p := &prober{c: c, cfg: cfg, w: w}
	probes := []struct {
		n     int
		title string
		run   func(context.Context) (verdict, error)
	}{
		{1, "Heatmap for a mid-size artist across one metro", p.heatmap},
		{2, "Location-signaled artist recs", p.recommendations},
		{3, "Trending velocity for emerging artists", p.trending},
		{4, "Compare two similar artists", p.compare},
		{5, "Demographics for a niche artist", p.demographics},
		{6, "Cross-domain: artist → place and podcast", p.crossDomain},
		{7, "Harness (qloo mcp) against the hackathon host", p.harness},
	}

	var results []string
	code := 0
	for _, pr := range probes {
		if cfg.Only != nil && !cfg.Only[pr.n] {
			continue
		}
		fmt.Fprintf(w, "\n%d. %s\n", pr.n, pr.title)
		v, err := pr.run(ctx)
		if err != nil {
			v, code = verdict{errLvl, err.Error()}, 1
		}
		fmt.Fprintf(w, "   → %s: %s\n", v.level, v.text)
		results = append(results, fmt.Sprintf("  %d  %-5s %s", pr.n, v.level, v.text))
	}

	fmt.Fprintln(w, "\nSummary")
	for _, r := range results {
		fmt.Fprintln(w, r)
	}
	if p.coords != "" {
		fmt.Fprintf(w, "\nPlace coordinates: %s\n", p.coords)
	}
	return code
}

func (p *prober) say(format string, args ...any) {
	fmt.Fprintf(p.w, "   "+format+"\n", args...)
}

func (p *prober) resolve(ctx context.Context, name string) (qloo.Entity, error) {
	res, err := p.c.Search(ctx, name, []string{qloo.TypeArtist}, 1)
	if err != nil {
		return qloo.Entity{}, err
	}
	if len(res) == 0 {
		return qloo.Entity{}, fmt.Errorf("search found no artist for %q", name)
	}
	e := res[0]
	p.say("%q → %s (%s, popularity %.3f)", name, e.Name, e.EntityID, e.Popularity)
	return e, nil
}

func (p *prober) seedArtist(ctx context.Context) (qloo.Entity, error) {
	if p.seed.EntityID == "" {
		e, err := p.resolve(ctx, p.cfg.Artist)
		if err != nil {
			return e, err
		}
		p.seed = e
	}
	return p.seed, nil
}

// 1. Heatmap. It asks twice: once exactly as local_heatmap does (default
// boundary, which the map draws), once with the locality boundary (the API
// accepts only urn:geohash or urn:entity:locality, not "neighborhood").
func (p *prober) heatmap(ctx context.Context) (verdict, error) {
	a, err := p.seedArtist(ctx)
	if err != nil {
		return verdict{}, err
	}
	best, parts, ok := fail, []string{}, 0
	for _, boundary := range []string{"", "urn:entity:locality"} {
		q := url.Values{
			"filter.type":               {qloo.TypeHeatmap},
			"signal.interests.entities": {a.EntityID},
			"filter.location.query":     {p.cfg.City},
		}
		label := "default boundary"
		if boundary != "" {
			q.Set("output.heatmap.boundary", boundary)
			label = "boundary=" + boundary
		}
		var body struct {
			Results struct {
				Heatmap []json.RawMessage `json:"heatmap"`
			} `json:"results"`
		}
		raw, err := p.c.Raw(ctx, "/v2/insights", q)
		if err == nil {
			err = json.Unmarshal(raw, &body)
		}
		if err != nil {
			p.say("%s: error: %v", label, err)
			parts = append(parts, label+" errored")
			continue
		}
		cells := body.Results.Heatmap
		var affs []float64
		located, bad := 0, 0
		for _, c := range cells {
			var pt qloo.HeatmapPoint
			if err := json.Unmarshal(c, &pt); err != nil {
				bad++
				continue
			}
			affs = append(affs, pt.Query.Affinity)
			if pt.Location.Latitude != 0 || pt.Location.Longitude != 0 {
				located++
			}
		}
		s := spreadOf(affs)
		p.say("%s: %d cells, %d with lat/lon as qloo.HeatmapPoint reads it, %d undecodable", label, len(cells), located, bad)
		p.say("  affinity %s", s)
		if len(cells) > 0 {
			p.say("  cell fields %v; location fields %v", keysOf(cells[0], ""), keysOf(cells[0], "location"))
		}

		level := pass
		switch {
		case len(cells) == 0:
			level = fail
		case len(cells) < 5 || s.max-s.min < 0.2:
			level = weak
		}
		best, ok = better(best, level), ok+1
		parts = append(parts, fmt.Sprintf("%s %d cells, spread %.2f (%s)", label, len(cells), s.max-s.min, level))
	}
	if ok == 0 {
		return verdict{}, fmt.Errorf("both heatmap requests failed")
	}
	return verdict{best, strings.Join(parts, "; ")}, nil
}

func (p *prober) genreTag(ctx context.Context) (*qloo.Tag, error) {
	if p.tag != nil {
		return p.tag, nil
	}
	tags, err := p.c.FindTags(ctx, p.cfg.Genre, nil, 10)
	if err != nil {
		return nil, err
	}
	// Prefer a genre tag named exactly like the input, then any genre tag.
	for _, exact := range []bool{true, false} {
		for _, t := range tags {
			if strings.Contains(t.URN(), "genre") && (!exact || strings.EqualFold(t.Name, p.cfg.Genre)) {
				p.say("genre %q → %s (%s)", p.cfg.Genre, t.URN(), t.Name)
				p.tag = &t
				return p.tag, nil
			}
		}
	}
	return nil, fmt.Errorf("no genre tag among %d tags for %q", len(tags), p.cfg.Genre)
}

// recommend asks for artists who share the seed artist's audience and
// over-index around the venue, the way discover_artists does, capped at
// popMax when it is in (0, 1).
func (p *prober) recommend(ctx context.Context, popMax float64) ([]qloo.Entity, error) {
	seed, err := p.seedArtist(ctx)
	if err != nil {
		return nil, err
	}
	tag, err := p.genreTag(ctx)
	if err != nil {
		return nil, err
	}
	v := p.cfg.Venue
	q := url.Values{
		"filter.type":               {qloo.TypeArtist},
		"signal.interests.entities": {seed.EntityID},
		"signal.interests.tags":     {tag.URN()},
		"signal.location":           {v.Point()},
		"signal.location.radius":    {"5000"},
		"take":                      {"15"},
	}
	if popMax > 0 && popMax < 1 {
		q.Set("filter.popularity.max", fmt.Sprint(popMax))
	}
	res, err := p.c.Insights(ctx, q)
	if err != nil {
		return nil, err
	}
	return res.Entities, nil
}

// 2. Seed- and location-signaled recs, uncapped and then capped toward mid-tier acts.
func (p *prober) recommendations(ctx context.Context) (verdict, error) {
	open, err := p.recommend(ctx, 0)
	if err != nil {
		return verdict{}, err
	}
	p.say("uncapped: %d artists, popularity %s", len(open), spreadOf(popularities(open)))
	p.say("  %s", names(open, 8))

	capped, err := p.recommend(ctx, p.cfg.PopMax)
	if err != nil {
		return verdict{}, err
	}
	pops := popularities(capped)
	p.say("popularity.max=%.2f: %d artists, popularity %s, affinity %s", p.cfg.PopMax, len(capped), spreadOf(pops), spreadOf(affinities(capped)))
	p.say("  %s", names(capped, 8))
	p.recs = capped

	over := 0
	for _, x := range pops {
		if x > p.cfg.PopMax {
			over++
		}
	}
	switch {
	case len(open) == 0 && len(capped) == 0:
		return verdict{fail, "no recs around " + p.cfg.Venue.LocationQuery()}, nil
	case len(capped) == 0:
		p.recs = open
		return verdict{weak, fmt.Sprintf("%d uncapped recs (median popularity %.3f) but none under popularity %.2f", len(open), spreadOf(popularities(open)).med, p.cfg.PopMax)}, nil
	case over > 0:
		return verdict{weak, fmt.Sprintf("%d recs, but %d exceed popularity.max %.2f: the cap looks ignored", len(capped), over, p.cfg.PopMax)}, nil
	case len(capped) < 5:
		return verdict{weak, fmt.Sprintf("only %d mid-tier recs", len(capped))}, nil
	}
	return verdict{pass, fmt.Sprintf("%d uncapped, %d mid-tier recs (median popularity %.3f); check the names are plausible", len(open), len(capped), spreadOf(pops).med)}, nil
}

// 3. Trending, one artist per call over six months, as momentum calls it.
// It also checks that the series actually move: the hackathon host has served
// a constant percentile for every artist.
func (p *prober) trending(ctx context.Context) (verdict, error) {
	if len(p.recs) == 0 {
		recs, err := p.recommend(ctx, p.cfg.PopMax)
		if err != nil {
			return verdict{}, err
		}
		p.recs = recs
	}
	ents := p.recs
	if len(ents) > 10 {
		ents = ents[:10]
	}
	if len(ents) == 0 {
		return verdict{fail, "no candidate artists to trend"}, nil
	}

	end := time.Now()
	start := end.AddDate(0, -6, 0)
	populated, points, moving := 0, 0, 0
	var fields []string
	for _, e := range ents {
		raw, err := p.c.Trending(ctx, qloo.TypeArtist, e.EntityID, start, end)
		if err != nil {
			p.say("%-28s error: %v", e.Name, err)
			continue
		}
		n, where, first := series(raw)
		s := spreadOf(percentiles(raw))
		p.say("%-28s %3d points in %s, percentile %s", e.Name, n, where, s)
		if n > 0 {
			populated++
			points += n
			if s.max > s.min {
				moving++
			}
			if fields == nil {
				fields = keysOf(first, "")
			}
		}
	}
	if fields != nil {
		p.say("point fields %v", fields)
	}

	text := fmt.Sprintf("%d/%d artists have a 6-month series", populated, len(ents))
	if populated > 0 {
		text += fmt.Sprintf(" (avg %d points), %d of them move", points/populated, moving)
	}
	switch {
	case populated == 0:
		return verdict{fail, text}, nil
	case moving == 0:
		return verdict{weak, text + ": every series is constant, so there is no momentum signal"}, nil
	case populated*10 < len(ents)*7:
		return verdict{weak, text}, nil
	}
	return verdict{pass, text}, nil
}

// 4. Compare, with the same extra params compare_audiences sends.
func (p *prober) compare(ctx context.Context) (verdict, error) {
	a, err := p.resolve(ctx, p.cfg.Compare[0])
	if err != nil {
		return verdict{}, err
	}
	b, err := p.resolve(ctx, p.cfg.Compare[1])
	if err != nil {
		return verdict{}, err
	}
	raw, err := p.c.Compare(ctx, []string{a.EntityID}, []string{b.EntityID}, url.Values{"take": {"15"}})
	if err != nil {
		return verdict{}, err
	}
	items := describe(p, raw)
	if items == 0 {
		return verdict{fail, "compare returned no items"}, nil
	}
	return verdict{pass, fmt.Sprintf("%d items for %s vs %s; check overlap and difference read sensibly", items, a.Name, b.Name)}, nil
}

// 5. Demographics: every numeric leaf of the result is a skew score.
func (p *prober) demographics(ctx context.Context) (verdict, error) {
	a, err := p.seedArtist(ctx)
	if err != nil {
		return verdict{}, err
	}
	res, err := p.c.Insights(ctx, url.Values{
		"filter.type":               {qloo.TypeDemographics},
		"signal.interests.entities": {a.EntityID},
	})
	if err != nil {
		return verdict{}, err
	}
	var v any
	if len(res.Demographics) > 0 {
		if err := json.Unmarshal(res.Demographics, &v); err != nil {
			return verdict{}, fmt.Errorf("demographics: %w", err)
		}
	}
	leaves := map[string]float64{}
	numericLeaves(v, "", leaves)
	paths := make([]string, 0, len(leaves))
	for k := range leaves {
		paths = append(paths, k)
	}
	sort.Strings(paths)
	maxAbs, maxPath := 0.0, ""
	for _, k := range paths {
		p.say("%-40s %+.3f", k, leaves[k])
		if math.Abs(leaves[k]) > maxAbs {
			maxAbs, maxPath = math.Abs(leaves[k]), k
		}
	}
	switch {
	case len(leaves) == 0:
		return verdict{fail, "no demographic scores"}, nil
	case maxAbs < 0.1:
		return verdict{weak, fmt.Sprintf("%d scores, all within ±0.1", len(leaves))}, nil
	}
	return verdict{pass, fmt.Sprintf("%d scores, strongest skew %+.2f at %s", len(leaves), leaves[maxPath], maxPath)}, nil
}

// 6. Cross-domain. The place request matches co_promo_partners, and its raw
// entities are checked for where coordinates live.
func (p *prober) crossDomain(ctx context.Context) (verdict, error) {
	a, err := p.seedArtist(ctx)
	if err != nil {
		return verdict{}, err
	}
	v := p.cfg.Venue
	raw, err := p.c.Raw(ctx, "/v2/insights", url.Values{
		"filter.type":               {qloo.TypePlace},
		"signal.interests.entities": {a.EntityID},
		"filter.location":           {fmt.Sprintf("POINT(%f %f)", v.Lon, v.Lat)},
		"filter.location.radius":    {"3000"},
		"take":                      {"8"},
	})
	if err != nil {
		return verdict{}, err
	}
	var body struct {
		Results struct {
			Entities []json.RawMessage `json:"entities"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return verdict{}, fmt.Errorf("places: %w", err)
	}
	places := body.Results.Entities
	shapes := map[string]int{}
	parsed := 0
	var firstFields, firstProps []string
	for i, r := range places {
		var e qloo.Entity
		if err := json.Unmarshal(r, &e); err != nil {
			p.say("place %d undecodable as qloo.Entity: %v", i, err)
			continue
		}
		var m map[string]any
		json.Unmarshal(r, &m)
		for _, s := range coordPaths(m, "", 0) {
			shapes[s]++
		}
		where := "no coordinates"
		if lat, lon, ok := e.Coordinates(); ok {
			parsed++
			where = fmt.Sprintf("%.1f km from venue", km(v.Lat, v.Lon, lat, lon))
		}
		aff, _ := e.Affinity()
		p.say("%-32s aff %.3f  %s", e.Name, aff, where)
		if i == 0 {
			firstFields, firstProps = keysOf(r, ""), keysOf(r, "properties")
		}
	}
	p.coords = coordSummary(len(places), shapes, parsed)
	p.say("coordinates: %s", p.coords)
	if len(places) > 0 && len(shapes) == 0 {
		p.say("  place fields %v; properties fields %v", firstFields, firstProps)
	}

	pods, err := p.c.Insights(ctx, url.Values{
		"filter.type":               {qloo.TypePodcast},
		"signal.interests.entities": {a.EntityID},
		"take":                      {"8"},
	})
	if err != nil {
		return verdict{}, err
	}
	p.say("podcasts: %d", len(pods.Entities))
	p.say("  %s", names(pods.Entities, 8))

	text := fmt.Sprintf("%d places within 3 km, %d podcasts; check relevance", len(places), len(pods.Entities))
	switch {
	case len(places) == 0 && len(pods.Entities) == 0:
		return verdict{fail, text}, nil
	case len(places) == 0 || len(pods.Entities) == 0:
		return verdict{weak, text}, nil
	}
	return verdict{pass, text}, nil
}

// coordPaths finds objects (to depth 3) holding a numeric latitude and
// longitude pair, e.g. "location{lat,lon}" or "properties.geocode{latitude,longitude}".
func coordPaths(m map[string]any, prefix string, depth int) []string {
	var out []string
	if lat, lon := pick(m, "lat", "latitude"), pick(m, "lon", "lng", "long", "longitude"); lat != "" && lon != "" {
		name := strings.TrimSuffix(prefix, ".")
		if name == "" {
			name = "(top level)"
		}
		out = append(out, fmt.Sprintf("%s{%s,%s}", name, lat, lon))
	}
	if depth >= 3 {
		return out
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if sub, ok := m[k].(map[string]any); ok {
			out = append(out, coordPaths(sub, prefix+k+".", depth+1)...)
		}
	}
	return out
}

func pick(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if _, ok := m[k].(float64); ok {
			return k
		}
	}
	return ""
}

func coordSummary(total int, shapes map[string]int, parsed int) string {
	if total == 0 {
		return "no places returned, shape unknown"
	}
	if len(shapes) == 0 {
		return fmt.Sprintf("neither location nor properties.geocode: no lat/lon pair in %d places; qloo.Entity.Coordinates() parsed %d", total, parsed)
	}
	keys := make([]string, 0, len(shapes))
	for k := range shapes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s in %d/%d", k, shapes[k], total)
	}
	s := strings.Join(parts, ", ") + fmt.Sprintf("; qloo.Entity.Coordinates() parsed %d/%d", parsed, total)
	for _, n := range shapes {
		if n > parsed {
			return s + " (PARSER MISSES A SHAPE: fix Coordinates)"
		}
	}
	return s
}

// series finds the time series in a /v2/trending result: either the result
// itself or its first array field. It returns the length, where it was found,
// and the first point.
func series(raw json.RawMessage) (int, string, json.RawMessage) {
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil {
		return len(arr), "results", first(arr)
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return 0, "unrecognized results", nil
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if json.Unmarshal(obj[k], &arr) == nil {
			return len(arr), "results." + k, first(arr)
		}
	}
	return 0, fmt.Sprintf("results with no array (fields %v)", keys), nil
}

func first(arr []json.RawMessage) json.RawMessage {
	if len(arr) == 0 {
		return nil
	}
	return arr[0]
}

// describe prints each top-level field of a result with its size and, for
// lists of named things, the first names. It returns the total item count.
func describe(p *prober, raw json.RawMessage) int {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		p.say("results are not an object: %s", truncate(string(raw), 120))
		return 0
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	total := 0
	for _, k := range keys {
		var arr []map[string]any
		if json.Unmarshal(obj[k], &arr) != nil {
			p.say("%-12s %s", k, truncate(string(obj[k]), 80))
			continue
		}
		total += len(arr)
		var ns []string
		for _, it := range arr {
			if n, ok := it["name"].(string); ok && len(ns) < 6 {
				ns = append(ns, n)
			}
		}
		p.say("%-12s %d items  %s", k, len(arr), strings.Join(ns, ", "))
	}
	return total
}

func numericLeaves(v any, path string, out map[string]float64) {
	switch t := v.(type) {
	case float64:
		out[path] = t
	case map[string]any:
		for k, x := range t {
			numericLeaves(x, strings.TrimPrefix(path+"."+k, "."), out)
		}
	case []any:
		for i, x := range t {
			p := path
			if len(t) > 1 {
				p = fmt.Sprintf("%s[%d]", path, i)
			}
			numericLeaves(x, p, out)
		}
	}
}

// keysOf lists the field names of raw (or of its field at sub), never values.
func keysOf(raw json.RawMessage, sub string) []string {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return nil
	}
	if sub != "" {
		inner, ok := obj[sub]
		if !ok {
			return nil
		}
		obj = nil
		if json.Unmarshal(inner, &obj) != nil {
			return nil
		}
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

type spread struct {
	n                 int
	min, med, max, sd float64
}

func spreadOf(xs []float64) spread {
	if len(xs) == 0 {
		return spread{}
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	mean := 0.0
	for _, x := range s {
		mean += x
	}
	mean /= float64(len(s))
	v := 0.0
	for _, x := range s {
		v += (x - mean) * (x - mean)
	}
	return spread{n: len(s), min: s[0], med: s[len(s)/2], max: s[len(s)-1], sd: math.Sqrt(v / float64(len(s)))}
}

func (s spread) String() string {
	if s.n == 0 {
		return "n/a"
	}
	return fmt.Sprintf("min %.3f · median %.3f · max %.3f · sd %.3f", s.min, s.med, s.max, s.sd)
}

func popularities(ents []qloo.Entity) []float64 {
	out := make([]float64, len(ents))
	for i, e := range ents {
		out[i] = e.Popularity
	}
	return out
}

// percentiles reads population_percentile from each point of a /v2/trending
// result in its live shape, results.trending.
func percentiles(raw json.RawMessage) []float64 {
	var body struct {
		Trending []struct {
			Percentile *float64 `json:"population_percentile"`
		} `json:"trending"`
	}
	json.Unmarshal(raw, &body)
	var out []float64
	for _, pt := range body.Trending {
		if pt.Percentile != nil {
			out = append(out, *pt.Percentile)
		}
	}
	return out
}

func affinities(ents []qloo.Entity) []float64 {
	var out []float64
	for _, e := range ents {
		if a, ok := e.Affinity(); ok {
			out = append(out, a)
		}
	}
	return out
}

func names(ents []qloo.Entity, n int) string {
	if len(ents) == 0 {
		return "(none)"
	}
	var parts []string
	for i, e := range ents {
		if i == n {
			parts = append(parts, fmt.Sprintf("+%d more", len(ents)-n))
			break
		}
		parts = append(parts, fmt.Sprintf("%s (pop %.2f)", e.Name, e.Popularity))
	}
	return strings.Join(parts, ", ")
}

// better returns the more favourable of two levels.
func better(a, b string) string {
	rank := map[string]int{fail: 0, weak: 1, pass: 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func km(lat1, lon1, lat2, lon2 float64) float64 {
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 6371 * 2 * math.Asin(math.Sqrt(h))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
