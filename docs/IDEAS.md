# Qloo Agentic Hackathon — Idea Slate

Facts behind these ideas (API, judges, past winners, evidence) are in [RESEARCH.md](RESEARCH.md).

## The bar every idea must clear

1. **Qloo is load-bearing.** Strip Qloo out and the product breaks. An LLM alone can't make up heatmaps, demographic skew, cross-domain affinity or trend velocity.
2. **It is really agentic.** It runs a multi-step tool loop: resolve entities → query → evaluate → re-query → act. Ideally it uses the official `qloo mcp` server or wires Qloo into a real agent.
3. **A specific audience with a budget or a measurable outcome.** Not "users who like culture."
4. **It produces a deliverable.** A brief, plan, kit or report a professional would actually forward.
5. **It stays out of the saturated lanes.** No travel planners, "cultural DNA" chatbots, mood recommenders, dating/twin matching, or generic marketing tools.
6. **It can ship in about 16 days.** That means a hosted container (the MCP server is stdio, so no serverless), an OSS license, and screenshots.

Endpoint shorthand used below:
- **HM** = heatmap
- **DEMO** = demographics
- **TA** = taste analysis (`urn:tag`)
- **CMP** = `/v2/analysis/compare`
- **TR** = `/v2/trending`
- **XD** = cross-domain insights
- **RANK** = shortlist scoring (`filter.results.entities`)
- **EXP** = explainability
- **EXT** = external-ID bridge (Spotify, IMDb, ISBN…)

---

## Part 1 — Core ideas (strong, buildable, judge-aligned)

### 1. Blasting Room — a booking and tour-routing agent for independent promoters and venues
**Audience:** independent venues, talent buyers, small promoters. 56% of NIVA members report advance sales down year over year.

**What the agent does**
- *Venue mode:* "Who should I book for a Thursday in March?" It finds rising artists (TR velocity) whose fan base over-indexes in the venue's zip code (HM / location signal). It then picks a support act whose audience overlaps the headliner's but isn't identical (CMP).
- *Promoter mode:* "Route this artist's 12-date tour." It heatmaps the artist's affinity across metros and drafts a routing.
- *Both modes:* it builds the **co-promo plan** — nearby bars, record stores and brands the fan base loves (XD → place/brand), the podcasts to advertise on (XD → podcast), and the age skew for ad targeting (DEMO).

**Deliverable:** a one-page booking memo per show, with an "explain why" section (EXP).

**Why it wins:** it fits Abrams (Live Nation) and Boehly (Vivid Seats) closely, and the CTO sees six or more endpoints used in concert. It also has a clear SaaS wedge for Calacanis (NIVA has about 1,400+ members, and promoters pay per show). It sits on music data, Qloo's strongest domain.

**Risks:** it can look like GeoTaste (a map-based consultant), so differentiate on *decisions* (book or pass, route order), not charts. Heatmap coverage for small artists is unknown.

**Day-1 data probe:** heatmap for one mid-size artist across the US; trending velocity for five emerging artists.

### 2. Sponsor Fit — sponsorship due diligence for sports and entertainment properties
**Audience:** team and venue partnership departments, agencies, and brands buying sponsorships. Sponsorship is a $60B+ market priced on gut feel and old surveys.

**What the agent does:** given a property (a team, festival or venue) and a candidate brand list, it:
- compares the two fan bases (CMP)
- checks age and gender skew alignment (DEMO)
- overlays geography: where the fans are versus where the brand sells (HM)
- tracks momentum (TR)
- scores the candidates (RANK)
- surfaces *unexpected* brands with high fan affinity that aren't on the list (XD → brand)

**Deliverable:** a "Fit Report" PDF with an activation idea for each brand, grounded in the fans' other tastes. For example, "fans over-index on X podcast → co-branded episode."

**Why it wins:** it follows Alloy's winning pattern (high-stakes B2B due diligence, "6 weeks → 6 minutes") in a vertical Boehly owns teams in. It is very investable.

