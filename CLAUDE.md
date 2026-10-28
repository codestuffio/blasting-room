# Blasting Room

A booking agent for independent music venues, grounded in Qloo's taste graph. It is an entry for the Qloo Agentic Hackathon (https://qloo.devpost.com); the deadline is **Oct 30, 2026, 11:45pm EDT**.

- Product plan and scope: `docs/BLASTING_ROOM.md`
- Hackathon, judges, and Qloo API facts: `docs/RESEARCH.md`
- Other ideas considered: `docs/IDEAS.md`

## Layout

- `server/` — Go 1.24 module `blastingroom`, a single binary
  - `cmd/blastingroom` — entry point, env config
  - `internal/qloo` — REST client for the Qloo hackathon API, TTL cache, mock transport
  - `internal/agent` — Claude tool-use loop (`anthropic-sdk-go`, beta Messages API), Qloo tools, memo schema, prompts, demo venues
  - `internal/api` — `net/http` routes: `/api/health`, `/api/venues`, `POST /api/runs` (SSE stream), and static SPA serving with a `200.html` fallback
- `web/` — SvelteKit 3 + Svelte 5 (runes), `adapter-static` single-page app
  - lib imports use `#lib/...` with explicit extensions, e.g. `#lib/types.ts`
- `Dockerfile` — Node builds the web app → Go builds the binary → distroless runtime. It deploys to Coolify behind a Cloudflare Tunnel; the steps are in `docs/DEPLOY.md`.

## Commands

```bash
make test        # go vet + go test ./...
make check       # make test + svelte-check
make build       # web build + Go binary
make dev-server  # Go API on :8080 (loads ../.env)
make dev-web     # Vite on :5173, proxies /api to :8080
make probe       # Day-1 data probes against live Qloo (needs QLOO_API_KEY; stdout only)
make docker
```

## Conventions

