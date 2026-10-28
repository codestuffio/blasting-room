package ticketmaster

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A synthetic response in the Discovery API's shape. Names are made up.
const eventsBody = `{
  "_embedded": {"events": [
    {"id": "e2", "name": "Mock Show 02",
     "dates": {"start": {"localDate": "2026-11-13", "localTime": "20:00:00"}, "status": {"code": "onsale"}},
     "classifications": [{"primary": true, "genre": {"name": "Rock"}, "subGenre": {"name": "Indie Rock"}}],
     "_embedded": {"venues": [{"id": "v2", "name": "Mock Hall", "distance": 2.4,
        "address": {"line1": "2 Mock Ave"}, "location": {"latitude": "41.93", "longitude": "-87.70"}}],
        "attractions": [{"name": "Mock Band A"}, {"name": "Mock Band B"}]}},
    {"id": "e1", "name": "Mock Show 01",
     "dates": {"start": {"localDate": "2026-11-12"}, "status": {"code": "onsale"}},
     "classifications": [{"primary": true, "genre": {"name": "Undefined"}, "subGenre": {"name": "Undefined"}}],
     "priceRanges": [{"min": 18, "max": 25, "currency": "USD"}],
     "_embedded": {"venues": [{"id": "v1", "name": "Mock Club", "distance": 0.6,
        "location": {"latitude": "41.92", "longitude": "-87.71"}}]}},
    {"id": "e9", "name": "Mock Test Listing", "test": true,
     "dates": {"start": {"localDate": "2026-11-12"}}}
  ]},
  "page": {"totalElements": 2}
}`

func TestMusicShowsQueryAndParse(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		io.WriteString(w, eventsBody)
	}))
	defer srv.Close()

	c := New("k3y", WithBaseURL(srv.URL))
	res, err := c.MusicShows(context.Background(), Search{Lat: 41.9231, Lon: -87.7093, RadiusMiles: 5, From: "2026-11-12", To: "2026-11-13"})
	if err != nil {
		t.Fatal(err)
	}
	q := got.URL.Query()
	if got.URL.Path != "/events.json" || q.Get("apikey") != "k3y" || q.Get("segmentName") != "Music" || q.Get("radius") != "5" ||
		q.Get("unit") != "miles" || q.Get("localStartDateTime") != "2026-11-12T00:00:00,2026-11-13T23:59:59" || len(q.Get("geoPoint")) != 9 {
		t.Fatalf("request = %s", got.URL)
	}
	if res.Total != 2 || len(res.Shows) != 2 {
		t.Fatalf("results = %+v", res)
	}
	first, second := res.Shows[0], res.Shows[1]
	if first.ID != "e1" || first.Venue != "Mock Club" || first.DistanceMiles != 0.6 || first.Genre != "" || first.PriceMin != 18 || first.Lat != 41.92 {
		t.Fatalf("first show (sorted by date) = %+v", first)
	}
	if second.Time != "20:00:00" || second.Subgenre != "Indie Rock" || strings.Join(second.Acts, ",") != "Mock Band A,Mock Band B" || second.Address != "2 Mock Ave" {
		t.Fatalf("second show = %+v", second)
	}
}

func TestMusicShowsRejectsBadDates(t *testing.T) {
	c := New("k", WithBaseURL("http://127.0.0.1:1"))
	if _, err := c.MusicShows(context.Background(), Search{From: "Nov 12", To: "2026-11-12"}); err == nil {
		t.Fatal("expected a date error")
	}
}

// net/http errors quote the URL, which carries the key.
func TestErrorsNeverCarryTheKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // connection refused
	c := New("s3cret-key", WithBaseURL(srv.URL))
	_, err := c.MusicShows(context.Background(), Search{From: "2026-11-12", To: "2026-11-12"})
	if err == nil || strings.Contains(err.Error(), "s3cret-key") {
		t.Fatalf("err = %v", err)
	}

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"fault":{"faultstring":"Invalid ApiKey"}}`, http.StatusUnauthorized)
	}))
	defer srv.Close()
	c = New("s3cret-key", WithBaseURL(srv.URL))
	_, err = c.MusicShows(context.Background(), Search{From: "2026-11-12", To: "2026-11-12"})
	if err == nil || strings.Contains(err.Error(), "s3cret-key") || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("err = %v", err)
	}
}

func TestRateLimitIsRetriedOnce(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		io.WriteString(w, eventsBody)
	}))
	defer srv.Close()
	c := New("k", WithBaseURL(srv.URL), WithRetryWait(time.Millisecond))
	if _, err := c.MusicShows(context.Background(), Search{From: "2026-11-12", To: "2026-11-12"}); err != nil || n.Load() != 2 {
		t.Fatalf("err = %v after %d calls", err, n.Load())
	}
}

func TestGeohash(t *testing.T) {
	// Reference value from the geohash spec's worked example.
	if got := Geohash(57.64911, 10.40744, 11); got != "u4pruydqqvj" {
		t.Fatalf("geohash = %s", got)
	}
}
