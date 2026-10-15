# Blasting Room

**A booking agent for independent music venues, grounded in Qloo's cultural taste graph.**

Ask "who should I book for a Thursday in March?" and get back a booking memo. It names a headliner and support act whose fans over-index in your neighborhood. It says who's likely to buy tickets and suggests a co-promo plan, and every claim carries an evidence trail back to the Qloo call that supports it.

Built for the [Qloo Agentic Hackathon](https://qloo.devpost.com). Product plan: [docs/BLASTING_ROOM.md](docs/BLASTING_ROOM.md).

## How it works

```
Svelte SPA ──POST /api/runs──► Go server ──► Claude (tool-use loop)
     ▲                              │              │
     └──── SSE: trace + memo ◄──────┘              ▼
                                         Qloo tools (hackathon API)
```

A Go tool-use loop on Claude drives a set of Qloo tools:

| Tool | Qloo endpoint | Purpose |
|---|---|---|
| `resolve_entity` | `/search` | name → entity ID |
| `find_genre_tags` | `/v2/tags` | genre words → tag URNs |
| `discover_artists` | `/v2/insights` (artist, location signal) | acts whose fans over-index near the venue |
| `score_shortlist` | `/v2/insights` (`filter.results.entities`, explainability) | rank candidates, with reasons |
| `local_heatmap` | `/v2/insights` (`urn:heatmap`) | where the fan base clusters vs. the venue |
| `momentum` | `/v2/trending` | 90-day popularity velocity |
| `compare_audiences` | `/v2/analysis/compare` | headliner/support overlap |
| `audience_demographics` | `/v2/insights` (`urn:demographics`) | age/gender skew → ticket tier |
| `co_promo_partners` | `/v2/insights` (cross-domain place/podcast/brand) | nearby bars, podcasts, sponsors |
| `submit_booking_memo` | — | strict-schema final memo |

The UI streams every tool call and result as it happens, then shows the memo.

## Run locally

Requirements:
- Go 1.24+. If your local Go is older, the pinned `toolchain` in `server/go.mod` downloads the right version automatically.
- Node 22+.

```bash
cp .env.example .env
```

Add `ANTHROPIC_API_KEY` to `.env`. You can leave `QLOO_API_KEY` empty to use the built-in mock, which serves synthetic Qloo-shaped data.

```bash
cd web && npm install && npm run build && cd ..
```

```bash
make dev-server
```

Then open http://localhost:8080.

For frontend work with hot reload, run `make dev-server` and `make dev-web` together and open http://localhost:5173. Vite proxies `/api` to the Go server.

```bash
make check
```

That runs `go vet`, the Go tests and `svelte-check`.

### Environment

| Variable | Default | Notes |
|---|---|---|
| `ANTHROPIC_API_KEY` | — | Required for agent runs; without it the UI loads but runs are disabled |
| `QLOO_API_KEY` | — | Empty = synthetic mock data |
| `QLOO_BASE_URL` | `https://hackathon.api.qloo.com` | Hackathon keys only work on this host |
| `BR_MODEL` | `claude-opus-5-5` | |
| `BR_EFFORT` | `medium` | `low` / `medium` / `high` / `xhigh` / `max` |
| `BR_MAX_TURNS` | `20` | Upper bound on agent turns per run |
| `PORT` | `8080` | |
| `STATIC_DIR` | `../web/build` | Built SPA served by the Go binary |

## Deploy

The repo builds a single container: Node builds the SPA, Go builds the binary, and both ship on a distroless runtime image.

```bash
docker build -t blasting-room .
```

```bash
docker run -p 8080:8080 --env-file .env blasting-room
```

The image runs on Fly.io as-is (see `fly.toml`), as well as on Railway or Render. Set `ANTHROPIC_API_KEY` and `QLOO_API_KEY` as secrets.

The public demo has guardrails: 6 runs per IP per hour and at most 4 concurrent runs.

## Data handling

- **Fan data:** only aggregate taste signals are used; no fan personal data is collected or sent anywhere.
- **Caching:** Qloo responses are cached in memory on the server and never written to the repo, per the hackathon data terms.
- **Demo venues:** they have fictional names in real neighborhoods and imply no partnership.

## License

[MIT](LICENSE)
