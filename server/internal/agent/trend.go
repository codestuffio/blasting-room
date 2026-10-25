package agent

import (
	"encoding/json"
	"math"
	"sort"
)

// Trend is streamed to the UI as a "trend" event so it can draw the momentum
// chart. Like Geo it never goes to the model, which gets trendSummary instead.
type Trend struct {
	ToolCallID string        `json:"tool_call_id"`
	Series     []TrendSeries `json:"series"`
}

type TrendSeries struct {
	EntityID string       `json:"entity_id"`
	Points   []TrendPoint `json:"points"`
}

// TrendPoint is one sample: the popularity percentile (0-1) on a date, and
// the velocity (fold change) when Qloo reports one.
type TrendPoint struct {
	Date       string   `json:"date"`
	Percentile float64  `json:"percentile"`
	Velocity   *float64 `json:"velocity,omitempty"`
}

// withTrend is what the momentum tool returns: model goes to Claude and trend
// is emitted as its own event.
type withTrend struct {
	model any
	trend *Trend
}

// The /v2/trending point shape is not yet verified against a live key, so
// parseTrend accepts the field names seen in Qloo's docs, in order of preference.
var (
	trendDateKeys     = []string{"date", "day", "week", "start_date"}
	trendPercentKeys  = []string{"population_percentile", "percentile", "popularity"}
	trendVelocityKeys = []string{"velocity_fold_change", "population_rank_velocity", "velocity"}
)

// parseTrend finds the time series in one /v2/trending result: the result
// itself when it is an array, otherwise its first array field. Points come
// back sorted by date, oldest first.
func parseTrend(raw json.RawMessage) []TrendPoint {
	var pts []map[string]any
	if json.Unmarshal(raw, &pts) != nil {
		var obj map[string]json.RawMessage
		if json.Unmarshal(raw, &obj) != nil {
			return nil
		}
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if json.Unmarshal(obj[k], &pts) == nil && len(pts) > 0 {
				break
			}
		}
	}
	out := make([]TrendPoint, 0, len(pts))
	for _, p := range pts {
		date, _ := firstString(p, trendDateKeys)
		pct, ok := firstNumber(p, trendPercentKeys)
		if date == "" || !ok {
			continue
		}
		tp := TrendPoint{Date: date, Percentile: round(pct)}
		if v, ok := firstNumber(p, trendVelocityKeys); ok {
			v = round(v)
			tp.Velocity = &v
		}
		out = append(out, tp)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

func firstString(m map[string]any, keys []string) (string, bool) {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s, true
		}
	}
	return "", false
}

func firstNumber(m map[string]any, keys []string) (float64, bool) {
	for _, k := range keys {
		if f, ok := m[k].(float64); ok && !math.IsNaN(f) {
			return f, true
		}
	}
	return 0, false
}

// trendSummary is what the model sees for one artist's series: where it
// started and ended, and which way it is heading.
func trendSummary(entityID string, pts []TrendPoint) map[string]any {
	if len(pts) == 0 {
		return map[string]any{"entity_id": entityID, "note": "no trending data for this artist in the last 90 days"}
	}
	first, last := pts[0], pts[len(pts)-1]
	peak := first
	for _, p := range pts {
		if p.Percentile > peak.Percentile {
			peak = p
		}
	}
	change := round(last.Percentile - first.Percentile)
	direction := "flat"
	switch {
	case change >= 0.03:
		direction = "rising"
	case change <= -0.03:
		direction = "falling"
	}
	m := map[string]any{
		"entity_id":        entityID,
		"points":           len(pts),
		"from":             first.Date,
		"to":               last.Date,
		"start_percentile": first.Percentile,
		"end_percentile":   last.Percentile,
		"change":           change,
		"peak_percentile":  peak.Percentile,
		"peak_date":        peak.Date,
		"direction":        direction,
	}
	if last.Velocity != nil {
		m["latest_velocity"] = *last.Velocity
	}
	return m
}
