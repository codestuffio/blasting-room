package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"blastingroom/internal/qloo"
	"blastingroom/internal/ticketmaster"
)

// tool is one capability exposed to Claude. run receives the raw JSON input
// and returns a value that is marshalled into the tool_result.
type tool struct {
	name        string
	description string
	properties  map[string]any
	required    []string
	strict      bool
	run         func(ctx context.Context, raw json.RawMessage) (any, error)
}

const memoToolName = "submit_booking_memo"

// toolset binds the Qloo tools, and the Ticketmaster show check, to one venue
// so the agent never has to pass coordinates or locality strings around
// itself. shows is nil when no Ticketmaster key is configured.
func toolset(q *qloo.Client, shows *ticketmaster.Client, v Venue) []tool {
	return []tool{
		{
			name:        "resolve_entity",
			description: "Resolve a name to Qloo entities (artist, place, podcast or brand). Returns entity_ids needed by the other tools.",
			properties: map[string]any{
				"name":        str("Name to search for, e.g. an artist name"),
				"entity_type": enum("Entity type", "artist", "place", "podcast", "brand"),
			},
			required: []string{"name", "entity_type"},
			run: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var in struct{ Name, EntityType string }
				if err := decode(raw, &in, map[string]*string{"name": &in.Name, "entity_type": &in.EntityType}); err != nil {
					return nil, err
				}
				ents, err := q.Search(ctx, in.Name, []string{"urn:entity:" + in.EntityType}, 5)
				if err != nil {
					return nil, err
				}
				return entitiesOrNote(ents, "Try a shorter or differently spelled name, or another entity_type."), nil
			},
		},
		{
			name:        "find_genre_tags",
			description: "Find Qloo tag URNs (genres, styles, keywords) matching free text such as 'post-punk' or 'alt-country'.",
			properties:  map[string]any{"query": str("Genre or style words")},
			required:    []string{"query"},
			run: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var in struct{ Query string }
				if err := decode(raw, &in, map[string]*string{"query": &in.Query}); err != nil {
					return nil, err
				}
				tags, err := q.FindTags(ctx, in.Query, nil, 10)
				if err != nil {
					return nil, err
				}
				out := make([]map[string]string, 0, len(tags))
				for _, t := range tags {
					out = append(out, map[string]string{"urn": t.URN(), "name": t.Name, "type": t.Type})
				}
				return out, nil
			},
		},
		{
			name: "discover_artists",
			description: "Find artists whose fans over-index around the venue and who share an audience with the seed artists. " +
				"Genre tags bias the results toward a genre; they are not a hard filter. " +
				"Use popularity_max to avoid acts too big for the room (0-1 percentile): mid-size touring acts sit around 0.80-0.93, so 0.9 suits a 300-700 cap room.",
			properties: map[string]any{
				"seed_ids":       arr(str("Artist entity_id the room already sells well, from resolve_entity (1-5)")),
				"genre_tags":     arr(str("Tag URN from find_genre_tags")),
				"popularity_max": map[string]any{"type": "number", "description": "Upper popularity percentile, 0-1"},
			},
			required: []string{"seed_ids"},
			run: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var in struct {
					SeedIDs       []string `json:"seed_ids"`
					GenreTags     []string `json:"genre_tags"`
					PopularityMax float64  `json:"popularity_max"`
				}
				if err := json.Unmarshal(raw, &in); err != nil || len(in.SeedIDs) == 0 {
					return nil, fmt.Errorf("seed_ids is required: resolve the room's favourite artists with resolve_entity first")
				}
				// Location alone returns city-wide superstars, and filter.tags returns
				// nothing for artists, so the seeds and tags go in as signals.
				p := url.Values{
					"filter.type":               {qloo.TypeArtist},
					"signal.interests.entities": {strings.Join(in.SeedIDs, ",")},
					"take":                      {"15"},
				}
				v.setLocationSignal(p)
				if len(in.GenreTags) > 0 {
					p.Set("signal.interests.tags", strings.Join(in.GenreTags, ","))
				}
				if in.PopularityMax > 0 && in.PopularityMax < 1 {
					p.Set("filter.popularity.max", fmt.Sprint(in.PopularityMax))
				}
				res, err := q.Insights(ctx, p)
				if err != nil {
					return nil, err
				}
				return entitiesOrNote(res.Entities, "Try fewer genre_tags, a higher popularity_max, or different seed_ids."), nil
			},
		},
		{
			name: "score_shortlist",
			description: "Rank specific candidate artists against the taste around the venue and the seed artists the room already sells well. " +
				"Returns affinity per candidate, Qloo's explanation of which seeds drove it (only when seed_ids are given), " +
				"and no_local_signal: candidates Qloo dropped because it has no local data on them.",
			properties: map[string]any{
				"candidate_ids": arr(str("Artist entity_id")),
				"seed_ids":      arr(str("Artist entity_id the room is known for (recommended)")),
			},
			required: []string{"candidate_ids"},
			run: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var in struct {
					CandidateIDs []string `json:"candidate_ids"`
					SeedIDs      []string `json:"seed_ids"`
				}
				if err := json.Unmarshal(raw, &in); err != nil || len(in.CandidateIDs) == 0 {
					return nil, fmt.Errorf("candidate_ids is required")
				}
				p := url.Values{
					"filter.type":             {qloo.TypeArtist},
					"filter.results.entities": {strings.Join(in.CandidateIDs, ",")},
					"feature.explainability":  {"true"},
				}
				v.setLocationSignal(p)
				if len(in.SeedIDs) > 0 {
					p.Set("signal.interests.entities", strings.Join(in.SeedIDs, ","))
				}
				res, err := q.Insights(ctx, p)
				if err != nil {
					return nil, err
				}
				if len(res.Entities) == 0 {
					return entitiesOrNote(nil, "Check the candidate_ids came from resolve_entity or discover_artists; drop seed_ids and retry."), nil
				}
				return withUnscored(compactEntities(res.Entities), in.CandidateIDs), nil
			},
		},
		{
			name: "local_heatmap",
			description: "Map where one artist's fan base over-indexes across the venue's city. " +
				"Returns the affinity of the cell nearest the venue, how it ranks among all cells, and the hottest cells.",
			properties: map[string]any{"entity_id": str("Artist entity_id")},
			required:   []string{"entity_id"},
			run: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var in struct{ EntityID string }
				if err := decode(raw, &in, map[string]*string{"entity_id": &in.EntityID}); err != nil {
					return nil, err
				}
				res, err := q.Insights(ctx, url.Values{
					"filter.type":               {qloo.TypeHeatmap},
					"signal.interests.entities": {in.EntityID},
					"filter.location.query":     {v.City},
				})
				if err != nil {
					return nil, err
				}
				return withGeo{summarizeHeatmap(res.Heatmap, v.Lat, v.Lon), heatGeo(v, in.EntityID, res.Heatmap)}, nil
			},
		},
		{
			name: "momentum",
			description: "Popularity trend over the last 6 months for up to 5 artists (percentile and velocity over time). " +
				"A series marked constant carries no momentum signal: Qloo often reports the same value at every point.",
			properties: map[string]any{"entity_ids": arr(str("Artist entity_id"))},
			required:   []string{"entity_ids"},
			run: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var in struct {
					EntityIDs []string `json:"entity_ids"`
				}
				if err := json.Unmarshal(raw, &in); err != nil || len(in.EntityIDs) == 0 {
					return nil, fmt.Errorf("entity_ids is required")
				}
				if len(in.EntityIDs) > 5 {
					in.EntityIDs = in.EntityIDs[:5]
				}
				// One request per artist: /v2/trending takes a single entity. Windows
				// much shorter than 6 months come back empty.
				end := time.Now()
				raws := make([]json.RawMessage, len(in.EntityIDs))
				errs := make([]error, len(in.EntityIDs))
				var wg sync.WaitGroup
				for i, id := range in.EntityIDs {
					wg.Add(1)
					go func() {
						defer wg.Done()
						raws[i], errs[i] = q.Trending(ctx, qloo.TypeArtist, id, end.AddDate(0, -6, 0), end)
					}()
				}
				wg.Wait()

				summaries := make([]any, 0, len(in.EntityIDs))
				trend := &Trend{}
				failed := 0
				for i, id := range in.EntityIDs {
					if errs[i] != nil {
						failed++
						summaries = append(summaries, map[string]any{"entity_id": id, "error": errs[i].Error()})
						continue
					}
					pts := parseTrend(raws[i])
					s := trendSummary(id, pts)
					if len(pts) == 0 && !isEmptyJSON(raws[i]) {
						s["unparsed"] = clip(raws[i], 1500) // a shape parseTrend does not know yet
					}
					summaries = append(summaries, s)
					if len(pts) > 0 {
						trend.Series = append(trend.Series, TrendSeries{EntityID: id, Points: pts})
					}
				}
				if failed == len(in.EntityIDs) {
					return nil, errs[0]
				}
				if len(trend.Series) == 0 {
					trend = nil
				}
				return withTrend{summaries, trend}, nil
			},
		},
		{
			name: "compare_audiences",
			description: "Compare the taste profiles of two groups of artists (e.g. a headliner vs. a candidate support act) to judge audience overlap. " +
				"Returns the strongest shared tags with scores, the tags only one side has, and shared entities.",
			properties: map[string]any{
				"group_a": arr(str("Artist entity_id")),
				"group_b": arr(str("Artist entity_id")),
			},
			required: []string{"group_a", "group_b"},
			run: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var in struct {
					GroupA []string `json:"group_a"`
					GroupB []string `json:"group_b"`
				}
				if err := json.Unmarshal(raw, &in); err != nil || len(in.GroupA) == 0 || len(in.GroupB) == 0 {
					return nil, fmt.Errorf("group_a and group_b are required")
				}
				res, err := q.Compare(ctx, in.GroupA, in.GroupB, url.Values{"take": {"15"}})
				if err != nil {
					return nil, err
				}
				return summarizeCompare(res), nil
			},
		},
		{
			name:        "audience_demographics",
			description: "Age and gender skew of an artist's fan base (positive = over-indexed). Use it for ticket pricing and ad targeting.",
			properties:  map[string]any{"entity_id": str("Artist entity_id")},
			required:    []string{"entity_id"},
			run: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var in struct{ EntityID string }
				if err := decode(raw, &in, map[string]*string{"entity_id": &in.EntityID}); err != nil {
					return nil, err
				}
				res, err := q.Insights(ctx, url.Values{
					"filter.type":               {qloo.TypeDemographics},
					"signal.interests.entities": {in.EntityID},
				})
				if err != nil {
					return nil, err
				}
				return clip(res.Demographics, 4000), nil
			},
		},
		{
			name: "co_promo_partners",
			description: "Cross-domain partners the artists' fans love: nearby bars, restaurants and record stores (place), " +
				"podcasts to advertise on (podcast), or brands to approach as sponsors (brand).",
			properties: map[string]any{
				"entity_ids": arr(str("Artist entity_id")),
				"kind":       enum("Partner category", "place", "podcast", "brand"),
			},
			required: []string{"entity_ids", "kind"},
			run: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var in struct {
					EntityIDs []string `json:"entity_ids"`
					Kind      string   `json:"kind"`
				}
				if err := json.Unmarshal(raw, &in); err != nil || len(in.EntityIDs) == 0 {
					return nil, fmt.Errorf("entity_ids is required")
				}
				p := url.Values{
					"filter.type":               {"urn:entity:" + in.Kind},
					"signal.interests.entities": {strings.Join(in.EntityIDs, ",")},
					"take":                      {"8"},
				}
				if in.Kind == "place" {
					p.Set("filter.location", v.Point())
					p.Set("filter.location.radius", "3000")
				}
				res, err := q.Insights(ctx, p)
				if err != nil {
					return nil, err
				}
				hint := "Try fewer or different entity_ids."
				if in.Kind != "place" {
					return entitiesOrNote(res.Entities, hint), nil
				}
				hint += " Nothing matched within 3 km of the venue."
				return withGeo{entitiesOrNote(res.Entities, hint), placesGeo(v, in.EntityIDs, res.Entities)}, nil
			},
		},
		competingShowsTool(shows, v),
		tourDatesTool(shows, v),
		priceDealTool(v),
		{
			name:        memoToolName,
			description: "Submit the final booking memo. Call exactly once, after the evidence is gathered.",
			properties:  memoProperties,
			strict:      true,
		},
	}
}