**Risks:** sports teams and leagues may be sparse as entities, so proxy through the team brand, star athletes and team podcasts (as Alloy did with "cultural proxies"). Validate on day 1.

### 3. Second Verse — a reminiscence agent for memory care
**Audience:** memory-care activity directors and family caregivers. CMS pressure to reduce antipsychotic use gives operators a budget.

**What the agent does**
- *Family intake:* a chat or voice interview. Families rarely name more than about five songs, so the agent asks for birth year, hometown, three artists, a favorite food or a show.
- *Expansion:* Qloo widens those seeds into an **era- and place-faithful world** — music from ages 15–25 (`release_year` filters), films and TV of that era, regional dishes and restaurants (location signal), and the brands of their youth.
- *Daily kit:* it produces a playlist, conversation starters, a themed "afternoon" plan (a 1962 Detroit diner hour), and photos.
- *Feedback loop:* staff log reactions (smiled, sang, agitated), and the agent reweights signals.

**Why it wins:** it addresses the "personalization fidelity" gap that may explain the null RCT, which is a real domain insight for the Idea criterion. It is close in spirit to the hackathon's own senior example without copying it. It has emotional weight for demos (Cedric) and a privacy-by-design story (Seligman: no PII sent to Qloo).

**Risks:**
- Other entrants may pick senior care because the hackathon suggested it.
- Older-era coverage is unknown.
- Avoid medical claims; say "engagement" rather than "treatment."

**Extension:** a hospice "legacy portrait" for the family.

### 4. Link Worker Copilot — taste-matched social prescribing for aging services
**Audience:** Area Agencies on Aging, NHS link workers, Medicare Advantage supplemental-benefit teams.

**What the agent does:** it turns a 5-minute intake into a **"prescription"** of three local activities, all chosen by taste (`place` with location filters, ambience and accessibility tags, `filter.hours`):
- *this* choir
- *this* film club
- *this* cuisine class within 2 miles

It suggests a buddy matched on shared affinities (RANK across consenting members) and schedules check-ins. The caseworker dashboard tracks UCLA loneliness scores over time.

**Why it wins:** loneliness is a Surgeon General–level problem, matching today is guesswork, and there is a clear public-sector or payer buyer. Accessibility tags (`urn:tag:accessibility:place`) are a nice, rarely used touch.

**Risks:** place data may be thin outside big metros; demo in one well-covered city.

### 5. Landing Pad — a relocation agent for military PCS moves and trailing spouses
**Audience:**
- Military families: active-duty spouse unemployment is 29.9%, and moves happen every 2–3 years.
- Corporate global-mobility teams: a failed assignment costs $40K–$1M+, and the spouse's poor adjustment is the top cause.

**What the agent does**
- *Neighborhood shortlist:* for each family member's taste, it builds a heatmap at the new base or city and shortlists neighborhoods (HM, neighborhood boundary).
- *"Your life here":* it plans each person's first 30 days (XD → places, podcasts and local events).
- *Taste profile across moves:* the profile persists from move to move, so each new base becomes "Fort Hood is your Fort Bragg, but here."

**Why it wins:** it has a quantified ROI and a corporate or DoD payer. It is relocation, not travel, but say that explicitly so judges don't file it under "travel planner."

**Risks:** it could still be read as a travel app, so lead with the B2B ROI.

### 6. Shelf Talker — a cross-domain reader's advisory agent for libraries
**Audience:** public and school librarians. Reading for pleasure has fallen from 28% to 16% a day. Teens who game but don't read are the hardest patrons to reach.

**What the agent does**
- The patron or teacher enters games, shows or a Spotify playlist (EXT bridges IGDB, IMDb and Spotify IDs).
- The agent recommends books (XD → book) and explains each in the patron's own language (EXP: "because you loved *Hades*…").
- It checks the local catalog's availability (Open Library or a library's public catalog API).
- It prints a "shelf talker" card or a reading ladder.
- Class mode builds a classroom set that covers everyone's tastes.

