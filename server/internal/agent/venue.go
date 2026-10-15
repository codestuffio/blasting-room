package agent

// Venue is the room the agent is booking for. Demo venues use fictional names
// in real neighborhoods so nothing implies a partnership.
type Venue struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Neighborhood string  `json:"neighborhood"`
	City         string  `json:"city"`
	Lat          float64 `json:"lat"`
	Lon          float64 `json:"lon"`
	Capacity     int     `json:"capacity"`
	Vibe         string  `json:"vibe"`
}

// LocationQuery is the fuzzy locality string Qloo resolves (e.g. "Logan Square, Chicago").
func (v Venue) LocationQuery() string {
	return v.Neighborhood + ", " + v.City
}

var DemoVenues = []Venue{
	{ID: "lantern", Name: "The Lantern Room", Neighborhood: "Logan Square", City: "Chicago", Lat: 41.9231, Lon: -87.7093, Capacity: 450, Vibe: "indie, alt, post-punk; strong weeknight bar crowd"},
	{ID: "copperline", Name: "Copperline", Neighborhood: "East Austin", City: "Austin", Lat: 30.2622, Lon: -97.7223, Capacity: 700, Vibe: "alt-country, garage rock, psych; festival-season spillover"},
	{ID: "harbor", Name: "Harbor Hall", Neighborhood: "Ballard", City: "Seattle", Lat: 47.6687, Lon: -122.3846, Capacity: 350, Vibe: "dream pop, folk, shoegaze; older listening-room crowd"},
}

func FindVenue(id string) (Venue, bool) {
	for _, v := range DemoVenues {
		if v.ID == id {
			return v, true
		}
	}
	return Venue{}, false
}
