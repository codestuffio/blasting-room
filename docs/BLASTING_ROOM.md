# Blasting Room — Product and Build Plan

> **A booking agent for independent venues and promoters, built on Qloo's taste graph.**
> Ask "who should I book for a Thursday in March?" and get a booking memo backed by real fan-base data, a support-act pick, and a co-promo plan.

Deadline: **Oct 30, 2026, 11:45pm EDT.** Background is in [RESEARCH.md](RESEARCH.md) and [IDEAS.md](IDEAS.md).

---

## 1. The problem and the pitch

- **Audience:** independent venues (200–2,000 capacity), talent buyers, and small promoters.
- **Pain:** 56% of independent operators report advance ticket sales down year over year (NIVA/Bandsintown 2026). Buyers book on gut, on agent pitches, and on raw follower counts, which don't tell you whether *this* town's fans will show up on *this* night.
- **Cost of a miss:** a soft night loses the guarantee, the production costs and the bar revenue. That is roughly $2K–$15K per show for a mid-size room (validate the figure with one or two real buyers if possible).
- **Pitch:** *Chain-grade audience intelligence for the indie room.* Live Nation has data science teams; the 300-cap club down the street has a spreadsheet. Blasting Room gives them an agent that knows what their neighborhood actually loves.
- **Why Qloo is load-bearing:**
  - local over-indexing (heatmaps and location signals)
  - fan-base overlap (compare)
  - rising momentum (trending velocity)
  - age skew for ad targeting (demographics)
  - cross-domain co-promo partners (artist → bars, podcasts, brands)
  - "why" explanations (explainability)

  An LLM alone can only guess at any of these.

## 2. Modes (scope for the hackathon)

| Mode | Prompt it answers | Priority |
|---|---|---|
| **Fill a Date** (venue) | "Who should I book on a Thursday in March? 450 cap, indie and alt crowd." | **MVP** |
| **Vet an Offer** (venue) | "An agent pitched me Artist X for Apr 12. Should I take it, and at what ticket price tier?" | **MVP** |
| **Pick the Support** | "I have the headliner; who opens?" Looks for overlapping but expanding audiences. | **MVP** (part of both modes above) |
| **Route a Run** (promoter) | "Route this artist's 10-date run across the Midwest." | Stretch |
| **Away Day** (arena) | "Program tonight's crowd experience for the visiting city's fans." | Stretch / demo flourish |

## 3. The agent loop

```
User goal ─► Planner (Claude)
               │
               ├─ resolve: /search (artist, venue locality), /v2/tags (genres)
               ├─ discover: /v2/insights artist  ← signal.location(venue) + genre tags + bias.trends
               ├─ momentum: /v2/trending (velocity over last 90 days) on candidates
               ├─ local fit: /v2/insights urn:heatmap per candidate around venue metro
               ├─ score: /v2/insights filter.results.entities (RANK the shortlist vs. venue taste)
               ├─ support act: /v2/analysis/compare headliner vs. candidates (overlap but not clone)
               ├─ audience: urn:demographics (age skew → ad targeting, ticket price tier)
               ├─ co-promo: insights cross-domain → place (nearby bars/record shops), podcast, brand
               ├─ conflicts: Ticketmaster Discovery API, competing shows same night within X miles
               └─ explain: feature.explainability=true → "why" per pick
             ▼
        Critic step: reject candidates with weak local heatmap, conflicting dates, or low data
             ▼
        Booking Memo (structured JSON → rendered page + PDF)
```

- **Model:** Claude (`claude-opus-5-5`) driven by a hand-written tool-use loop in Go (`anthropic-sdk-go`). Each Qloo endpoint is a typed Go tool. The loop streams every step to the UI. A strict `submit_booking_memo` tool returns the structured memo. Optional later: expose the same tools as an MCP server, or call the official `qloo mcp` from Go.
- **Agent behavior judges should see:** multi-step planning, a critic that throws out weak picks and re-queries, graceful handling of `empty` / `degraded` results, and a visible trace.
- **Provenance panel:** every claim in the memo links to the Qloo call that produced it. This scores on the kit's "provenance and user clarity" criterion and on Design.

## 4. Deliverable: the Booking Memo

A single page, exportable to PDF:

1. **Recommendation:** headliner + support, with a confidence level.
2. **Why this works in your room:**
   - local affinity rank (heatmap tile around the venue)
   - momentum chart (trending)
   - explainability bullets
3. **Who's coming:** age and gender skew, which suggests a ticket price tier and on-sale timing.
4. **Co-promo plan:**
   - 3 nearby bars or record stores to partner with
   - 3 podcasts to advertise on
   - 1 brand sponsor idea
   - draft copy for each
5. **Risks:** competing shows that night, data gaps.
6. **Alternates:** two backup picks.

## 5. Demo design (how judges experience it)

