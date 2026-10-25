package agent

import (
	"fmt"
	"net/url"
)

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
	// Seeds are artists the room already sells well. Qloo's artist discovery
	// needs a taste signal besides location, or it returns city-wide superstars.
	Seeds []string `json:"seeds"`
}

// LocationQuery is the fuzzy locality string Qloo resolves (e.g. "Logan Square, Chicago").
// Only some neighborhoods resolve ("Ballard, Seattle" silently returns
// nothing), so the artist tools signal the venue's point instead.
func (v Venue) LocationQuery() string {
	return v.Neighborhood + ", " + v.City
}

// Point is the venue as a WKT point, longitude first, as Qloo expects.
func (v Venue) Point() string {
	return fmt.Sprintf("POINT(%f %f)", v.Lon, v.Lat)
}

// signalRadius is how far around the venue the location signal reaches, in meters.
const signalRadius = "5000"

// setLocationSignal points p's location signal at the area around the venue.
func (v Venue) setLocationSignal(p url.Values) {
	p.Set("signal.location", v.Point())
	p.Set("signal.location.radius", signalRadius)
}

var DemoVenues = []Venue{
	{ID: "lantern", Name: "The Lantern Room", Neighborhood: "Logan Square", City: "Chicago", Lat: 41.9231, Lon: -87.7093, Capacity: 450, Vibe: "indie, alt, post-punk; strong weeknight bar crowd", Seeds: []string{"Horsegirl", "Wednesday", "Dehd"}},
	{ID: "copperline", Name: "Copperline", Neighborhood: "East Austin", City: "Austin", Lat: 30.2622, Lon: -97.7223, Capacity: 700, Vibe: "alt-country, garage rock, psych; festival-season spillover", Seeds: []string{"MJ Lenderman", "Ty Segall", "Night Beats"}},
	{ID: "harbor", Name: "Harbor Hall", Neighborhood: "Ballard", City: "Seattle", Lat: 47.6687, Lon: -122.3846, Capacity: 350, Vibe: "dream pop, folk, shoegaze; older listening-room crowd", Seeds: []string{"Beach House", "Fleet Foxes", "Slowdive"}},
}

func FindVenue(id string) (Venue, bool) {
	for _, v := range DemoVenues {
		if v.ID == id {
			return v, true
		}
	}
	return Venue{}, false
}
