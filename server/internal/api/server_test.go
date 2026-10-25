package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func newTestServer(t *testing.T, r Runner) *httptest.Server {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "200.html"), []byte("<html>app</html>"), 0o644)
	s := &Server{Runner: r, QlooMock: true, MockScenario: "sparse", StaticDir: dir, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
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
		`not json`:                                  http.StatusBadRequest,
	} {
		resp, _ := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
		if resp.StatusCode != want {
			t.Errorf("%s: got %d want %d", body, resp.StatusCode, want)
		}
	}
}
