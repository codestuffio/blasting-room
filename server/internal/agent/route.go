package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"blastingroom/internal/ticketmaster"
)

// Routing rules of thumb for a touring club act.
const (
	driveMiles  = 450 // the most a band drives between shows on consecutive days
	marketMiles = 100 // a typical radius clause...
	marketDays  = 60  // ...and how long it lasts either side of a show
	stopsShown  = 30  // stops the model reads, nearest the booking date first
)

// Route is streamed to the UI as a "route" event for the tour chart. Like
// Geo it never goes to the model, which gets the routeSummary instead.
type Route struct {
	ToolCallID string        `json:"tool_call_id"`
	Artist     string        `json:"artist"`
	EntityID   string        `json:"entity_id,omitempty"` // Qloo id, to match the memo's picks
	Venue      GeoPoint      `json:"venue"`
	Date       string        `json:"date,omitempty"`
	Fit        string        `json:"fit"`
	Stops      []RouteStop   `json:"stops"`
	Windows    []RouteWindow `json:"windows"`
}

type RouteStop struct {
	Date   string  `json:"date"`
	Venue  string  `json:"venue"`
	City   string  `json:"city"`
	Region string  `json:"region,omitempty"`
	Miles  float64 `json:"miles"` // from the booking venue
}

// RouteWindow is a run of open nights the act could play the venue: between
// two nearby stops, or just before or after the listed dates.
type RouteWindow struct {
	From   string  `json:"from"`
	To     string  `json:"to"`
	After  string  `json:"after,omitempty"`  // city of the stop before
	Before string  `json:"before,omitempty"` // city of the stop after
	Detour float64 `json:"detour_miles"`
}

// withRoute is what tour_dates returns: model goes to Claude and route is
// emitted as its own event.
type withRoute struct {
	model any
	route *Route
}

type stop struct {
	ticketmaster.Show
	day   time.Time
	miles float64
}

// tourDatesTool looks up an act's upcoming Ticketmaster dates and judges how
// the booking date fits its routing.
func tourDatesTool(shows *ticketmaster.Client, v Venue) tool {
	return tool{
		name: "tour_dates",
		description: "Look up an act's upcoming tour dates (Ticketmaster) and see how the booking date fits: where they play the nights before and after, " +
			fmt.Sprintf("open nights between nearby stops, and shows within %d miles in the %d days around the date (a radius clause may block it). ", marketMiles, marketDays) +
			"Also returns the ticket prices they charge elsewhere. Indie tours often sell through other ticketers, so a short list is not proof of a light schedule.",
		properties: map[string]any{
			"artist_name": str("Act name exactly as Qloo returned it"),
			"entity_id":   str("The act's Qloo entity_id, so the memo can chart its tour"),
			"date":        str("Booking date, YYYY-MM-DD; omit when there is none"),
		},
		required: []string{"artist_name"},
		run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				ArtistName string `json:"artist_name"`
				EntityID   string `json:"entity_id"`
				Date       string `json:"date"`
			}
			if err := json.Unmarshal(raw, &in); err != nil || strings.TrimSpace(in.ArtistName) == "" {
				return nil, fmt.Errorf("artist_name is required")
			}
			var day time.Time
			if in.Date != "" {
				d, err := time.Parse(time.DateOnly, strings.TrimSpace(in.Date))
				if err != nil {
					return nil, fmt.Errorf("date must be YYYY-MM-DD, got %q", in.Date)
				}
				day = d
			}
			if shows == nil {
				return map[string]any{"checked": false, "note": "Tour data (Ticketmaster) is not configured on this server. Say routing was not checked."}, nil
			}
			a, err := shows.FindArtist(ctx, in.ArtistName)
			if err != nil {
				return nil, err
			}
			if a == nil {
				return map[string]any{"checked": true, "found": false, "fit": "no_tour_data",
					"note": "Ticketmaster has no artist listing with that exact name. Many indie acts sell through other ticketers, so this says nothing about their schedule."}, nil
			}
			res, err := shows.ArtistShows(ctx, a.ID)
			if err != nil {
				return nil, err
			}
			stops := tourStops(v, res.Shows)
			fit, reason := routeFit(stops, day)
			windows := routeWindows(stops, time.Now())
			route := &Route{Artist: a.Name, EntityID: in.EntityID, Venue: venuePoint(v), Date: in.Date, Fit: fit, Stops: make([]RouteStop, 0, len(stops)), Windows: windows}
			for _, s := range stops {
				route.Stops = append(route.Stops, RouteStop{Date: s.Date, Venue: s.Venue, City: s.City, Region: s.Region, Miles: math.Round(s.miles)})
			}
			return withRoute{routeSummary(a, res.Shows, stops, windows, day, fit, reason), route}, nil
		},
	}
}

