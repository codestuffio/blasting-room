package agent

import "blastingroom/internal/qloo"

// Geo is streamed to the UI as a "geo" event so it can draw the map. It never
// goes to the model, whose tool results stay compact summaries.
type Geo struct {
	ToolCallID string     `json:"tool_call_id"`
	Kind       string     `json:"kind"` // heatmap | partners
	EntityIDs  []string   `json:"entity_ids"`
	Venue      GeoPoint   `json:"venue"`
	Cells      []HeatCell `json:"cells,omitempty"`
	Places     []GeoPlace `json:"places,omitempty"`
}

type GeoPoint struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

type HeatCell struct {
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	Affinity float64 `json:"affinity"`
}

type GeoPlace struct {
	EntityID string  `json:"entity_id"`
	Name     string  `json:"name"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	Address  string  `json:"address,omitempty"`
}

// withGeo is what a tool returns when it also has map data for the UI.
// runTools sends model to Claude and emits geo as its own event.
type withGeo struct {
	model any
	geo   *Geo
}

func venuePoint(v Venue) GeoPoint { return GeoPoint{Name: v.Name, Lat: v.Lat, Lon: v.Lon} }

func heatGeo(v Venue, entityID string, pts []qloo.HeatmapPoint) *Geo {
	if len(pts) == 0 {
		return nil
	}
	cells := make([]HeatCell, 0, len(pts))
	for _, p := range pts {
		cells = append(cells, HeatCell{Lat: p.Location.Latitude, Lon: p.Location.Longitude, Affinity: round(p.Query.Affinity)})
	}
	return &Geo{Kind: "heatmap", EntityIDs: []string{entityID}, Venue: venuePoint(v), Cells: cells}
}

// placesGeo keeps only places Qloo returned coordinates for.
func placesGeo(v Venue, entityIDs []string, ents []qloo.Entity) *Geo {
	var places []GeoPlace
	for _, e := range ents {
		lat, lon, ok := e.Coordinates()
		if !ok {
			continue
		}
		addr, _ := e.Properties["address"].(string)
		places = append(places, GeoPlace{EntityID: e.EntityID, Name: e.Name, Lat: lat, Lon: lon, Address: addr})
	}
	if len(places) == 0 {
		return nil
	}
	return &Geo{Kind: "partners", EntityIDs: entityIDs, Venue: venuePoint(v), Places: places}
}
