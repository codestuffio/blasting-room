# Devpost submission: Blasting Room

Copy for the Devpost project page, the submission form fields, a demo-video script, a shot list for the gallery, and a pre-submit checklist. Text in `[BRACKETS]` is yours to fill in.

**Submit by midday ET on Oct 30**, not at the 11:45pm deadline.

---

## Form fields

**Project name:** Blasting Room

**Tagline** (short): A talent buyer for indie venues that knows what your neighborhood loves

**Elevator pitch** (about 200 characters):
> Live Nation has a data team. The 300-cap club down the street has a gut feeling. Blasting Room is an AI talent buyer: who to book, what to offer, and why, grounded in Qloo's taste graph.

**Try it out:**
- Live demo: https://blastingroom.codestuff.io
- Code: https://github.com/codestuffio/blasting-room (MIT)

**Built with:** `qloo`, `claude`, `anthropic`, `go`, `svelte`, `sveltekit`, `typescript`, `maplibre`, `openfreemap`, `ticketmaster-discovery-api`, `docker`, `coolify`, `cloudflare`

**The hackathon's own questions:**

| Question | Answer |
|---|---|
| When did you begin your project? | October 14, 2026 (the first commit), during the submission window |
| The public URL to your project | https://blastingroom.codestuff.io |
| Link to your PUBLIC code repo | https://github.com/codestuffio/blasting-room |
| If this was an existing project, how did you upgrade it for Qloo? | Optional. Leave blank: this is a new project, built for the hackathon. |

---

## About the project

### Inspiration

Live Nation has data science teams. The 300-cap club down the street has a spreadsheet and a gut feeling.

Independent venues are having a hard time: 56% of independent operators report advance ticket sales down year over year (NIVA and Bandsintown, 2026). A talent buyer at a small room books on instinct, on whatever agents pitch, and on follower counts. None of those say whether *this* neighborhood will show up on *this* night. One soft night costs the guarantee, the production and the bar.

A plain chatbot doesn't help. Ask a general LLM "who should I book in Logan Square?" and you get famous names that would never play a 450-cap room. The missing piece is real, local taste data, and that's what Qloo has.

### What it does

Blasting Room is a booking agent for independent music venues. Pick your room and ask one of two questions:

- **Fill an open date:** "Who should I book on a Thursday in three weeks?"
- **Vet an offer:** "An agent pitched me this act for Nov 19 at $2,500 flat. Should I take it?"

About a minute later you get a **booking memo**: a decision, not a dashboard.

- **A verdict:** Book it, Book with changes, or Pass, with a confidence meter.
- **The pick:** a headliner and a support act whose fans overlap without being clones, each with evidence-backed reasons.
- **The offer:** guarantee, backend, ticket prices and a walk-away point. The memo prices the offer against the room's costs: break-even tickets and what the artist and the room take home at a half, three-quarters and full house. **Copy offer** puts a ready-to-send offer email on the clipboard.
- **Where they'll be:** the act's tour plotted by distance from your room. It shows whether your date is on the way, a detour, or blocked: the act is booked elsewhere that night, or already playing nearby, where a radius clause would bar a second show.
- **Competing shows:** ticketed music within 5 miles that night, each rated for how hard it competes for the same crowd and pinned on the map.
- **Where the fans are:** a heatmap of where the act's fans cluster around the venue.
- **Who's coming:** age and gender skew and a suggested ticket tier.
- **A co-promo plan:** nearby bars and record shops, podcasts and brands the fan base over-indexes on, each with an activation idea.
- **Risks, alternates, and an evidence trail.** Every claim links to the exact data call (Q1, Q2, …) that supports it. Click a claim and the run log scrolls to that call.

The memo exports to PDF for a partner or a promoter.

### How we built it

