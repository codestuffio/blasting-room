package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"

	"blastingroom/internal/qloo"
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

// toolset binds the Qloo tools to one venue so the agent never has to pass
// coordinates or locality strings around itself.
func toolset(q *qloo.Client, v Venue) []tool {
	loc := v.LocationQuery()
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
				return compactEntities(ents), nil
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
			description: "Find artists whose fans over-index in the venue's neighborhood, optionally within genre tags. " +
				"Use popularity_max to avoid acts too big for the room (0-1 percentile) and trend_bias to favour rising acts.",
			properties: map[string]any{
				"genre_tags":     arr(str("Tag URN from find_genre_tags")),
				"popularity_max": map[string]any{"type": "number", "description": "Upper popularity percentile, 0-1"},
				"trend_bias":     enum("How much to favour trending acts", "off", "low", "medium", "high"),
			},
			run: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var in struct {
					GenreTags     []string `json:"genre_tags"`
					PopularityMax float64  `json:"popularity_max"`
					TrendBias     string   `json:"trend_bias"`
				}
				if err := json.Unmarshal(raw, &in); err != nil {
					return nil, err
				}
				p := url.Values{"filter.type": {qloo.TypeArtist}, "signal.location.query": {loc}, "take": {"15"}}
				if len(in.GenreTags) > 0 {
					p.Set("filter.tags", strings.Join(in.GenreTags, ","))
					p.Set("operator.filter.tags", "union")
				}
				if in.PopularityMax > 0 && in.PopularityMax < 1 {
					p.Set("filter.popularity.max", fmt.Sprint(in.PopularityMax))
				}
				if in.TrendBias != "" {
					p.Set("bias.trends", in.TrendBias)
				}
				res, err := q.Insights(ctx, p)
				if err != nil {
					return nil, err
				}
				return compactEntities(res.Entities), nil
			},
		},
		{
			name: "score_shortlist",
			description: "Rank specific candidate artists against the venue neighborhood's taste (and optional seed artists the room already sells well). " +
				"Returns affinity per candidate plus Qloo's explanation of which signals drove it.",
			properties: map[string]any{
				"candidate_ids": arr(str("Artist entity_id")),
				"seed_ids":      arr(str("Artist entity_id the room is known for (optional)")),
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
					"signal.location.query":   {loc},
					"feature.explainability":  {"true"},
				}
				if len(in.SeedIDs) > 0 {
					p.Set("signal.interests.entities", strings.Join(in.SeedIDs, ","))
				}
				res, err := q.Insights(ctx, p)
				if err != nil {
					return nil, err
				}
				return compactEntities(res.Entities), nil
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
				return summarizeHeatmap(res.Heatmap, v.Lat, v.Lon), nil
			},
		},
		{
			name:        "momentum",
			description: "Popularity trend over the last 90 days for up to 5 artists (percentile and velocity over time).",
			properties:  map[string]any{"entity_ids": arr(str("Artist entity_id"))},
			required:    []string{"entity_ids"},
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
				end := time.Now()
				res, err := q.Trending(ctx, qloo.TypeArtist, in.EntityIDs, end.AddDate(0, 0, -90), end)
				if err != nil {
					return nil, err
				}
				return clip(res, 6000), nil
			},
		},
		{
			name:        "compare_audiences",
			description: "Compare the taste profiles of two groups of artists (e.g. a headliner vs. a candidate support act) to judge audience overlap.",
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
				return clip(res, 6000), nil
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
					p.Set("filter.location", fmt.Sprintf("POINT(%f %f)", v.Lon, v.Lat))
					p.Set("filter.location.radius", "3000")
				}
				res, err := q.Insights(ctx, p)
				if err != nil {
					return nil, err
				}
				return compactEntities(res.Entities), nil
			},
		},
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

func clip(raw json.RawMessage, n int) any {
	if len(raw) == 0 {
		return map[string]any{"note": "empty result"}
	}
	if len(raw) <= n {
		return raw
	}
	return map[string]any{"truncated": true, "data": string(raw[:n])}
}

func round(f float64) float64 { return math.Round(f*1000) / 1000 }
