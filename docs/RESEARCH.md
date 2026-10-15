# Qloo Agentic Hackathon — Research Notes

Compiled 2026-10-14. Sources are listed at the bottom.

## The event at a glance

| | |
|---|---|
| Submission window | Sep 30 → **Oct 30, 2026, 11:45pm EDT** (16 days from today) |
| Judging / winners | Judging Nov 2–16; winners announced ~Nov 23 |
| Prizes | $15K / $6K / $4K, plus **Jason Calacanis personally invests $25K** in his favorite entry |
| Participants | ~1,400 as of Oct 14 (the 2025 event ended with 2,847 participants and 343 projects) |
| Must ship | Live, externally hosted demo usable end to end; public repo with an OSS license **visible in the repo's About section**; text description. No video required, but the rules still say judges may score on text, images and video, and may use AI-driven analysis, so include good screenshots and an optional video. |
| Judging | Stage 1 is pass/fail (theme fit and real Qloo use). Stage 2 weighs four criteria equally: **Tech Implementation** (tiebreaker), Design, Potential Impact, Quality of Idea. |
| Key line | "If your submission would work the same without Qloo, you're building the wrong thing." |

**Action item:** request an API key now (https://forms.gle/zz12orkLHTAneLGz6). Keys are issued by hand and take a day or more.

## Judges: what each likely rewards

- **Mike Diolosa (Qloo CTO, new this year):** depth and correctness of Qloo use, real agent architecture, use of the official kit or MCP. Likely carries the Tech Implementation criterion.
- **Jason Calacanis:** an investable wedge — a specific customer, a business model, and founder speed. His $25K goes to a company, not a demo.
- **Todd Boehly (Eldridge; Dodgers, Lakers, Chelsea; Vivid Seats):** sports, live events, ticketing, fan monetization.
- **Michael Abrams (Live Nation EVP):** concerts, venues, routing, audience development. Ticketmaster is a Qloo customer.
- **Nicole Seligman (OpenAI board; former Sony GC):** responsible AI, privacy, governance. Qloo pitches "intelligence without identity" (no PII).
- **Cedric the Entertainer (an early Qloo investor):** fun, human, memorable, culturally authentic.

## What won last time (2025 Qloo LLM Hackathon)

| Prize | Project | Pattern |
|---|---|---|
| Grand ($10K) | **GeoTaste** — a business consultant agent with location market analysis, a map, and 13+ charts | B2B, map-first, live deploy |
| HM | **Alloy** — "cultural due diligence" for M&A, ReAct agent, PDF board reports | High-stakes B2B niche, deepest Qloo use, ROI pitch ("6 min vs 6 weeks") |
| HM | **Resonance AI** — political campaign audience intelligence (Google ADK multi-agent) | Specific vertical. The UI was rough Streamlit, so polish was not decisive. |
| HM | **Zesty** — the "unrecommendation" engine (recommends the opposite of your taste) | The only consumer winner; won on a contrarian concept |

**Saturated themes (avoid):**
- travel and itinerary planners (about 60 entries)
- generic "discover your cultural DNA" chatbots (60–70)
- generic marketing and ad tools (about 55)
- restaurant and group-dining pickers (about 40)
- movie, music and book recommenders, including mood-based ones (35–40)
- dating, "cultural twin" and roommate matching (about 20; Qloo already sells this to Match Group)

None of the generic chatbots won.

**Under-explored themes:**
- health and accessibility (about 15 entries, all weak)
- education
- B2B vertical decision tools
- real agent workflows
- MCP servers (only 2 entries)
- gift finders (3)

**What winners shared:**
1. A specific paying audience with high stakes.
2. Qloo is load-bearing.
3. A real agent loop with tools and fallbacks.
4. A live demo that produces a professional deliverable.
5. For consumer apps, an inverted or non-obvious concept.

## Qloo API cheat sheet (hackathon key)

- **Host:** `https://hackathon.api.qloo.com`. It is **not** api.qloo.com; every docs example uses prod, so swap the host.
- **Auth:** header `X-Api-Key`, GET only.
- **Response data:** may be cached privately server-side; never commit it to the public repo.
- **Official kit:** `github.com/qloo/qloo-hackathon-kit`, npm `@qloo/qloo-harness` (Node ≥22.19).
  - `qloo mcp` runs a **stdio MCP server**. Tools:
    - `qloo_recommend`, `qloo_rank`, `qloo_where_popular`, `qloo_compare_audiences`
    - `qloo_audience_demographics`, `qloo_trends`, `qloo_find_tags`, `qloo_describe`
  - Every tool returns an envelope with `status`, `summary`, `results` and `error.recovery`.
  - It needs a VM or container backend; it can't run in serverless functions.
  - Probably set `QLOO_BASE_URL=https://hackathon.api.qloo.com` (inferred from the package source, not yet tested).
  - The kit's SUBMISSION.md says judges weigh usefulness, execution, **provenance and user clarity**, responsible data handling, and reproducibility.

**Endpoints:**

| Endpoint | What it does | Notable params |
|---|---|---|
| `/search` | name → entity UUID | `query`, `types`, `filter.location` + `filter.radius` (**miles**) |
| `/entities` | batch lookup; also accepts **external IDs** (Spotify, IMDb, ISBN13, IGDB, MusicBrainz, Wikidata, Resy, Michelin…) | `entity_ids`, `external.spotify.ids`, … |
| `/v2/tags`, `/v2/tags/types` | resolve tag URNs (genres, cuisines, ambience, accessibility, amenities, keywords) | `filter.query`, `feature.semantic_search` |
| `/v2/insights` `filter.type=urn:entity:*` | affinity recommendations, including **cross-domain** (artist → restaurants, show → brands) | signals (entities, tags, demographics, location, all weightable), many filters, `feature.explainability=true` |
| `/v2/insights` `filter.type=urn:heatmap` | **where** a taste over-indexes geographically (geohash, city or neighborhood) | `filter.location.query`, `output.heatmap.boundary`, `heatmap.dimensions=price_level,…` |
| `/v2/insights` `filter.type=urn:demographics` | age and gender skew of any entity or tag | signal entity/tag |
| `/v2/insights` `filter.type=urn:tag` | **taste analysis**: describe a fan base, neighborhood or audience as tags | `filter.tag.types`, `diversify.by=subtype` |
| `/v2/insights` with `filter.results.entities` | **score your own shortlist** against signals (rank, not discover) | |
| `/v2/analysis/compare` | compare two entity groups' taste profiles | `a.signal…`, `b.signal…`, `filter.location.query` |
| `/v2/trending` | popularity time series with **velocity** | `filter.start_date/end_date` |
| `/v2/audiences` | audience segments (life stage, hobbies, professional area, communities…) | deprecated in the spec but still listed |

**Entity types:**
- artist, book, brand, destination, movie, person, place, podcast, tv_show, video_game. Test both `video_game` and `videogame`.
- locality works as a signal only.
- There is no `music` or `food` type; use artist, and place or brand.

**Gotchas:**
- IDs must be resolved first (free text is rejected).
- Invalid parameters are **silently ignored** and return 200 with empty results.
- `take` max is 50 and `page` max is 100.
- Radius units differ: miles in `/search`, meters in Insights.
- Location filters apply only to place, destination and heatmap.
- Data coverage gaps exist.
- Docs error bodies are unreliable.

## Problem spaces with evidence (from the problem-space research)

1. **Dementia and personalized music.** Playlists drawn from ages 15–25 reduced antipsychotic use (GMU 2024). But a 976-person RCT found no effect, plausibly because of poor personalization fidelity. That gap is the opening.
2. **Senior and long-term-care dining.** Unmet cultural food preference is a risk factor for weight loss in care homes. More menu choice raises satisfaction but also waste.
3. **Hospice legacy and comfort.** Music interventions helped in 6 of 11 studies.
4. **Rehab adherence.** 76% of patients don't finish home PT (Ohio State 2026).
5. **Loneliness and social prescribing.** About half of US adults report loneliness. Link workers currently match people to activities by guesswork.
6. **Refugees, expats and the trailing spouse.** Spouse maladjustment is the top cause of failed assignments, which cost $40K–$1M each. Military spouse unemployment is 29.9%.
7. **Independent venues.** 56% report advance sales down year over year (NIVA 2026). Independent restaurants shrank 2.3% in 2025.
8. **Reading for pleasure.** Daily reading fell from 28% to 16% (2004 → 2023). Libraries need cross-domain reader's advisory.
9. **Public health.** Tailoring the *message* barely moves outcomes. Tailoring the **channel and messenger** is where taste data helps.
10. **Avoid** using taste for hiring or "culture fit": it reproduces class bias. Seligman will notice either way, so make that an ethics point.

**Agent-native hooks:**
- MCP, now under the Linux Foundation.
- Agent commerce protocols (ACP, UCP, AP2): they standardize checkout and say nothing about taste.
- Voice agents suit seniors.
- Computer-use agents suit staff who can't integrate APIs.

## Sources

- Event: https://qloo.devpost.com, /rules, /details/dates, /resources
- Last event: https://qloo-hackathon.devpost.com/project-gallery
- 2025 winners:
  - https://devpost.com/software/geotaste-your-agentic-qloo-taste-business-consultant
  - https://devpost.com/software/axiom-2bn391
  - https://devpost.com/software/resonance-ai-1hjbf0
  - https://devpost.com/software/zesty
- API guide: https://docs.qloo.com/reference/qloo-llm-hackathon-developer-guide
- Insights deep dive: https://docs.qloo.com/reference/insights-api-deep-dive
- Use cases:
  - https://docs.qloo.com/reference/heatmaps-use-case
  - https://docs.qloo.com/reference/demographics-use-case
  - https://docs.qloo.com/reference/taste-analysis
  - https://docs.qloo.com/reference/analysis-compare-1
  - https://docs.qloo.com/reference/get-trending
  - https://docs.qloo.com/reference/available-parameters-by-entity-type
- Kit: https://github.com/qloo/qloo-hackathon-kit
- Evidence:
  - https://publichealth.gmu.edu/news/2024-06/songs-adolescence-and-early-adulthood-enhance-mood-and-social-engagement-among-people
  - https://pmc.ncbi.nlm.nih.gov/articles/PMC9256757/
  - https://www.hypebot.com/niva-bandsintown-survey-tracks-state-of-indie-live-events/
  - https://www.fidi.org/real-cost-expatriate-assignment-failure
  - https://news.ufl.edu/2025/08/reading-for-pleasure-study/
  - https://pmc.ncbi.nlm.nih.gov/articles/PMC6608571
  - https://www.medicaleconomics.com/view/patients-aren-t-doing-their-physical-therapy-homework
