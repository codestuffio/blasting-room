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

// showRadiusMiles is how far from the venue a show counts as competition.
const showRadiusMiles = 5

// showsForModel caps how many shows the model reads; the rest are counted.
const showsForModel = 25

// competingShowsTool looks up ticketed music shows near the venue around the
// booking date. With no Ticketmaster key it says the check was skipped, so
// the agent names the gap instead of implying a quiet night.
func competingShowsTool(shows *ticketmaster.Client, v Venue) tool {
	return tool{
		name: "competing_shows",
		description: fmt.Sprintf("List ticketed music shows within %d miles of the venue on the booking date and the nights around it (Ticketmaster). ", showRadiusMiles) +
			"Use it to spot shows that would pull the same crowd the same night. Ticketmaster lists ticketed shows only, so DIY and free bar shows won't appear.",
		properties: map[string]any{
			"date":               str("Booking date, YYYY-MM-DD"),
			"nights_either_side": map[string]any{"type": "integer", "description": "Also check this many nights before and after (0-3, default 1)"},
		},
		required: []string{"date"},
		run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Date             string `json:"date"`
				NightsEitherSide *int   `json:"nights_either_side"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, fmt.Errorf("invalid input JSON: %w", err)
			}
			day, err := time.Parse(time.DateOnly, strings.TrimSpace(in.Date))
			if err != nil {
				return nil, fmt.Errorf("date must be YYYY-MM-DD, got %q", in.Date)
			}
			if shows == nil {
				return map[string]any{
					"checked": false,
					"note":    "Competing-show data (Ticketmaster) is not configured on this server. Say in risks that nearby shows were not checked.",
				}, nil
			}
			n := 1
			if in.NightsEitherSide != nil {
				n = min(max(*in.NightsEitherSide, 0), 3)
			}
			from, to := day.AddDate(0, 0, -n).Format(time.DateOnly), day.AddDate(0, 0, n).Format(time.DateOnly)
			res, err := shows.MusicShows(ctx, ticketmaster.Search{Lat: v.Lat, Lon: v.Lon, RadiusMiles: showRadiusMiles, From: from, To: to})
			if err != nil {
				return nil, err
			}
			return withGeo{summarizeShows(res, from, to, in.Date), showsGeo(v, res.Shows)}, nil
		},
	}
}

// summarizeShows is what the model reads: the shows nearest the booking
// date first, each trimmed to what bears on whether it competes.
func summarizeShows(res *ticketmaster.Results, from, to, date string) map[string]any {
	out := map[string]any{"checked": true, "window": from + " to " + to, "radius_miles": showRadiusMiles, "total": res.Total}
	shows := append([]ticketmaster.Show(nil), res.Shows...)
	sort.SliceStable(shows, func(i, j int) bool {
		si, sj := shows[i].Date == date, shows[j].Date == date
		if si != sj {
			return si
		}
		if shows[i].Date != shows[j].Date {
			return shows[i].Date < shows[j].Date
		}
		return shows[i].DistanceMiles < shows[j].DistanceMiles
	})
	list := make([]map[string]any, 0, min(len(shows), showsForModel))
	for _, s := range shows[:min(len(shows), showsForModel)] {
		m := map[string]any{"name": s.Name, "date": s.Date, "venue": s.Venue, "distance_mi": math.Round(s.DistanceMiles*10) / 10}
		if s.Time != "" {
			m["time"] = strings.TrimSuffix(s.Time, ":00")
		}
		if s.Status != "" && s.Status != "onsale" {
			m["status"] = s.Status
		}
		if g := strings.Trim(s.Genre+" / "+s.Subgenre, " /"); g != "" {
			m["genre"] = g
		}
		if len(s.Acts) > 0 {
			m["acts"] = s.Acts[:min(len(s.Acts), 4)]
		}
		if s.PriceMax > 0 {
			m["price"] = fmt.Sprintf("%.0f-%.0f %s", s.PriceMin, s.PriceMax, s.Currency)
		}
		list = append(list, m)
	}
	out["shows"] = list
	switch {
	case len(shows) == 0:
		out["note"] = fmt.Sprintf("No ticketed music shows within %d miles in this window. Ticketmaster lists ticketed shows only, so this is not proof of a quiet night.", showRadiusMiles)
	case res.Total > len(list):
		out["note"] = fmt.Sprintf("Showing %d of %d shows, booking date first.", len(list), res.Total)
	}
	return out
}

// showsGeo pins each venue with shows once, listing its shows as the detail.
func showsGeo(v Venue, shows []ticketmaster.Show) *Geo {
	byVenue := map[string]*GeoPlace{}
	var order []string
	for _, s := range shows {
		if s.Lat == 0 && s.Lon == 0 {
			continue
		}
		key := s.VenueID
		if key == "" {
			key = s.Venue
		}
		p, ok := byVenue[key]
		if !ok {
			p = &GeoPlace{EntityID: "tm:" + key, Name: s.Venue, Lat: s.Lat, Lon: s.Lon}
			byVenue[key] = p
			order = append(order, key)
		}
		line := s.Name + " (" + s.Date + ")"
		if p.Address == "" {
			p.Address = line
		} else {
			p.Address += "; " + line
		}
	}
	if len(order) == 0 {
		return nil
	}
	places := make([]GeoPlace, 0, len(order))
	for _, k := range order {
		places = append(places, *byVenue[k])
	}
	return &Geo{Kind: "shows", Venue: venuePoint(v), Places: places}
}