// decode unmarshals raw and checks that the named string fields are non-empty.
func decode(raw json.RawMessage, into any, required map[string]*string) error {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("invalid input JSON: %w", err)
	}
	for k, dst := range required {
		s, _ := m[k].(string)
		if s == "" {
			return fmt.Errorf("%s is required", k)
		}
		*dst = s
	}
	return nil
}

// compactEntities trims Qloo entities to what the model needs, keeping
// tool results small.
func compactEntities(ents []qloo.Entity) []map[string]any {
	out := make([]map[string]any, 0, len(ents))
	for _, e := range ents {
		m := map[string]any{"entity_id": e.EntityID, "name": e.Name, "popularity": round(e.Popularity)}
		if aff, ok := e.Affinity(); ok {
			m["affinity"] = round(aff)
		}
		if exp, ok := e.Query["explainability"]; ok {
			m["explainability"] = exp
		}
		var tags []string
		for _, t := range e.Tags {
			if len(tags) == 6 {
				break
			}
			tags = append(tags, t.Name)
		}
		if len(tags) > 0 {
			m["tags"] = tags
		}
		if e.Disambiguation != "" {
			m["disambiguation"] = e.Disambiguation
		}
		if addr, ok := e.Properties["address"]; ok {
			m["address"] = addr
		}
		out = append(out, m)
	}
	return out
}