**Why it wins:** cross-domain affinity is Qloo's unique strength, and this is the clearest case where an LLM alone gives generic picks. Education is under-explored, and libraries offer a sympathetic public-good angle.

**Risks:** a smaller buyer and a softer revenue story for Calacanis. Position it as B2B to library vendors (OverDrive/Libby, BiblioCommons).

### 7. Signal Boost — a "channel and messenger" planner for public health and clinical-trial outreach
**Audience:** county health departments, FQHCs, and clinical-trial recruiters, who struggle to recruit underrepresented groups (a link to the rare-disease example).

**What the agent does:** given a target community and area, it finds:
- which podcasts, radio-style artists, venues, barbershops, restaurants and local events the community over-indexes on (TA + XD with location and demographic signals)
- where on the map to put a pop-up clinic (HM)

It then drafts the outreach plan. Its premise is the evidence-based insight that tailoring the *message* barely helps, while tailoring the *channel and messenger* does.

**Why it wins:** it has real-world impact and shows domain understanding (Idea criterion). Clinical-trial diversity is a regulatory priority with real budgets.

**Risks:** the mechanism resembles Resonance AI (political targeting, 2025 HM). Lean on health outcomes and the ethics framing.

### 8. House Night — a programming agent for independent bars and restaurants
**Audience:** independent restaurants and bars. 42% are unprofitable, and the sector shrank 2.3% in 2025.

**What it does:** "What should I run on slow Tuesdays?" It answers from the neighborhood's taste (location signal + TA + TR):
- a trivia theme
- a DJ or genre night
- a film screening
- a limited-time dish
- a brand or collab partner
- local creators to invite

It writes the event copy and tracks which nights work.

**Why it wins:** it is chain-grade audience insight for independents, with a clear distribution path through POS marketplaces (Toast, Square).

**Risks:** it sits close to GeoTaste and the crowded marketing lane. It is better as a module inside Blasting Room than as a standalone entry.

---

## Part 2 — Highly experimental ideas (higher variance, more memorable)

### A. Room Reader — a crowd-work and set-tuning agent for touring comedians
**Concept:** comedy lives on references, and references are cultural. Given tonight's city and venue, the agent:
- profiles the likely crowd (location signal + DEMO + TA)
- **scores each reference in the comedian's set** for "will this land here?" — popularity and affinity for that place and age mix (RANK)
- suggests local swaps ("swap the Trader Joe's bit for H-E-B in Austin")
- builds a pre-show crowd-work cheat sheet: the local spots, teams and shows this crowd loves

**Why it's exciting:** it has an obvious Cedric hook, is extremely demo-able and funny, and is non-obvious. The same engine generalizes to speechwriters, keynote speakers and wedding toasts.

**Unknown:** whether Qloo's popularity data varies enough by city to make the scoring feel real. Probe early.

### B. Period Piece — an anachronism checker and set-dressing agent for writers and production designers
**Concept:**
- Paste a scene set in "Dayton, Ohio, summer 1994, two 17-year-olds." The agent flags anachronisms (release dates of songs, films, shows and brands mentioned).
- It suggests period- and place-authentic replacements: what was on TV, which mall brands, what they'd quote — using `release_year` filters + location signal + DEMO (age-appropriate).
- It outputs a set-dressing and soundtrack-clearance shortlist.

**Why it's exciting:** it is a novel professional niche (film/TV, novelists, game writers), and LLMs are *known* to hallucinate period details. Qloo's dated, located entities are the fix. Cedric and Seligman (former Sony) would both get it.

**Unknown:** how well Qloo captures locale-specific popularity *for a past year*. Release dates are solid; "what was big in Dayton in 1994" is approximated.

