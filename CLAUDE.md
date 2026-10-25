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
- `Dockerfile` — Node builds the web app → Go builds the binary → distroless runtime. `fly.toml` deploys it.

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
- **Never commit Qloo response data** (hackathon terms). Caching stays in memory on the server.
- **Without `QLOO_API_KEY`, the server uses the mock transport** (`qloo.NewMock`). Mock names look like "Mock Artist 07" so they are never mistaken for real data.
  - `QLOO_MOCK_SCENARIO` (`full`, `sparse`, `degraded`, `empty`) serves thin, failing or empty data, to exercise the agent's critic and the UI's degraded paths. Tests use `qloo.NewMockScenario`.
  - The client retries a 429, 502, 503 or 504 (or no response) once.
- **Claude:**
  - Model `claude-opus-5-5` with adaptive thinking (`display: updates`, which streams progress notes) and an explicit effort setting.
  - Server-side refusal fallback is on: `fallbacks: "default"`.
  - Message history is append-only. Pass `resp.ToParam()` back unchanged.
- **Tool schemas must serialize deterministically** (sorted `required`) so the prompt cache stays warm.
- **Changing tools or the memo:**
  - Add a tool in `internal/agent/tools.go` and give it a label in `web/src/lib/components/Trace.svelte`.
  - When the memo changes, keep `Memo` (Go), `memoProperties` (the schema) and `web/src/lib/types.ts` in sync.
- **Map and chart data never go to the model.** A tool that has data for the map returns `withGeo{model, geo}`. `runTools` sends `model` to Claude and streams `geo` as a `geo` SSE event (`local_heatmap` and place `co_promo_partners` do this today). Keep `Geo` in `internal/agent/geo.go` and in `web/src/lib/types.ts` in sync.
  - `momentum` does the same with `withTrend{model, trend}`: the model gets a start/end/direction summary per artist, and the full series streams as a `trend` event for `MomentumChart.svelte`. Keep `Trend` in `internal/agent/trend.go` and in `types.ts` in sync.
  - The `/v2/trending` point shape is unverified, so `parseTrend` accepts several field names. Check it against `make probe` output once the key arrives.
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
- **Agent tests** use a fake Messages API (`httptest`). Never call the real Anthropic API from tests.
- **Local Go may be older than 1.24.** `go.mod` pins `toolchain go1.24.9`, which downloads automatically.
