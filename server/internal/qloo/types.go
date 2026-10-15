package qloo

import "encoding/json"

// Entity types used by Blasting Room.
const (
	TypeArtist  = "urn:entity:artist"
	TypePlace   = "urn:entity:place"
	TypePodcast = "urn:entity:podcast"
	TypeBrand   = "urn:entity:brand"

	TypeHeatmap      = "urn:heatmap"
	TypeDemographics = "urn:demographics"
	TypeTag          = "urn:tag"
)

// Entity is the common entity shape. Field coverage varies by type, so
// type-specific detail stays in the loosely typed Properties and Query maps.
type Entity struct {
	EntityID       string         `json:"entity_id"`
	Name           string         `json:"name"`
	Type           string         `json:"type,omitempty"`
	Types          []string       `json:"types,omitempty"`
	Subtype        string         `json:"subtype,omitempty"`
	Popularity     float64        `json:"popularity,omitempty"`
	Disambiguation string         `json:"disambiguation,omitempty"`
	Tags           []Tag          `json:"tags,omitempty"`
	Properties     map[string]any `json:"properties,omitempty"`
	Query          map[string]any `json:"query,omitempty"`
}

// Affinity returns query.affinity when present.
func (e Entity) Affinity() (float64, bool) {
	v, ok := e.Query["affinity"].(float64)
	return v, ok
}

// Tag appears both on entities ("id") and in /v2/tags results ("tag_id").
type Tag struct {
	ID      string `json:"id,omitempty"`
	TagID   string `json:"tag_id,omitempty"`
	Name    string `json:"name"`
	Type    string `json:"type,omitempty"`
	Subtype string `json:"subtype,omitempty"`
}

func (t Tag) URN() string {
	if t.TagID != "" {
		return t.TagID
	}
	return t.ID
}

type HeatmapPoint struct {
	Location struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Geohash   string  `json:"geohash"`
	} `json:"location"`
	Query struct {
		Affinity     float64 `json:"affinity"`
		AffinityRank float64 `json:"affinity_rank"`
		Popularity   float64 `json:"popularity"`
	} `json:"query"`
}

// InsightsResults holds whichever result list the request's filter.type
// produces. Demographics stays raw because its shape is not yet verified
// against a live key.
type InsightsResults struct {
	Entities     []Entity        `json:"entities,omitempty"`
	Tags         []Tag           `json:"tags,omitempty"`
	Heatmap      []HeatmapPoint  `json:"heatmap,omitempty"`
	Demographics json.RawMessage `json:"demographics,omitempty"`
}