### C. Sealed Bids — private multi-agent taste negotiation
**Concept:**
- Each person in a group (a family, team offsite or friend trip) has a *personal agent* that holds their taste privately.
- The agents negotiate over A2A or MCP for a shared choice (venue, film, menu). They exchange only **Qloo-scored rankings** of candidates (RANK), never raw preferences.
- A "fairness ledger" makes sure the same person doesn't lose every time.

**Why it's exciting:**
- Multi-agent and privacy-preserving, an advanced pattern that would impress the CTO.
- It turns the saturated group-picker idea into a novel architecture.
- It fits Seligman directly ("intelligence without identity").
- It is a preview of how personal AI agents will coordinate.

**Unknown:** whether it reads as substantive rather than a gimmick. Pick a high-stakes framing, such as corporate offsites or family reunions.

### D. Analogy Engine — a tutor that explains concepts through the student's own culture
**Concept:**
- The student shares their favorite games, shows and artists. The agent explains photosynthesis, supply and demand, or the French Revolution through analogies built from entities the student actually loves.
- Qloo XD widens the "reference pool" beyond what the student named, and DEMO and popularity filters keep references age-appropriate.
- A teacher mode generates a whole-class analogy set that covers every student.

**Why it's exciting:** culturally responsive teaching is a real pedagogy, and this is a fresh use of taste — **explanation, not recommendation**. Without Qloo, the LLM defaults to references from its own era, like *Star Wars* and *The Office*.

**Unknown:** it is harder to show Qloo is load-bearing. Make the "reference pool" visible in the UI, with and without Qloo.

### E. Taste Oracle — a hosted taste-reasoning service for *other* agents (shopping, gifting, concierge)
**Concept:**
- Agent-commerce protocols (ACP, UCP, AP2) standardize checkout but say nothing about taste.
- Build a hosted remote MCP and A2A service that any agent can call with "rank these 20 products for this person or occasion."
- It returns scores **plus provenance receipts** (EXP: why, from which signals).
- The user holds a portable "taste passport" that they control and can revoke.
- Demo it by wiring it into Claude or ChatGPT as a connector and buying a gift through a real store's catalog.

**Why it's exciting:** it is agent-native infrastructure, a strong investable story for Calacanis ("Plaid for taste"), and literally "wire Qloo into an agent you already have."

**Unknown:** infrastructure is harder to demo and judges like products. Qloo's own kit already ships a stdio MCP, so value has to come from hosting, provenance, the passport and the protocol bridge.

### F. Synthetic Block — a neighborhood focus group grounded in real taste data
**Concept:**
- Spin up 30 agent "residents" of a specific neighborhood. Each persona is sampled from that place's Qloo taste profile, demographic skew and heatmap (TA + DEMO + HM).
- Pitch them a new business, a public-art proposal or a rezoning plan, and get a focus-group transcript plus a dissent report.
- Validate the personas: each persona's "picks" must score well against the neighborhood's real Qloo affinities, or it gets regenerated.

**Why it's exciting:**
- Synthetic personas are hot but usually ungrounded. This is the grounded version.
- It could serve city planners, small-business owners and nonprofits.

**Unknown:** critics will call synthetic research junk. Pitch it as "rehearsal before real outreach," with an explicit ethics section.