**An agent, not a pipeline.** A hand-written tool-use loop in Go drives Claude (Opus 5.5, adaptive thinking) over 13 typed tools. Nothing about the order is scripted. The agent plans and resolves names to IDs first. It discovers candidates, checks local fit, tours and competing shows, prices the deal, then writes the memo through a strict-schema tool. It's told to be a critic of its own shortlist: drop candidates with weak local signal, loosen queries that come back empty, and name every data gap instead of guessing. In one test run it passed on an offer because the act played six miles away the night before (a radius-clause risk), then checked its own alternates' tours before suggesting them.

**Qloo is load-bearing.** Every judgment about taste comes from Qloo:

| Agent tool | Qloo endpoint | What it decides |
|---|---|---|
| `resolve_entity` | `/search` | Names → entity IDs, including the room's own favorite acts |
| `find_genre_tags` | `/v2/tags` | Genre words → tag URNs |
| `discover_artists` | `/v2/insights` (artist), seeded by the room's favorites plus a location signal at the venue | Acts that share the room's audience and over-index nearby, capped by popularity so they fit the room |
| `score_shortlist` | `/v2/insights` with `filter.results.entities` and explainability | Ranks candidates against local taste, and says which of the room's favorites drove each match |
| `local_heatmap` | `/v2/insights` (`urn:heatmap`) | Where the fan base clusters, and how the venue's own cell ranks |
| `momentum` | `/v2/trending` | Six-month popularity trend per act |
| `compare_audiences` | `/v2/analysis/compare` | A support act that overlaps the headliner without being a clone |
| `audience_demographics` | `/v2/insights` (`urn:demographics`) | Age and gender skew → ticket tier and ad targeting |
| `co_promo_partners` | `/v2/insights`, cross-domain (place, podcast, brand) | Nearby venues, podcasts and sponsors the fans love |

Three more tools ground the business side:

- **`competing_shows`** and **`tour_dates`** use Ticketmaster's Discovery API. They cover same-night competition, the act's routing, radius-clause risk and the ticket prices the act charges elsewhere.
- **`price_deal`** prices an offer against the room's costs. The agent tries terms with it before committing. The server then recomputes the memo's deal math from the final terms, so the numbers a buyer reads never come from the model's arithmetic.

**Provenance by construction.** Every tool result starts with a ref (`ref Q7`). The memo's evidence entries must cite the ref and the tool, and the server clears any ref that doesn't match a real call of that tool. In the UI, every claim links to its row in the run log.

**Map and chart data never go to the model.** Heatmap cells, partner coordinates, tour stops and trend series stream straight to the browser for the map and charts. Claude only sees compact summaries, which keeps its context small and its claims tied to numbers it actually read.

**Stack:**
- **Server:** one Go binary (`net/http`, `anthropic-sdk-go`) serving the API and a SvelteKit/Svelte 5 single-page app. The run streams over server-sent events.
- **Maps:** MapLibre GL with free OpenFreeMap tiles.
- **Charts:** hand-built SVG, with colors checked for colorblind separation.
- **PDF:** print styles; the map is snapshotted to an image so it prints reliably.
- **Hosting:** a single Docker image on a homelab Coolify server, published through a Cloudflare Tunnel.

**Built to be judged by strangers:**
- **Limits:** per-visitor rate limits, a daily run budget, a concurrency cap and a 5-minute run timeout.
- **Memory:** caches are bounded and kept in memory only.
- **Errors:** visitors get plain error messages; internal details stay in the server log.
- **Mock mode:** without a Qloo key the app runs on synthetic data. Four scenarios (full, sparse, degraded, empty) exercise the agent's critic and the UI's empty and degraded paths.
- **Tests:** 65 Go tests, run against a fake Messages API and fake Qloo and Ticketmaster servers. The real APIs are never called from tests.

### Challenges we ran into

