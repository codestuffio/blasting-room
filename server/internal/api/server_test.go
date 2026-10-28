package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"blastingroom/internal/agent"
)

type stubRunner struct{}

func (stubRunner) Run(_ context.Context, _ agent.Request, emit func(agent.Event)) (*agent.Memo, error) {
	emit(agent.Event{Type: "tool_call", Data: agent.ToolCall{ID: "t1", Name: "discover_artists"}})
	m := &agent.Memo{Summary: "ok"}
	emit(agent.Event{Type: "memo", Data: m})
	emit(agent.Event{Type: "done"})
	return m, nil
}

func newTestServer(t *testing.T, r Runner, opts ...func(*Server)) *httptest.Server {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "200.html"), []byte("<html>app</html>"), 0o644)
	s := &Server{Runner: r, QlooMock: true, MockScenario: "sparse", StaticDir: dir, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for _, o := range opts {
		o(s)
	}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, url string) (int, string) {
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestHealthVenuesAndSPAFallback(t *testing.T) {
	srv := newTestServer(t, nil)
	if code, body := get(t, srv.URL+"/api/health"); code != 200 || !strings.Contains(body, `"qloo_mode":"mock"`) || !strings.Contains(body, `"agent_ready":false`) || !strings.Contains(body, `"mock_scenario":"sparse"`) {
		t.Fatalf("health: %d %s", code, body)
	}
	if code, body := get(t, srv.URL+"/api/venues"); code != 200 || !strings.Contains(body, "lantern") {
		t.Fatalf("venues: %d %s", code, body)
	}
	for _, path := range []string{"/", "/some/client/route"} {
		if code, body := get(t, srv.URL+path); code != 200 || !strings.Contains(body, "app") {
			t.Fatalf("spa fallback %s: %d %s", path, code, body)
		}
	}
}

func TestRunStreamsEvents(t *testing.T) {
	srv := newTestServer(t, stubRunner{})
	resp, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(`{"venue_id":"lantern","mode":"fill_date"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	body := string(b)
	if resp.Header.Get("Content-Type") != "text/event-stream" || !strings.Contains(body, "event: memo\ndata: {") || !strings.Contains(body, "event: done") {
		t.Fatalf("stream: %s", body)
	}
}

func TestRunValidation(t *testing.T) {
	srv := newTestServer(t, nil)
	resp, _ := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(`{"venue_id":"lantern"}`))
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("no runner: %d", resp.StatusCode)
	}

	srv = newTestServer(t, stubRunner{})
	for body, want := range map[string]int{
		`{"venue_id":"nowhere"}`:                    http.StatusBadRequest,
		`{"venue_id":"lantern","mode":"vet_offer"}`: http.StatusBadRequest,
		`not json`: http.StatusBadRequest,
	} {
		resp, _ := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
		if resp.StatusCode != want {
			t.Errorf("%s: got %d want %d", body, resp.StatusCode, want)
		}
	}
}

// slowRunner stays quiet long enough for several heartbeats.
type slowRunner struct{ quiet time.Duration }

func (r slowRunner) Run(_ context.Context, _ agent.Request, emit func(agent.Event)) (*agent.Memo, error) {
	emit(agent.Event{Type: "start"})
	time.Sleep(r.quiet)
	emit(agent.Event{Type: "done"})
	return &agent.Memo{}, nil
}

func TestRunSendsHeartbeatsWhileQuiet(t *testing.T) {
	srv := newTestServer(t, slowRunner{quiet: 80 * time.Millisecond}, func(s *Server) { s.Heartbeat = 10 * time.Millisecond })
	resp, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(`{"venue_id":"lantern","mode":"fill_date"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	body := string(b)
	if n := strings.Count(body, ": keepalive\n\n"); n < 2 {
		t.Fatalf("%d heartbeats in %q, want several", n, body)
	}
	// Every frame is either a whole event or a whole comment.
	for _, frame := range strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n") {
		if frame != ": keepalive" && !strings.HasPrefix(frame, "event: ") {
			t.Fatalf("malformed frame %q in %q", frame, body)
		}
	}
}

func TestClientIP(t *testing.T) {
	req := func(remote string, headers map[string]string) *http.Request {
		r := httptest.NewRequest("POST", "/api/runs", nil)
		r.RemoteAddr = remote
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		return r
	}
	direct := &Server{}
	tunnel := &Server{ClientIPHeader: "CF-Connecting-IP"}
	for _, c := range []struct {
		name string
		s    *Server
		r    *http.Request
		want string
	}{
		{"remote address", direct, req("203.0.113.7:5555", nil), "203.0.113.7"},
		{"forged X-Forwarded-For is ignored", direct, req("203.0.113.7:5555", map[string]string{"X-Forwarded-For": "198.51.100.1"}), "203.0.113.7"},
		{"untrusted CF header is ignored", direct, req("203.0.113.7:5555", map[string]string{"CF-Connecting-IP": "198.51.100.1"}), "203.0.113.7"},
		{"trusted CF header", tunnel, req("10.0.0.2:5555", map[string]string{"CF-Connecting-IP": "198.51.100.1"}), "198.51.100.1"},
		{"trusted CF header, IPv6", tunnel, req("10.0.0.2:5555", map[string]string{"CF-Connecting-IP": "2001:db8::1"}), "2001:db8::1"},
		{"garbage in trusted header falls back", tunnel, req("10.0.0.2:5555", map[string]string{"CF-Connecting-IP": "not-an-ip"}), "10.0.0.2"},
	} {
		if got := c.s.clientIP(c.r); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// Behind a tunnel every request comes from the proxy, so the limit must key
// on the trusted header, not the shared remote address.
func TestRateLimitIsPerVisitorBehindATunnel(t *testing.T) {
	srv := newTestServer(t, stubRunner{}, func(s *Server) { s.ClientIPHeader = "CF-Connecting-IP" })
	post := func(ip string) int {
		r, _ := http.NewRequest("POST", srv.URL+"/api/runs", strings.NewReader(`{"venue_id":"lantern","mode":"fill_date"}`))
		r.Header.Set("CF-Connecting-IP", ip)
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}
	for i := 0; i < 6; i++ {
		if code := post("198.51.100.1"); code != http.StatusOK {
			t.Fatalf("run %d: %d", i+1, code)
		}
	}
	if code := post("198.51.100.1"); code != http.StatusTooManyRequests {
		t.Fatalf("7th run from one visitor: %d, want 429", code)
	}
	if code := post("198.51.100.2"); code != http.StatusOK {
		t.Fatalf("another visitor: %d, want 200", code)
	}
}

// The daily budget is shared by every visitor, unlike the per-IP limit.
func TestDailyBudgetCapsRunsAcrossVisitors(t *testing.T) {
	srv := newTestServer(t, stubRunner{}, func(s *Server) {
		s.ClientIPHeader = "CF-Connecting-IP"
		s.DailyRuns = 2
	})
	post := func(ip string) int {
		r, _ := http.NewRequest("POST", srv.URL+"/api/runs", strings.NewReader(`{"venue_id":"lantern","mode":"fill_date"}`))
		r.Header.Set("CF-Connecting-IP", ip)
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}
	if _, body := get(t, srv.URL+"/api/health"); !strings.Contains(body, `"runs_left_today":2`) {
		t.Fatalf("health before runs: %s", body)
	}
	for i, ip := range []string{"198.51.100.1", "198.51.100.2"} {
		if code := post(ip); code != http.StatusOK {
			t.Fatalf("run %d: %d", i+1, code)
		}
	}
	if code := post("198.51.100.3"); code != http.StatusTooManyRequests {
		t.Fatalf("run past the budget: %d, want 429", code)
	}
	if _, body := get(t, srv.URL+"/api/health"); !strings.Contains(body, `"runs_left_today":0`) {
		t.Fatalf("health after runs: %s", body)
	}
}

func TestDailyBudgetResetsAtUTCMidnight(t *testing.T) {
	now := time.Date(2026, 11, 3, 23, 59, 0, 0, time.UTC)
	b := newDailyBudget(1, func() time.Time { return now })
	if !b.take() || b.take() || b.left() != 0 {
		t.Fatal("budget of 1 should allow exactly one run")
	}
	now = now.Add(2 * time.Minute)
	if b.left() != 1 || !b.take() {
		t.Fatal("budget should reset on the next UTC day")
	}
}

func TestLimiterForgetsIdleVisitors(t *testing.T) {
	l := newIPLimiter(1, 10*time.Millisecond)
	l.allow("198.51.100.1")
	l.allow("198.51.100.2")
	time.Sleep(20 * time.Millisecond)
	l.allow("198.51.100.3")
	if _, ok := l.hits["198.51.100.1"]; ok || len(l.hits) != 1 {
		t.Fatalf("hits = %v, want only the latest visitor", l.hits)
	}
}

type failingRunner struct{ err error }

func (r failingRunner) Run(context.Context, agent.Request, func(agent.Event)) (*agent.Memo, error) {
	return nil, r.err
}

func TestRunErrorsHideTransportDetails(t *testing.T) {
	for _, c := range []struct {
		err      error
		want     string
		mustSkip string
	}{
		{fmt.Errorf("%w: %w", agent.ErrClaude, errors.New(`Post "http://10.0.0.5/v1/messages": dial tcp: refused`)), "couldn't reach Claude", "10.0.0.5"},
		{errors.New("the model declined this request"), "the model declined this request", ""},
	} {
		srv := newTestServer(t, failingRunner{c.err})
		resp, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(`{"venue_id":"lantern","mode":"fill_date"}`))
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		body := string(b)
		if !strings.Contains(body, "event: error") || !strings.Contains(body, c.want) || c.mustSkip != "" && strings.Contains(body, c.mustSkip) {
			t.Fatalf("%v: stream %q", c.err, body)
		}
	}
}