- **Qloo API:**
  - Use host `https://hackathon.api.qloo.com`, header `X-Api-Key`, GET only.
  - Entities and tags must be resolved to IDs or URNs first.
  - Invalid parameters fail silently: you get an empty result, not an error.
  - **Live behavior, probed Oct 25, 2026** (re-check with `make probe`):
    - Artist discovery needs a seed: `signal.interests.entities` (the venue's `Seeds`) plus the location. Location alone returns city-wide superstars.
    - Signal location with the venue point (`signal.location=POINT(lon lat)` plus `signal.location.radius`, via `Venue.setLocationSignal`). `signal.location.query` resolves "Logan Square, Chicago" but silently returns nothing for "East Austin, Austin" or "Ballard, Seattle".
    - `filter.tags` returns nothing for artists, whatever the tag format. Pass genres as `signal.interests.tags`. `bias.trends` has no visible effect.
    - Mid-size touring acts sit at popularity 0.80–0.93, so `filter.popularity.max=0.9` is the mid-tier cut.
    - A ranking (`filter.results.entities`) with a location signal silently drops artists Qloo has no local data on. `score_shortlist` reports them as `no_local_signal`. Explainability is only filled in when seeds are passed.
    - A `/v2/analysis/compare` response for two artists is about 44 KB, mostly because every shared tag repeats both groups' entities. `compare_audiences` sends the model `summarizeCompare` (the top shared tags with scores, plus the tags only one side has) instead of the raw response.
    - `output.heatmap.boundary` accepts only `urn:geohash` or `urn:entity:locality`, and `urn:entity:locality` returned 0 cells for Chicago. Keep the default boundary, which the map draws.
- **Never commit Qloo response data** (hackathon terms). Caching stays in memory on the server.
- **Ticketmaster** (`internal/ticketmaster`, `competing_shows`): Discovery API event search, music only, 5 miles around the venue, the date ±1 night by default.
  - Auth is the consumer key as the `apikey` query param. The client strips the request URL from transport errors so the key never reaches a log or the model.
  - Without `TICKETMASTER_API_KEY` the tool returns `checked: false`, and the agent names the gap in risks. Don't commit Ticketmaster responses either; test fixtures are synthetic.
  - The tool returns `withGeo`: show venues stream as a `shows` geo event and draw as hollow red rings, distinct by shape from the filled partner pins.
  - The client spaces requests 220 ms apart (the API allows 5 a second and tools run in parallel) and caches responses in memory for 30 minutes.
- **Tour routing** (`tour_dates`, `internal/agent/route.go`): looks up the act by exact name (keyword search is loose), lists its events with `locale=*`, and drops a second listing for the same night and city.
  - `routeFit` judges the date: `on_the_way`, `detour`, `off_route`, `already_booked`, `already_in_market` (a show within `marketMiles` 100 in `marketDays` 60), `no_tour_data`, `not_checked`. `routeWindows` finds open nights between stops within `driveMiles` 450.
  - It returns `withRoute{model, route}`: the full stop list streams as a `route` event for `RouteChart.svelte`. Keep `Route` in `route.go` and `types.ts` in sync.
  - Ticketmaster's indie coverage is uneven (live probe: Wednesday 17 dates, Cut Worms and FACS 1 each, Ty Segall 0 despite a listed count), so a short or empty tour is never evidence of a light schedule.
- **Deal math** (`price_deal`, `internal/agent/deal.go`): a "plus" deal — guarantee plus `backend_pct` of the door above the split point (house costs + support fee + guarantee), at the advance price, before fees.
  - Demo venues carry made-up `HouseCosts` and `BarNetPerHead`.
  - The model sets the memo's `deal` terms; the server fills `deal.math` from them with `dealMath` after the memo arrives. `math` is not in the memo schema, so the numbers a buyer reads never come from the model's arithmetic.
- **Deploy (Coolify + Cloudflare Tunnel):**
  - Runs are capped per UTC day across all visitors (`BR_DAILY_RUNS`, default 100); `/api/health` reports `runs_left_today`. The Qloo cache is bounded at 64 MB, and the IP limiter drops idle visitors.
  - Rate limiting keys on `BR_CLIENT_IP_HEADER` (`CF-Connecting-IP` in production), never on raw `X-Forwarded-For`, which clients can forge.
  - `POST /api/runs` flushes its headers at once and sends `: keepalive` comments (`Server.Heartbeat`, default 15s). Clients must skip frames with no `event:` line.
  - The image is distroless with no curl, so its `HEALTHCHECK` runs `blastingroom -healthcheck`.
- **Without `QLOO_API_KEY`, the server uses the mock transport** (`qloo.NewMock`). Mock names look like "Mock Artist 07" so they are never mistaken for real data.
  - `QLOO_MOCK_SCENARIO` (`full`, `sparse`, `degraded`, `empty`) serves thin, failing or empty data, to exercise the agent's critic and the UI's degraded paths. Tests use `qloo.NewMockScenario`.
  - The client retries a 429, 502, 503 or 504 (or no response) once.
- **Claude:**
  - Model `claude-opus-5-5` with adaptive thinking (`display: updates`, which streams progress notes) and an explicit effort setting.
  - Server-side refusal fallback is on: `fallbacks: "default"`.
  - Message history is append-only. Pass `resp.ToParam()` back unchanged.
- **Tool schemas must serialize deterministically** (sorted `required`) so the prompt cache stays warm.
- **Changing tools or the memo:**
  - Add a tool in `internal/agent/tools.go` and give it a label in `web/src/lib/components/Trace.svelte`, a `describeCall` case in `internal/agent/describe.go` (the trace shows that summary instead of raw JSON), and a stage in `RunSheet.svelte`.
  - Entity names are learned from tool results during a run (`names.learn`) and stream on `tool_result.names`, so the trace and the map label artists before the memo arrives.
  - When the memo changes, keep `Memo` (Go), `memoProperties` (the schema) and `web/src/lib/types.ts` in sync.
  - **Provenance:** `runTools` gives each call a run-local ref (`Q1`, `Q2`, …), streamed on `tool_call` and prefixed to the result as a `ref Qn` line. Memo evidence cites the ref, and `checkMemo` clears any ref that names no call of the cited tool. The evidence trail links each ref to its row in the trace (`#call-Qn`).
- **Map and chart data never go to the model.** A tool that has data for the map returns `withGeo{model, geo}`. `runTools` sends `model` to Claude and streams `geo` as a `geo` SSE event (`local_heatmap`, place `co_promo_partners` and `competing_shows` do this today). Keep `Geo` in `internal/agent/geo.go` and in `web/src/lib/types.ts` in sync.
  - `momentum` does the same with `withTrend{model, trend}`: the model gets a start/end/direction summary per artist, and the full series streams as a `trend` event for `MomentumChart.svelte`. Keep `Trend` in `internal/agent/trend.go` and in `types.ts` in sync.
  - `/v2/trending` takes exactly one entity per call, and windows much shorter than 6 months come back empty, so `momentum` asks for 6 months per artist. Points are `{date, population_percentile, population_rank, population_rank_velocity, velocity_fold_change, population_percent_delta}` under `results.trending`.
  - On the hackathon host every series probed so far is constant (one percentile, zero velocity). `trendSummary` marks those `constant` so the model claims no momentum, and `MomentumChart` notes the lack of movement. Don't pitch "rising momentum" unless the probe shows series that move.
- **Empty results:** entity tools return `{"results": [], "note": ...}` with a loosening hint, since Qloo fails silently. When nothing survives, the agent submits a memo with an empty headliner, and `MemoView` shows "No confident booking".
- **Charts:** the series colors are `--series-1`…`--series-5` in `app.css`, in a fixed order where slot 1 is the headliner. They were validated for colorblind separation on `--panel` in both themes, so re-validate before changing them.
- **Look and feel: the "Console" theme** (a mixing desk).
  - Color and font tokens live in `web/src/app.css`, with a dark default and a "daylight" light variant. Use `--accent` for fills and `--accent-text` for amber text, since amber text fails contrast on the light background.
  - Shared global classes: `.led` (status light: `.on`, `.ok`, `.bad`, `.live`), `.tape` (channel label) and `.label` (mono section label).
  - Fonts are Michroma (display), IBM Plex Sans (body) and IBM Plex Mono (data), loaded from Google Fonts in `app.html`.
  - `Logo.svelte` is the VU-needle mark; `VuMeter.svelte` is the analog confidence meter.
- **Map UI** (`web/src/lib/components/VenueMap.svelte`):
  - Uses MapLibre GL, pinned to an exact version, with OpenFreeMap tiles (no key; positron for light, dark for dark).
  - MapLibre 6 loads its worker separately. It is bundled with `?worker&url` and handed to `setWorkerUrl`.
  - Heat cells, pins and labels are layers, not DOM markers, so labels never collide. Re-add them after every `setStyle`.
- **PDF export** is `window.print()` with `@media print` styles (no PDF library). Print shows only the memo, in the light palette.
  - WebGL can print blank, so print shows a PNG snapshot of the map. `VenueMap.prepareForPrint()` re-renders the map light at 2× before the snapshot and reverts on `afterprint`. A plain Cmd+P snapshots the map as it is on `beforeprint`, which relies on `preserveDrawingBuffer`.
- **Demo venues** use fictional names in real neighborhoods.
  - The "Try an example" chips in `+page.svelte` fill the form but never start a run. The two vet examples rely on Wednesday's live Ticketmaster tour (St. Louis Nov 18, Radio East in Austin Nov 14) and hide themselves once their dates pass; re-check them if the demo stories change.
- **Agent tests** use a fake Messages API (`httptest`). Never call the real Anthropic API from tests.
- **Local Go may be older than 1.24.** `go.mod` pins `toolchain go1.24.9`, which downloads automatically.
