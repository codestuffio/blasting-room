// Command probe runs the Day-1 data probes from docs/BLASTING_ROOM.md §7
// against the live Qloo hackathon host and prints a verdict for each.
//
// Output goes to stdout only. Don't redirect it into the repo: Qloo's
// hackathon terms forbid committing response data.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"blastingroom/internal/agent"
	"blastingroom/internal/qloo"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

// config holds the probe subjects. The defaults are real, mid-size acts so
// the probes test the tier Blasting Room actually books.
type config struct {
	Artist  string    // mid-size, fairly niche artist: heatmap, demographics, cross-domain
	Compare [2]string // two similar artists
	Genre   string    // free text resolved to a genre tag
	PopMax  float64   // filter.popularity.max for the mid-tier run
	City    string    // metro for the heatmap
	Venue   agent.Venue
	Only    map[int]bool // nil runs every probe

	APIKey  string
	BaseURL string
	QlooBin string // harness CLI for probe 7; "" skips it
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	cfg := config{Venue: agent.DemoVenues[0]}
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.Artist, "artist", "Horsegirl", "mid-size artist for the heatmap, demographics and cross-domain probes")
	compare := fs.String("compare", "Wednesday,MJ Lenderman", "two similar artists to compare, comma-separated")
	fs.StringVar(&cfg.Genre, "genre", "indie rock", "genre resolved to a tag for location-signaled recs")
	fs.Float64Var(&cfg.PopMax, "popmax", 0.9, "filter.popularity.max for the mid-tier recs run")
	fs.StringVar(&cfg.City, "city", cfg.Venue.City, "metro for the heatmap")
	only := fs.String("only", "", "comma-separated probe numbers to run (default: all)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg.APIKey = getenv("QLOO_API_KEY")
	if cfg.APIKey == "" {
		fmt.Fprintln(stderr, "probe: QLOO_API_KEY is not set. Add it to .env at the repo root (make probe loads it) or export it, then rerun.")
		return 2
	}
	cfg.BaseURL = qloo.DefaultBaseURL
	if u := getenv("QLOO_BASE_URL"); u != "" {
		cfg.BaseURL = u
	}

	pair := strings.Split(*compare, ",")
	if len(pair) != 2 {
		fmt.Fprintln(stderr, "probe: -compare needs exactly two comma-separated artists")
		return 2
	}
	cfg.Compare = [2]string{strings.TrimSpace(pair[0]), strings.TrimSpace(pair[1])}
	if *only != "" {
		cfg.Only = map[int]bool{}
		for _, s := range strings.Split(*only, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(s))
			if err != nil || n < 1 || n > 7 {
				fmt.Fprintf(stderr, "probe: -only: %q is not a probe number (1-7)\n", s)
				return 2
			}
			cfg.Only[n] = true
		}
	}
	if bin, err := exec.LookPath("qloo"); err == nil {
		cfg.QlooBin = bin
	}

	fmt.Fprintf(stdout, "Qloo data probes against %s (venue: %s, %s)\n", cfg.BaseURL, cfg.Venue.Name, cfg.Venue.LocationQuery())
	return probeAll(ctx, qloo.New(cfg.APIKey, qloo.WithBaseURL(cfg.BaseURL)), cfg, stdout)
}