- **Qloo fails silently.** An invalid or over-narrow query returns an empty list, not an error. We probed the live hackathon API before building. Location alone returns city-wide superstars, so discovery needs the room's favorite acts as seeds. Some neighborhood names silently resolve to nothing, so we signal the venue's exact point instead. Genre filters return nothing for artists, so genres go in as signals. Every empty result goes back to the agent with a hint on how to loosen the query.
- **Big responses.** One audience-comparison response is about 44 KB, mostly repetition. We summarize it to the top shared and distinct traits before Claude sees it.
- **Momentum data that doesn't move.** On the hackathon host every trend series we probed was flat. The agent is told that a flat series says nothing either way, so it never pitches "rising momentum" the data doesn't show, and the chart says so too.
- **Ticketmaster coverage of indie tours is uneven.** Some acts list 17 dates and others list one or none, because many indie tours sell through other ticketers. The tool and the agent treat a short list as no evidence of a light schedule.
- **Keeping numbers honest.** A buyer will act on the break-even and payout figures, so they can't rest on a language model's arithmetic. The agent only proposes terms, and the server computes every figure from them. Citations work the same way: a claim's reference is kept only if it names a real call of the tool it cites.

### Accomplishments that we're proud of

- The memo makes a decision a buyer can act on: whether to book, what to offer, and on which night. It isn't a dashboard to interpret.
- It catches things a human skimming data would miss, like a radius clause triggered by a show six miles away the night before.
- Every claim is traceable to the call that produced it.
- It's honest about gaps. A sparse or failing data source lowers confidence and shows up in the risks instead of being papered over.

### What we learned

- With a taste graph, the right signal matters more than the right filter. Qloo is most useful when you give it something to be similar *to*, here the acts a room already sells.
- Agents get trustworthy when the tools make dishonesty hard: checked citations, server-side math, and summaries that only contain what the model should reason about.
- Probe first. A day of checking the live API's behavior saved us from building on assumptions the docs didn't contradict.

### What's next

- **Close the loop with real sales.** Connect a venue's ticketing and bar systems so the confidence meter becomes a forecast checked against the room's own history.
- **A routing scout.** Watch touring acts and alert a buyer when an act with local fans has an open night nearby, before someone else books it.
- **Marketing after the booking:** turn the audience profile into ad targeting, send the co-promo outreach, and suggest fixes when a show's sales pace lags.
- **A data co-op for independent venues.** Pooled, anonymized results across many rooms would give every buyer "acts like this sold 85% in comparable rooms."
- **The business:** per-venue SaaS ($99–$299 a month), promoter seats, and a channel through NIVA, the independent venue association.

### Responsible data

- **Aggregate taste only.** Qloo's signals describe groups, not people. Blasting Room never sees or stores personal data about fans, and the agent is told not to speculate about individuals.
- **No stored responses.** Qloo and Ticketmaster responses are cached in server memory for a few hours at most, never written to disk, and never committed to the repo.
- **Keys stay on the server.** API keys live only in server environment variables, and error messages are scrubbed so a key can't leak to a visitor or the model.
- **Made-up venues.** Demo venues use fictional names in real neighborhoods, so nothing implies a partnership. Their nightly costs are illustrative.

---

## Demo video script (optional, about 2 minutes)

| Time | On screen | Voiceover |
|---|---|---|
| 0:00 | The landing page | "Live Nation has a data team. The 300-cap club down the street has a gut feeling. This is Blasting Room." |
| 0:10 | Click the "Vet Wednesday at Copperline" example, then Run | "An agent pitches a buyer in East Austin: Wednesday, Nov 15, $3,000 flat. Should they take it?" |
| 0:20 | The run sheet lighting up, the trace in plain words | "The agent works it like a talent buyer. It checks local fans in Qloo's taste graph, pulls the act's tour, checks competing shows, and prices the deal." |
| 0:45 | The verdict, Pass, then "Where they'll be": the tour line dipping into the 100-mile zone the night before | "It passes. The fans are there, but the band plays six miles away the night before. That's a radius clause, and a split crowd." |
| 1:00 | Click the "Vet Wednesday at The Lantern Room" example; show the finished memo | "Same act in Chicago on Nov 19: the night after St. Louis. On the way." |
| 1:10 | The offer card: gauge, scenarios, Copy offer | "It prices the offer against the room's costs: break-even, what everyone takes home, and when to walk away. Then it writes the offer." |
| 1:30 | The map with fans, co-promo pins and competing-show rings; the co-promo plan | "Where the fans live, the bars to partner with, and the shows competing for the same crowd that night." |
| 1:45 | The evidence trail; click a ref, and the run log jumps to the call | "And every claim links back to the exact Qloo call behind it." |
| 1:55 | The logo | "Chain-grade audience intelligence for the indie room." |

