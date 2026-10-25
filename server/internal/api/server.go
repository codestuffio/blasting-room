// Package api serves the JSON/SSE API and the built Svelte app.
package api

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"blastingroom/internal/agent"
)

type Runner interface {
	Run(ctx context.Context, req agent.Request, emit func(agent.Event)) (*agent.Memo, error)
}

type Server struct {
	Runner       Runner // nil when ANTHROPIC_API_KEY is not configured
	QlooMock     bool
	MockScenario string // full | sparse | degraded | empty, when QlooMock
	StaticDir    string
	Log          *slog.Logger
	// ClientIPHeader names a header set by a trusted proxy that holds the
	// visitor's IP, e.g. CF-Connecting-IP behind a Cloudflare Tunnel. Empty
	// means the connection's remote address. Only set it when every request
	// arrives through that proxy, or visitors can forge it.
	ClientIPHeader string
	// Heartbeat is how often a run's event stream sends a comment line, so
	// proxies don't close it while the agent is quiet. Zero means 15s.
	Heartbeat time.Duration

	slots   chan struct{} // caps concurrent agent runs
	limiter *ipLimiter
}

func (s *Server) Handler() http.Handler {
	if s.slots == nil {
		s.slots = make(chan struct{}, 4)
	}
	if s.limiter == nil {
		s.limiter = newIPLimiter(6, time.Hour)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/venues", s.venues)
	mux.HandleFunc("POST /api/runs", s.run)
	mux.Handle("/", s.static())
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	body := map[string]any{
		"ok":          true,
		"agent_ready": s.Runner != nil,
		"qloo_mode":   map[bool]string{true: "mock", false: "live"}[s.QlooMock],
	}
	if s.QlooMock {
		body["mock_scenario"] = s.MockScenario
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) venues(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, agent.DemoVenues)
}

// run streams one agent run as server-sent events over the POST response.
func (s *Server) run(w http.ResponseWriter, r *http.Request) {
	if s.Runner == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "agent not configured: set ANTHROPIC_API_KEY"})
		return
	}
	var req agent.Request
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if _, ok := agent.FindVenue(req.VenueID); !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown venue_id"})
		return
	}
	if req.Mode == "vet_offer" && strings.TrimSpace(req.OfferArtist) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "offer_artist is required for vet_offer"})
		return
	}
	if !s.limiter.allow(s.clientIP(r)) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "demo rate limit reached, try again later"})
		return
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "all booking agents are busy, try again in a minute"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush() // send headers now, so a proxy sees the response start

	// Tool results arrive from concurrent goroutines.
	var mu sync.Mutex
	emit := func(e agent.Event) {
		b, err := json.Marshal(e.Data)
		if err != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, b)
		flusher.Flush()
	}

	// A comment line keeps the stream alive while the agent thinks; Cloudflare
	// closes connections that go quiet. Clients skip frames with no event.
	stop := make(chan struct{})
	beats := make(chan struct{})
	go func() {
		defer close(beats)
		t := time.NewTicker(cmp.Or(s.Heartbeat, 15*time.Second))
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				mu.Lock()
				fmt.Fprint(w, ": keepalive\n\n")
				flusher.Flush()
				mu.Unlock()
			}
		}
	}()
	defer func() { close(stop); <-beats }() // never write after the handler returns

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	start := time.Now()
	if _, err := s.Runner.Run(ctx, req, emit); err != nil {
		s.Log.Warn("run failed", "venue", req.VenueID, "err", err)
		emit(agent.Event{Type: "error", Data: map[string]string{"message": err.Error()}})
		return
	}
	s.Log.Info("run complete", "venue", req.VenueID, "mode", req.Mode, "elapsed", time.Since(start).Round(time.Millisecond))
}

// static serves the SPA build. Unknown paths get SvelteKit's 200.html
// fallback shell so client-side routes work on reload.
func (s *Server) static() http.Handler {
	fs := http.FileServer(http.Dir(s.StaticDir))
	fallback := filepath.Join(s.StaticDir, "200.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Join(s.StaticDir, filepath.Clean("/"+r.URL.Path))
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		if _, err := os.Stat(fallback); err == nil {
			http.ServeFile(w, r, fallback)
			return
		}
		http.NotFound(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// clientIP is the visitor's IP for rate limiting. X-Forwarded-For is never
// trusted on its own: its first entry is whatever the client sent.
func (s *Server) clientIP(r *http.Request) string {
	if s.ClientIPHeader != "" {
		if ip := net.ParseIP(strings.TrimSpace(r.Header.Get(s.ClientIPHeader))); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ipLimiter allows n runs per IP per window; agent runs are expensive.
type ipLimiter struct {
	n      int
	window time.Duration
	mu     sync.Mutex
	hits   map[string][]time.Time
}

func newIPLimiter(n int, window time.Duration) *ipLimiter {
	return &ipLimiter{n: n, window: window, hits: map[string][]time.Time{}}
}

func (l *ipLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	kept := l.hits[ip][:0]
	for _, t := range l.hits[ip] {
		if now.Sub(t) < l.window {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.n {
		l.hits[ip] = kept
		return false
	}
	l.hits[ip] = append(kept, now)
	return true
}
