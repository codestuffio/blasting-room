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
                                         + Ticketmaster shows and tours
```

A Go tool-use loop on Claude drives a set of Qloo tools, plus Ticketmaster lookups and a deal calculator:

| Tool | Endpoint | Purpose |
|---|---|---|
| `resolve_entity` | `/search` | name → entity ID |
| `find_genre_tags` | `/v2/tags` | genre words → tag URNs |
| `discover_artists` | `/v2/insights` (artist, seed + venue-point signals) | acts that share the room's audience and over-index near the venue |
| `score_shortlist` | `/v2/insights` (`filter.results.entities`, explainability) | rank candidates, with reasons |
| `local_heatmap` | `/v2/insights` (`urn:heatmap`) | where the fan base clusters vs. the venue |
| `momentum` | `/v2/trending` | 6-month popularity trend, drawn as the memo's momentum chart |
| `compare_audiences` | `/v2/analysis/compare` | headliner/support overlap |
| `audience_demographics` | `/v2/insights` (`urn:demographics`) | age/gender skew → ticket tier |
| `co_promo_partners` | `/v2/insights` (cross-domain place/podcast/brand) | nearby bars, podcasts, sponsors |
| `competing_shows` | Ticketmaster Discovery `/events` (music, 5 mi, the date ±1 night) | shows that would pull the same crowd the same night, pinned on the map |
| `tour_dates` | Ticketmaster Discovery `/attractions` + `/events` by act | how the date fits the act's tour: nights before and after, open windows nearby, radius-clause risk, ticket prices elsewhere |
| `price_deal` | — (server-side math) | prices an offer against the room's costs: break-even and what each side takes home at half, three-quarters and a full house |
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
| `TICKETMASTER_API_KEY` | — | Discovery API consumer key (the secret isn't used). Empty = competing shows aren't checked, and memos say so |
| `QLOO_BASE_URL` | `https://hackathon.api.qloo.com` | Hackathon keys only work on this host |
| `QLOO_MOCK_SCENARIO` | `full` | Mock only. `sparse`: short lists, many artists missing heatmap/trend/demographics. `degraded`: heatmap down, some calls fail with 503. `empty`: every call returns nothing, like Qloo's silent failures |
| `BR_MODEL` | `claude-opus-5-5` | |
| `BR_EFFORT` | `medium` | `low` / `medium` / `high` / `xhigh` / `max` |
| `BR_MAX_TURNS` | `20` | Upper bound on agent turns per run |
| `BR_DAILY_RUNS` | `100` | Agent runs allowed per UTC day across all visitors, to cap Anthropic spend on a public demo. `0` = no cap |
| `BR_CLIENT_IP_HEADER` | — | Header from a trusted proxy holding the visitor's IP, for rate limiting. Set `CF-Connecting-IP` behind a Cloudflare Tunnel, and only when every request comes through it |
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

The public demo runs on Coolify behind a Cloudflare Tunnel. [docs/DEPLOY.md](docs/DEPLOY.md) has the steps. The image runs on any container host. It needs `ANTHROPIC_API_KEY` and `QLOO_API_KEY`, and it checks its own health with `blastingroom -healthcheck`, because the distroless image has no curl.

The public demo has guardrails: 6 runs per visitor IP per hour and at most 4 concurrent runs. Run streams send a keepalive comment every 15 seconds, so proxies don't drop them while the agent thinks.

## Data handling

- **Fan data:** only aggregate taste signals are used; no fan personal data is collected or sent anywhere.
- **Caching:** Qloo responses are cached in memory on the server and never written to the repo, per the hackathon data terms.
- **Demo venues:** they have fictional names in real neighborhoods and imply no partnership.

## License

[MIT](LICENSE)