Record two runs ahead of time and cut between them, since each run takes about a minute.

---

## Gallery shot list

The captured images are in `devpost-assets/`, which is gitignored because they show Qloo-derived data. Upload them from there. They're at 2× resolution from a 1500-pixel-wide window, dark theme unless noted.

- **Full-frame shots** (3:2, 3000×2000) work as Devpost thumbnails.
- **Section crops** keep their natural shape. Devpost crops thumbnails to 3:2 but shows the full image when clicked, so lead with full-frame shots.

| File | What it shows | Caption |
|---|---|---|
| `01-memo-top.png` | Verdict, title, summary, confidence meter, beside the run log | A decision, not a dashboard. |
| `06-run-in-progress.png` | Run sheet lighting up and the run log in plain words, mid-run | A real agent: it plans, checks, and criticizes its own shortlist. |
| `04a-pass-top.png` | The Copperline memo: Pass, and the reason | It says no when the data says no. |
| `02-offer.png` | Offer card: terms, gauge, scenarios, walk-away | What to offer, priced against the room's costs. |
| `03-tour-on-the-way.png` | Tour chart with the dashed leg to the room | Is the date on their way? |
| `04-tour-radius-clause.png` | Tour line inside the 100-mile zone the night before | It catches the radius clause a buyer would miss. |
| `05-map.png` | Fan heatmap, co-promo pins, competing-show rings | Where the fans are, and what you're up against. |
| `09-competing-shows.png` | Same-night shows nearby, rated by threat | Who else wants your crowd that night. |
| `10-co-promo.png` | Bars, podcasts and brands the fans love, with ideas | A co-promo plan from the fans' own taste. |
| `07-evidence-trail.png` | Claims, the tool behind each, and the call ref | Every claim traces to the call that produced it. |
| `08-landing-light.png` | Landing page with the example chips, light theme | Pick a room, ask a question, get a memo in about a minute. |

---

## Pre-submit checklist

**Repo**
- [ ] Make the repo **public** (it is private as of Oct 28). The rules require a public repo.
- [ ] License shows in the GitHub About sidebar (MIT is detected).
- [ ] Set the repo's Website field to the demo URL.
- [ ] README: the live demo link at the top.

**Live demo**
- [x] `main` is deployed in Coolify, and the app log shows `qloo: live` and `ticketmaster: live`.
- [x] Coolify env has `ANTHROPIC_API_KEY`, `QLOO_API_KEY`, `TICKETMASTER_API_KEY` (consumer key) and `BR_CLIENT_IP_HEADER=CF-Connecting-IP`.
- [ ] `BR_DAILY_RUNS` is set to a daily spend you're comfortable with (default 100).
- [x] Run both vet examples and one fill on the live site, and check the memo, map, offer and tour chart.
- [ ] **Keep the demo open** from submission through Nov 5, when judging ends (Nov 2–5) and winners are announced. No Cloudflare Access on it. Check in a private window that the URL loads without a sign-in.

**Devpost**
- [ ] Paste the fields and "About the project" above.
- [ ] Upload the gallery images (at least 3) and the optional video.
- [ ] Submit by **midday ET on Oct 30**.