// tourStops keeps the dated shows with coordinates, in date order, with
// their distance from the venue.
func tourStops(v Venue, shows []ticketmaster.Show) []stop {
	var out []stop
	for _, s := range shows {
		d, err := time.Parse(time.DateOnly, s.Date)
		if err != nil || (s.Lat == 0 && s.Lon == 0) {
			continue
		}
		out = append(out, stop{Show: s, day: d, miles: milesBetween(v.Lat, v.Lon, s.Lat, s.Lon)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].day.Before(out[j].day) })
	return out
}

// routeFit judges a booking day against the tour: on_the_way, detour,
// off_route, already_booked (playing elsewhere that night),
// already_in_market (a nearby show close enough in time to trip a radius
// clause), no_tour_data, or not_checked when there is no date.
func routeFit(stops []stop, day time.Time) (fit, reason string) {
	if len(stops) == 0 {
		return "no_tour_data", "no upcoming dates with a venue location are listed"
	}
	if day.IsZero() {
		return "not_checked", "no booking date to check against"
	}
	for _, s := range stops {
		if s.day.Equal(day) {
			return "already_booked", fmt.Sprintf("they play %s (%s) that night, %.0f mi away", place(s), s.Venue, s.miles)
		}
	}
	for _, s := range stops {
		if s.miles <= marketMiles && math.Abs(days(day, s.day)) <= marketDays {
			return "already_in_market", fmt.Sprintf("they play %s, %.0f mi away, on %s; a radius clause may block another date this close", s.Venue, s.miles, s.Date)
		}
	}
	var prev, next *stop
	for i := range stops {
		if stops[i].day.Before(day) {
			prev = &stops[i]
		} else if next == nil {
			next = &stops[i]
		}
	}
	near := func(s *stop) bool { return s != nil && s.miles <= driveMiles && math.Abs(days(day, s.day)) <= 3 }
	reachable := func(s *stop) bool { return s != nil && s.miles <= 2*driveMiles && math.Abs(days(day, s.day)) <= 7 }
	switch {
	case prev != nil && next != nil && near(prev) && near(next):
		detour := prev.miles + next.miles - milesBetween(prev.Lat, prev.Lon, next.Lat, next.Lon)
		if detour <= 250 {
			return "on_the_way", fmt.Sprintf("between %s on %s (%.0f mi) and %s on %s (%.0f mi), a %.0f mi detour", place(*prev), prev.Date, prev.miles, place(*next), next.Date, next.miles, detour)
		}
		return "detour", fmt.Sprintf("between %s and %s, but a %.0f mi detour", place(*prev), place(*next), detour)
	case near(prev) && next == nil:
		return "on_the_way", fmt.Sprintf("extends the run after %s on %s, %.0f mi away", place(*prev), prev.Date, prev.miles)
	case near(next) && prev == nil:
		return "on_the_way", fmt.Sprintf("opens the run before %s on %s, %.0f mi away", place(*next), next.Date, next.miles)
	case near(prev) || near(next) || reachable(prev) || reachable(next):
		parts := []string{}
		for _, s := range []*stop{prev, next} {
			if s != nil {
				parts = append(parts, fmt.Sprintf("%s on %s (%.0f mi)", place(*s), s.Date, s.miles))
			}
		}
		return "detour", "nearest dates: " + strings.Join(parts, " and ")
	default:
		return "off_route", "no listed date within a week and a long day's drive of the venue"
	}
}