// withUnscored lists the requested candidates Qloo left out of a ranking
// alongside the scored ones. With a location signal, Qloo silently drops
// artists it has no local data on, which the critic should know.
func withUnscored(scored []map[string]any, requested []string) any {
	got := map[string]bool{}
	for _, m := range scored {
		got[m["entity_id"].(string)] = true
	}
	var missing []string
	for _, id := range requested {
		if id = strings.TrimSpace(id); id != "" && !got[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return scored
	}
	return map[string]any{"results": scored, "no_local_signal": missing}
}

// compareTag is one tag in a /v2/analysis/compare result. Shared tags carry
// a score; each also repeats both groups' entities, which is most of the
// response's size and is dropped here.
type compareTag struct {
	Name    string `json:"name"`
	TagID   string `json:"tag_id"`
	Subtype string `json:"subtype"`
	Query   struct {
		Score *float64 `json:"score"`
	} `json:"query"`
}

// kind is the tag family from its URN, e.g. "genre" for urn:tag:genre:music:post_punk.
func (t compareTag) kind() string {
	urn := t.Subtype
	if urn == "" {
		urn = t.TagID
	}
	if parts := strings.Split(urn, ":"); len(parts) > 2 && parts[0] == "urn" && parts[1] == "tag" {
		return parts[2]
	}
	return ""
}

// Caps on what summarizeCompare keeps, so the result stays well under the
// size where a raw compare response used to be cut off.
const (
	compareSharedMax   = 15
	compareDistinctMax = 10
	compareEntitiesMax = 5
)

// summarizeCompare keeps what the model needs from a compare result: the
// strongest shared tags with their scores, the tags only one group has (in
// Qloo's order), and the shared entities. Qloo repeats a tag name across tag
// subtypes, so repeats are dropped. A result in another shape falls back to a
// clipped copy.
func summarizeCompare(raw json.RawMessage) any {
	var res struct {
		Tags          []compareTag `json:"tags"`
		A             []compareTag `json:"a"`
		B             []compareTag `json:"b"`
		MatchEntities []struct {
			Name string `json:"name"`
		} `json:"matchEntities"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return clip(raw, 6000)
	}
	if len(res.Tags)+len(res.A)+len(res.B)+len(res.MatchEntities) == 0 {
		return map[string]any{"note": "Qloo returned no comparison. Check both groups hold artist entity_ids from resolve_entity."}
	}

	shared := append([]compareTag(nil), res.Tags...)
	sort.SliceStable(shared, func(i, j int) bool { return tagScore(shared[i]) > tagScore(shared[j]) })
	sharedOut := []map[string]any{}
	seen := map[string]bool{}
	for _, t := range shared {
		if len(sharedOut) == compareSharedMax {
			break
		}
		if key := t.kind() + "/" + t.Name; seen[key] {
			continue
		} else {
			seen[key] = true
		}
		m := map[string]any{"name": t.Name, "kind": t.kind()}
		if t.Query.Score != nil {
			m["score"] = round(*t.Query.Score)
		}
		sharedOut = append(sharedOut, m)
	}
	distinct := func(tags []compareTag) []string {
		out := []string{}
		seen := map[string]bool{}
		for _, t := range tags {
			if len(out) == compareDistinctMax {
				break
			}
			if !seen[t.Name] {
				seen[t.Name] = true
				out = append(out, t.Name)
			}
		}
		return out
	}
	entities := []string{}
	for _, e := range res.MatchEntities[:min(compareEntitiesMax, len(res.MatchEntities))] {
		entities = append(entities, e.Name)
	}
	return map[string]any{
		"shared_tags":        sharedOut,
		"shared_tag_count":   len(res.Tags),
		"only_group_a":       distinct(res.A),
		"only_group_a_count": len(res.A),
		"only_group_b":       distinct(res.B),
		"only_group_b_count": len(res.B),
		"shared_entities":    entities,
	}
}

func tagScore(t compareTag) float64 {
	if t.Query.Score == nil {
		return 0
	}
	return *t.Query.Score
}

func summarizeHeatmap(pts []qloo.HeatmapPoint, lat, lon float64) map[string]any {
	if len(pts) == 0 {
		return map[string]any{"cells": 0, "note": "no heatmap data for this artist in this city"}
	}
	nearest, best := 0, math.MaxFloat64
	for i, p := range pts {
		d := math.Hypot(p.Location.Latitude-lat, p.Location.Longitude-lon)
		if d < best {
			nearest, best = i, d
		}
	}
	sorted := append([]qloo.HeatmapPoint(nil), pts...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Query.Affinity > sorted[j].Query.Affinity })
	venueAff := pts[nearest].Query.Affinity
	rank := 1
	for _, p := range sorted {
		if p.Query.Affinity > venueAff {
			rank++
		}
	}
	top := []map[string]any{}
	for _, p := range sorted[:min(5, len(sorted))] {
		top = append(top, map[string]any{"lat": p.Location.Latitude, "lon": p.Location.Longitude, "affinity": round(p.Query.Affinity)})
	}
	return map[string]any{
		"cells":              len(pts),
		"venue_cell":         map[string]any{"affinity": round(venueAff), "rank": rank, "distance_deg": round(best)},
		"venue_percentile":   round(1 - float64(rank-1)/float64(len(pts))),
		"hottest_cells":      top,
		"interpretation_tip": "venue_percentile near 1 means the fan base clusters around the venue",
	}
}

// entitiesOrNote returns compact entities, or a note with a hint when Qloo
// returned none: invalid or over-narrow parameters fail silently that way.
func entitiesOrNote(ents []qloo.Entity, hint string) any {
	if len(ents) == 0 {
		return map[string]any{"results": []any{}, "note": "Qloo returned no results. " + hint}
	}
	return compactEntities(ents)
}

func isEmptyJSON(raw json.RawMessage) bool {
	switch strings.TrimSpace(string(raw)) {
	case "", "null", "[]", "{}":
		return true
	}
	return false
}

func clip(raw json.RawMessage, n int) any {
	if isEmptyJSON(raw) {
		return map[string]any{"note": "empty result"}
	}
	if len(raw) <= n {
		return raw
	}
	return map[string]any{"truncated": true, "data": string(raw[:n])}
}

func round(f float64) float64 { return math.Round(f*1000) / 1000 }