- The landing page has **3 seeded demo venues** in well-covered cities. Pick real neighborhoods, and use fictional or anonymized venue names so we don't imply partnerships.
- Click "Fill a Date." The agent trace streams on the left while the memo assembles on the right.
- No signup. Rate-limit by IP, and cache Qloo responses server-side, privately. Never commit Qloo data to the repo.
- A map view (MapLibre with free tiles) shows the heatmap overlay and partner venues.

## 6. Tech stack

- **Frontend:** Svelte 5 + SvelteKit with `adapter-static`, built as a single-page app. A plain `fetch` stream reads the agent trace (SSE format). MapLibre GL with OpenFreeMap tiles draws the map from `geo` events.
- **Backend:** Go 1.24, standard library `net/http`.
  - one binary serves `/api/*` and the built Svelte app
  - Qloo REST client with an in-memory TTL cache
  - agent loop built on `anthropic-sdk-go`
- **Mock mode:** with no `QLOO_API_KEY`, a built-in mock transport serves synthetic Qloo-shaped data. The whole app runs before the hackathon key arrives.
- **External APIs:**
  - Qloo, at `https://hackathon.api.qloo.com`
  - Anthropic
  - Ticketmaster Discovery later, for competing events (5K calls/day, 5 req/s)
- **Hosting:** a single multi-stage Docker image (Node builds the web app → Go builds the binary → a small runtime image). It deploys as-is to Railway, Fly or Render.
- **Repo:** public GitHub, MIT license (make sure it shows in the About sidebar), README with setup and env vars.

## 7. Day-1 data probes (go/no-go before building)

1. **Heatmap for a mid-size artist across one metro:** `filter.type=urn:heatmap&signal.interests.entities=<artist>&filter.location.query=Chicago&output.heatmap.boundary=neighborhood`. Do we get meaningful variation?
2. **Location-signaled artist recs:** `filter.type=urn:entity:artist&signal.location.query=Logan Square&filter.tags=<indie genre>&bias.trends=high`. Are they plausible and not just superstars? Try `filter.popularity.max` to push toward mid-tier acts.
3. **Trending velocity** for about 10 emerging artists over 90 days. Are the series populated?
4. **Compare** two similar artists. Are overlap and difference interpretable?
5. **Demographics** for a niche artist. Does it return non-trivial skew?
6. **Cross-domain:** artist → `place` near the venue, and artist → `podcast`. Are the results relevant?
7. **The harness against the hackathon host:** does `QLOO_BASE_URL` work with `qloo mcp`?

**If 1 or 2 fail for mid-size artists:** shift the target audience toward 1,000–3,000 cap rooms and better-known acts, or lean on Vet an Offer mode, where the user supplies the candidate.

## 8. Schedule

| Dates | Work |
|---|---|
| Oct 14–15 | Request the Qloo key (and a Ticketmaster key). Scaffold the repo, license and container deploy pipeline (deploy "hello world" on day 1). |
| Oct 16–17 | Run the data probes as soon as the key arrives, then lock scope. Build the Qloo client and MCP wiring, plus the response cache. |
| Oct 18–21 | Agent loop for Fill a Date: planner, tools, critic, structured memo output. |
| Oct 22–23 | Vet an Offer mode, support-act logic, co-promo cross-domain. |
| Oct 24–26 | UI: trace panel, memo page, map, PDF export, seeded demo venues. |
| Oct 27–28 | Hardening: empty and degraded paths, rate limiting, cold-start speed. Stretch: Away Day. |
| Oct 29 | Devpost write-up, screenshots, optional 2-minute video, ethics and data-handling note. |
| Oct 30 | Buffer. **Submit by midday ET**, not at 11:45pm. |

## 9. Risks and mitigations

| Risk | Mitigation |
|---|---|
| The API key arrives late | Build against recorded fixtures (kept locally, out of the repo) and the kit's mock mode, if one exists. |
| Thin data for small artists | `filter.popularity` bands; Vet an Offer mode; target mid-size rooms. |
| Looks like GeoTaste (the 2025 winner) | Lead with *decisions* (book or pass, who opens, what to charge), not dashboards. |
| Agent latency (many calls) | Run tool calls in parallel, cache, and stream partial memo sections. |
| Name collision | "The Blasting Room" is a well-known punk recording studio in Fort Collins (Descendents/ALL). Fine for a hackathon, but note it before using the name commercially. |

## 10. Devpost story beats

- **Opening line:** "Live Nation has a data team. The 300-cap club down the street has a spreadsheet and a gut feeling."
- **Before / after:** ask a plain LLM "who should I book in Logan Square" and you get famous names. Blasting Room returns rising, locally over-indexed acts with the receipts.
- **Qloo surface area used:** list every endpoint and explain why each one matters.
- **Business:** per-venue SaaS ($99–$299/mo), plus promoter seats, plus a NIVA partnership channel.
- **Responsible data:** aggregate taste only, no fan PII, and a provenance trail for every claim.
