// Command blastingroom serves the Blasting Room API and web app.
package main

import (
	"context"
	"errors"
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

	srv := &api.Server{QlooMock: q.IsMock(), MockScenario: string(q.MockScenario()), StaticDir: env("STATIC_DIR", "../web/build"), Log: log}
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

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
