// Command blastingroom serves the Blasting Room API and web app.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"blastingroom/internal/agent"
	"blastingroom/internal/api"
	"blastingroom/internal/qloo"
)

func main() {
	// The runtime image is distroless (no curl or wget), so container health
	// checks run the binary itself: blastingroom -healthcheck.
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		if err := healthcheck("http://127.0.0.1:" + env("PORT", "8080") + "/api/health"); err != nil {
			fmt.Fprintln(os.Stderr, "healthcheck:", err)
			os.Exit(1)
		}
		return
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	var q *qloo.Client
	if key := os.Getenv("QLOO_API_KEY"); key != "" {
		q = qloo.New(key, qloo.WithBaseURL(env("QLOO_BASE_URL", qloo.DefaultBaseURL)))
		log.Info("qloo: live", "base_url", env("QLOO_BASE_URL", qloo.DefaultBaseURL))
	} else {
		scenario, err := qloo.ParseMockScenario(os.Getenv("QLOO_MOCK_SCENARIO"))
		if err != nil {
			log.Error("qloo: bad QLOO_MOCK_SCENARIO", "err", err)
			os.Exit(1)
		}
		q = qloo.NewMockScenario(scenario)
		log.Warn("qloo: QLOO_API_KEY not set, serving synthetic mock data", "scenario", scenario)
	}

	srv := &api.Server{
		QlooMock:       q.IsMock(),
		MockScenario:   string(q.MockScenario()),
		StaticDir:      env("STATIC_DIR", "../web/build"),
		Log:            log,
		ClientIPHeader: os.Getenv("BR_CLIENT_IP_HEADER"),
	}
	if srv.ClientIPHeader != "" {
		log.Info("rate limit keyed on a proxy header", "header", srv.ClientIPHeader)
	}
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		maxTurns, _ := strconv.Atoi(env("BR_MAX_TURNS", "20"))
		srv.Runner = agent.New(anthropic.NewClient(), q, agent.Config{
			Model:    env("BR_MODEL", "claude-opus-5-5"),
			Effort:   env("BR_EFFORT", "medium"),
			MaxTurns: maxTurns,
		})
	} else {
		log.Warn("agent disabled: ANTHROPIC_API_KEY not set")
	}

	httpSrv := &http.Server{
		Addr:              ":" + env("PORT", "8080"),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		log.Info("listening", "addr", httpSrv.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()
	<-ctx.Done()

	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpSrv.Shutdown(shutdown)
}

// healthcheck GETs url and fails unless it answers 200 within a few seconds.
func healthcheck(url string) error {
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