// routeWindows finds open nights the act could add the venue: between two
// stops within a day's drive, or just before or after the listed run (the
// nights before only when they are still ahead of today).
func routeWindows(stops []stop, today time.Time) []RouteWindow {
	var out []RouteWindow
	for i := 0; i+1 < len(stops); i++ {
		a, b := stops[i], stops[i+1]
		if days(b.day, a.day) < 2 || a.miles > driveMiles || b.miles > driveMiles {
			continue
		}
		detour := a.miles + b.miles - milesBetween(a.Lat, a.Lon, b.Lat, b.Lon)
		out = append(out, RouteWindow{From: a.day.AddDate(0, 0, 1).Format(time.DateOnly), To: b.day.AddDate(0, 0, -1).Format(time.DateOnly),
			After: place(a), Before: place(b), Detour: math.Round(detour)})
	}
	if n := len(stops); n > 0 {
		if last := stops[n-1]; last.miles <= driveMiles {
			out = append(out, RouteWindow{From: last.day.AddDate(0, 0, 1).Format(time.DateOnly), To: last.day.AddDate(0, 0, 2).Format(time.DateOnly),
				After: place(last), Detour: math.Round(last.miles)})
		}
		if first := stops[0]; first.miles <= driveMiles && first.day.After(today.AddDate(0, 0, 2)) {
			out = append([]RouteWindow{{From: first.day.AddDate(0, 0, -2).Format(time.DateOnly), To: first.day.AddDate(0, 0, -1).Format(time.DateOnly),
				Before: place(first), Detour: math.Round(first.miles)}}, out...)
		}
	}
	return out
}

func routeSummary(a *ticketmaster.Attraction, shows []ticketmaster.Show, stops []stop, windows []RouteWindow, day time.Time, fit, reason string) map[string]any {
	out := map[string]any{"checked": true, "found": true, "artist": a.Name, "listed_dates": len(stops), "fit": fit, "fit_reason": reason}
	if a.Upcoming > len(stops) {
		out["note"] = fmt.Sprintf("Ticketmaster counts %d upcoming events but lists %d with a location; the tour is likely longer than this.", a.Upcoming, len(stops))
	} else if len(stops) < 5 {
		out["note"] = "Few dates listed. Indie tours often sell through other ticketers, so the tour is likely longer than this."
	}
	// The stops nearest the booking date matter most; without a date, the soonest.
	near := append([]stop(nil), stops...)
	if !day.IsZero() {
		sort.SliceStable(near, func(i, j int) bool { return math.Abs(days(near[i].day, day)) < math.Abs(days(near[j].day, day)) })
	}
	near = near[:min(len(near), stopsShown)]
	sort.SliceStable(near, func(i, j int) bool { return near[i].day.Before(near[j].day) })
	list := make([]map[string]any, 0, len(near))
	for _, s := range near {
		list = append(list, map[string]any{"date": s.Date, "city": place(s), "venue": s.Venue, "miles": math.Round(s.miles)})
	}
	out["stops"] = list
	var inMarket []map[string]any
	for _, s := range stops {
		if s.miles <= marketMiles {
			inMarket = append(inMarket, map[string]any{"date": s.Date, "venue": s.Venue, "miles": math.Round(s.miles)})
		}
	}
	if len(inMarket) > 0 {
		out["in_market"] = inMarket
	}
	if len(windows) > 0 {
		out["open_windows"] = windows
	}
	if p := priceRange(shows); p != "" {
		out["ticket_prices_elsewhere"] = p
	}
	return out
}

// priceRange spans the face-value ranges Ticketmaster lists for the shows.
func priceRange(shows []ticketmaster.Show) string {
	lo, hi, cur := math.Inf(1), 0.0, ""
	for _, s := range shows {
		if s.PriceMax <= 0 {
			continue
		}
		lo, hi, cur = math.Min(lo, s.PriceMin), math.Max(hi, s.PriceMax), s.Currency
	}
	if hi == 0 {
		return ""
	}
	return fmt.Sprintf("%.0f-%.0f %s", lo, hi, cur)
}

func place(s stop) string {
	if s.Region != "" {
		return s.City + ", " + s.Region
	}
	return s.City
}

// days is a - b in days.
func days(a, b time.Time) float64 { return a.Sub(b).Hours() / 24 }

// milesBetween is the great-circle distance between two points.
func milesBetween(lat1, lon1, lat2, lon2 float64) float64 {
	const earthMiles = 3958.8
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthMiles * math.Asin(math.Sqrt(h))
}
