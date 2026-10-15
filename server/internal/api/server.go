// Package api serves the JSON/SSE API and the built Svelte app.
package api

import (
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
	Runner    Runner // nil when ANTHROPIC_API_KEY is not configured
	QlooMock  bool
	StaticDir string
	Log       *slog.Logger

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
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"agent_ready": s.Runner != nil,
		"qloo_mode":   map[bool]string{true: "mock", false: "live"}[s.QlooMock],
	})
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
	if !s.limiter.allow(clientIP(r)) {
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

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.Split(fwd, ",")[0])
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