### G. Away Day — a match-day experience agent for the visiting crowd
**Concept:** for a stadium or arena, given tonight's opponent (and so the visiting fans' city), the agent programs:
- the pre-game playlist
- a concession special (the visiting city's signature taste)
- in-venue screen content and a welcome for away fans
- partner bars near the venue that the visiting fan base would like (HM + XD → place)

**Why it's exciting:** it is a direct Boehly (Dodgers, Lakers, Chelsea) and Abrams hook, a fun demo and monetizable through fan experience and concession revenue. It is a narrow but vivid wedge into Blasting Room–style venue intelligence.

**Unknown:** it is a thin product on its own; it could be one "mode" of Blasting Room.

### H. Displacement Radar — early warning of cultural displacement for community groups and cities
**Concept:**
- Track the taste fingerprint of a neighborhood's places over time (TR on its long-standing venues versus newcomers, TA drift).
- Flag when legacy cultural anchors (the bodega, the jazz club, the botanica) lose momentum while new tastes surge.
- Suggest which anchors to support, through grants, landmark status or programming.

**Why it's exciting:** it is civic impact with a non-obvious use of trend velocity and Qloo as a *measurement instrument*, not a recommender.

**Unknown:** the biggest data risk on this list. Trending is per entity, and coverage of small local businesses may be thin. Test before committing.

---

## Comparison

Scores run from 1 to 5, where 5 is best. For **Data risk**, 5 means the risk is low.

| # | Idea | Qloo depth | Impact | Novelty | Judge fit | Data risk | Build size |
|---|---|---|---|---|---|---|---|
| 1 | Blasting Room | 5 | 4 | 4 | 5 (Abrams, Boehly, CTO) | 4 | M |
| 2 | Sponsor Fit | 5 | 4 | 4 | 5 (Boehly, Calacanis) | 3 | M |
| 3 | Second Verse | 4 | 5 | 3 | 4 (Cedric, Seligman) | 3 | M |
| 4 | Link Worker | 4 | 5 | 4 | 3 | 3 | M |
| 5 | Landing Pad | 4 | 4 | 3 | 3 | 4 | M |
| 6 | Shelf Talker | 4 | 4 | 4 | 3 | 4 | S–M |
| 7 | Signal Boost | 4 | 5 | 4 | 4 (Seligman) | 3 | M |
| 8 | House Night | 4 | 3 | 2 | 3 | 4 | S |
| A | Room Reader | 4 | 3 | 5 | 5 (Cedric) | 2 | S–M |
| B | Period Piece | 4 | 3 | 5 | 4 | 2 | S–M |
| C | Sealed Bids | 3 | 3 | 5 | 4 (CTO, Seligman) | 4 | L |
| D | Analogy Engine | 3 | 4 | 5 | 3 | 4 | S |
| E | Taste Oracle | 4 | 4 | 4 | 4 (Calacanis, CTO) | 4 | L |
| F | Synthetic Block | 5 | 3 | 4 | 3 | 3 | M–L |
| G | Away Day | 4 | 2 | 4 | 5 (Boehly) | 3 | S |
| H | Displacement Radar | 4 | 4 | 5 | 3 | 1 | M |

## Recommendation

- **Safest high-ceiling pick: Blasting Room (1), with Away Day (G) as a bonus "venue mode."**
  - It uses the most Qloo surface area in a domain where Qloo's data is strongest.
  - It maps directly onto two judges' businesses.
  - It has a clear B2B buyer and produces concrete decisions that a pure LLM can't fake.
- **Highest-impact pick: Second Verse (3).** It is worth it if you want the emotional and impact story, and it pairs well with a voice interface.
- **Wildcard: Room Reader (A) or Period Piece (B).** These are cheap to build, unforgettable in a demo and squarely non-obvious. Good as a *second* submission, since multiple distinct entries are allowed.

## Next steps once a direction is picked

1. **Day 0:** request the API key, and scaffold a container app (Next.js + a Node agent backend running `qloo mcp`, or a direct REST client).
2. **Days 1–2:** data probes for the chosen idea (see the "Day-1 data probe" and "Unknown" notes). Kill or pivot fast if coverage is thin.
3. **Days 3–10:** build the agent loop, the deliverable output and an explainability/provenance panel. That panel scores on the kit's "provenance and user clarity" criterion.
4. **Days 11–14:** polish, deploy (Railway, Fly or Render), add an OSS license visible in the repo's About section, and set up a seeded demo path that works without signup.
5. **Days 15–16:** Devpost write-up with an ROI or impact story, screenshots, an optional 2-minute video and an ethics/data-handling note.
