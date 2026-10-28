<script lang="ts">
	import { flushSync } from 'svelte';
	import type { Geo, Memo, Mode, Route, RouteFit, Trend, Venue } from '#lib/types.ts';
	import DealCard from '#lib/components/DealCard.svelte';
	import RouteChart from '#lib/components/RouteChart.svelte';
	import VenueMap from '#lib/components/VenueMap.svelte';
	import VuMeter from '#lib/components/VuMeter.svelte';
	import MomentumChart from '#lib/components/MomentumChart.svelte';

	let {
		memo,
		venue,
		mode = 'fill_date',
		date = '',
		geo = [],
		trends = [],
		routes = [],
		runNames = {}
	}: {
		memo: Memo;
		venue: Venue;
		mode?: Mode;
		date?: string;
		geo?: Geo[];
		trends?: Trend[];
		routes?: Route[];
		runNames?: Record<string, string>;
	} = $props();

	// The headliner's tour: matched by Qloo id, else by name; the latest lookup wins.
	let route = $derived(
		routes.findLast(
			(r) =>
				(r.entity_id && r.entity_id === memo.headliner.entity_id) ||
				r.artist.toLowerCase() === memo.headliner.name.trim().toLowerCase()
		)
	);
	let showDeal = $derived(!!memo.deal && (memo.deal.guarantee > 0 || memo.deal.ticket_price_advance > 0));
	let routing = $derived(memo.routing);
	const fitLabel: Record<RouteFit, string> = {
		on_the_way: 'On the way',
		detour: 'Detour',
		off_route: 'Off route',
		already_in_market: 'Already in market',
		already_booked: 'Booked elsewhere that night',
		no_tour_data: 'No tour data',
		not_checked: 'Not checked'
	};
	const fitLed: Record<RouteFit, string> = {
		on_the_way: 'ok',
		detour: 'on',
		off_route: '',
		already_in_market: 'bad',
		already_booked: 'bad',
		no_tour_data: '',
		not_checked: ''
	};

	// When no candidate survives the agent's checks, it submits a memo with an empty headliner.
	let hasPick = $derived(memo.headliner.name.trim() !== '');
	let title = $derived(
		!hasPick ? 'No confident booking' : memo.support.name ? `${memo.headliner.name} with ${memo.support.name}` : memo.headliner.name
	);
	let picks = $derived(
		[
			// When vetting, the headliner is the act that was offered, whether or not the verdict is to book it.
			{ role: mode === 'vet_offer' ? 'Offered act' : 'Headliner', pick: memo.headliner },
			{ role: 'Support', pick: memo.support }
		].filter((p) => p.pick.name.trim() !== '')
	);
	let chartPicks = $derived(
		[
			{ ...memo.headliner, role: 'Headliner' },
			{ ...memo.support, role: 'Support' },
			...memo.alternates.map((a) => ({ ...a, role: 'Alternate' }))
		].filter((p) => p.entity_id)
	);
	let hasTrend = $derived(
		trends.some((t) => t.series.some((s) => s.points.length > 1 && chartPicks.some((p) => p.entity_id === s.entity_id)))
	);

	// Every artist the run named, with the memo's own spelling winning.
	let names = $derived({
		...runNames,
		...Object.fromEntries([memo.headliner, memo.support, ...memo.alternates].map((p) => [p.entity_id, p.name]))
	});
	// Map labels the memo names win collisions: its co-promo places and the venues of competing shows.
	let highlight = $derived([
		...memo.co_promo.filter((c) => c.kind === 'place').map((c) => c.name),
		...(memo.competing_shows ?? []).map((s) => s.venue)
	]);
	let clashes = $derived(memo.competing_shows ?? []);

	let showRouting = $derived(hasPick && !!routing && routing.fit !== 'not_checked');

	// Jump links to the sections this memo has, in page order.
	let jumps = $derived(
		[
			{ id: 'memo-offer', label: showDeal ? 'Offer' : 'No offer', on: showDeal || !!memo.deal?.walk_away },
			{ id: 'memo-routing', label: 'Tour', on: showRouting },
			{ id: 'memo-fans', label: 'Fans', on: geo.length > 0 },
			{ id: 'memo-momentum', label: 'Momentum', on: hasTrend },
			{ id: 'memo-audience', label: 'Audience', on: true },
			{ id: 'memo-copromo', label: 'Co-promo', on: memo.co_promo.length > 0 },
			{ id: 'memo-shows', label: 'Shows nearby', on: clashes.length > 0 },
			{ id: 'memo-risks', label: 'Risks', on: memo.risks.length + memo.alternates.length > 0 },
			{ id: 'memo-evidence', label: 'Evidence', on: memo.evidence.length > 0 }
		].filter((j) => j.on)
	);


	let linked = $derived(memo.evidence.filter((ev) => ev.ref).length);

	const verdictLabel = { book: 'Book it', book_with_changes: 'Book with changes', pass: 'Pass' };
	const threatLabel = { high: 'High', medium: 'Medium', low: 'Low' };
	const kindLabel = { place: 'Nearby spot', podcast: 'Podcast', brand: 'Brand' };
	const needle = { low: 0.2, medium: 0.55, high: 0.9 };
	const tiers = ['budget', 'standard', 'premium'] as const;

	// "Download PDF" is the browser's print dialog with a print stylesheet; the map is
	// re-rendered for paper first. The title becomes the PDF's suggested file name.
	let mapView = $state<VenueMap>();
	let preparing = $state(false);
	let evidenceOpen = $state(false);
	let saved: { title: string; open: boolean } | null = null;

	async function download() {
		preparing = true;
		try {
			await mapView?.prepareForPrint();
		} finally {
			preparing = false;
		}
		window.print();
	}

	function beforePrint() {
		saved = { title: document.title, open: evidenceOpen };
		document.title = `Booking memo - ${title} - ${venue.name}`;
		evidenceOpen = true;
		flushSync();
	}

	function afterPrint() {
		if (!saved) return;
		document.title = saved.title;
		evidenceOpen = saved.open;
		saved = null;
	}
</script>

<svelte:window onbeforeprint={beforePrint} onafterprint={afterPrint} />

<article class="memo">
	<header class="head">
		<div>
			<p class="eyebrow">{mode === 'vet_offer' ? 'Offer review' : 'Booking memo'} · {venue.name}, {venue.neighborhood}</p>
			{#if memo.verdict}
				<p class="verdict {memo.verdict}"><span class="led" class:ok={memo.verdict === 'book'} class:on={memo.verdict === 'book_with_changes'} class:bad={memo.verdict === 'pass'}></span>{verdictLabel[memo.verdict]}</p>
			{/if}
			{#if hasPick && memo.support.name}
				<h2>{memo.headliner.name} <span class="with">with</span> {memo.support.name}</h2>
			{:else}
				<h2>{title}</h2>
			{/if}
			<p class="summary">{memo.summary}</p>
			<button type="button" class="download" onclick={download} disabled={preparing}>
				{preparing ? 'Preparing…' : 'Download PDF'}
			</button>
		</div>
		<div class="meter">
			<VuMeter value={needle[memo.confidence]} label={`${memo.confidence} confidence`} width={168} />
			<span class="label">Confidence · <strong>{memo.confidence}</strong></span>
		</div>
	</header>

	{#if picks.length}
	<section class="picks">
		{#each picks as p (p.role)}
			<div>
				<span class="tape">{p.role}</span>
				<h3 class="pick">{p.pick.name}</h3>
				<ul>
					{#each p.pick.rationale as r, i (i)}<li>{r}</li>{/each}
				</ul>
			</div>
		{/each}
	</section>
	{/if}

	<nav class="jump" aria-label="Memo sections">
		{#each jumps as j (j.id)}<a href={`#${j.id}`}>{j.label}</a>{/each}
	</nav>

	{#if showDeal}
		<div id="memo-offer" class="anchor">
			<DealCard deal={memo.deal} {venue} headliner={memo.headliner.name} support={memo.support.name} {date} />
		</div>
	{:else if memo.deal?.walk_away}
		<section id="memo-offer" class="no-offer anchor">
			<span class="tape">No offer</span>
			<p><span class="label">What would change it</span> {memo.deal.walk_away}</p>
		</section>
	{/if}

	{#if showRouting}
		<section id="memo-routing" class="routing anchor">
			<div class="routing-head">
				<h3>Where they'll be</h3>
				<span class="fit"><span class={`led ${fitLed[routing.fit]}`}></span>{fitLabel[routing.fit]}</span>
			</div>
			<p>{routing.summary}</p>
			{#if route}<RouteChart {route} {date} />{/if}
		</section>
	{/if}

	{#if geo.length}
		<section id="memo-fans" class="anchor">
			<h3>Where the fans are</h3>
			<VenueMap bind:this={mapView} {venue} {geo} {names} focus={memo.headliner.entity_id} {highlight} />
		</section>
	{/if}

	{#if hasTrend}
		<section id="memo-momentum" class="momentum anchor">
			<h3>Momentum</h3>
			<p>{memo.momentum}</p>
			<MomentumChart {trends} picks={chartPicks} />
		</section>
	{/if}

	<section id="memo-audience" class="grid anchor">
		<div>
			<h3>{hasPick ? 'Why it works in this room' : 'Local fit'}</h3>
			<p>{memo.local_fit}</p>
		</div>
		{#if !hasTrend}
			<div>
				<h3>Momentum</h3>
				<p>{memo.momentum}</p>
			</div>
		{/if}
		<div>
			<h3>Who's coming</h3>
			<p>{memo.audience.summary}</p>
			<p class="muted">Age skew: {memo.audience.age_skew}</p>
			<p class="muted">Suggested ticket tier</p>
			<div class="tiers">
				{#each tiers as t (t)}
					<span class:on={t === memo.audience.ticket_tier}><span class="led" class:on={t === memo.audience.ticket_tier}></span>{t}</span>
				{/each}
			</div>
		</div>
	</section>

	{#if memo.co_promo.length}
		<section id="memo-copromo" class="anchor">
			<h3>Co-promo plan</h3>
			<ul class="partners">
				{#each memo.co_promo as c, i (i)}
					<li>
						<span class="kind">{kindLabel[c.kind]}</span>
						<strong>{c.name}</strong>
						<span>{c.idea}</span>
					</li>
				{/each}
			</ul>
		</section>
	{/if}

	{#if clashes.length}
		<section id="memo-shows" class="anchor">
			<h3>Competing shows nearby</h3>
			<ul class="clashes">
				{#each clashes as s, i (i)}
					<li>
						<span class="threat"><span class="led" class:bad={s.threat === 'high'} class:on={s.threat === 'medium'}></span>{threatLabel[s.threat]}</span>
						<strong>{s.name}</strong>
						<span class="where">{s.venue} · {s.date}</span>
						<span class="why">{s.note}</span>
					</li>
				{/each}
			</ul>
		</section>
	{/if}

	<section id="memo-risks" class="grid anchor">
		{#if memo.alternates.length}
			<div>
				<h3>Alternates</h3>
				<ul>
					{#each memo.alternates as a, i (i)}<li><strong>{a.name}</strong>: {a.reason}</li>{/each}
				</ul>
			</div>
		{/if}
		{#if memo.risks.length}
			<div>
				<h3>Risks and data gaps</h3>
				<ul>
					{#each memo.risks as r, i (i)}<li>{r}</li>{/each}
				</ul>
			</div>
		{/if}
	</section>

	{#if memo.evidence.length}
		<details id="memo-evidence" class="anchor" bind:open={evidenceOpen}>
			<summary>
				Evidence trail ({memo.evidence.length} claims{linked < memo.evidence.length ? `, ${linked} traced to a call` : ''})
			</summary>
			<table>
				<thead><tr><th>Claim</th><th>Source tool</th><th>Call</th></tr></thead>
				<tbody>
					{#each memo.evidence as ev, i (i)}
						<tr>
							<td>{ev.claim}</td>
							<td><code>{ev.tool}</code></td>
							<td>
								{#if ev.ref}
									<a class="ref" href={`#call-${ev.ref}`} title="Show this call in the run of show">{ev.ref}</a>
								{:else}
									<span class="muted" title="The agent cited no call of this tool">—</span>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</details>
	{/if}

	<footer class="colophon">
		Prepared with Blasting Room for {venue.name}, {venue.neighborhood}. Taste data from Qloo: aggregate signals,
		no fan personal data.
	</footer>
</article>

<style>
	.memo {
		display: grid;
		gap: 1.5rem;
	}
	.head {
		display: flex;
		flex-wrap: wrap;
		justify-content: space-between;
		align-items: start;
		gap: 1rem 1.5rem;
	}
	.head > div:first-child {
		flex: 1 1 18rem;
	}
	.meter {
		display: grid;
		justify-items: center;
		gap: 0.4rem;
	}
	.meter strong {
		color: var(--text);
		font-weight: 500;
	}
	.eyebrow {
		margin: 0;
		color: var(--accent-text);
		font-family: var(--font-mono);
		font-size: 0.75rem;
		letter-spacing: 0.12em;
		text-transform: uppercase;
	}
	.verdict {
		display: inline-flex;
		align-items: center;
		gap: 0.55rem;
		margin: 0.6rem 0 0;
		padding: 0.3rem 0.7rem;
		border: 1px solid currentColor;
		border-radius: 6px;
		font-family: var(--font-display);
		font-size: 0.8rem;
		letter-spacing: 0.12em;
		text-transform: uppercase;
	}
	.verdict.book {
		color: var(--ok);
	}
	.verdict.book_with_changes {
		color: var(--accent-text);
	}
	.verdict.pass {
		color: var(--bad);
	}
	h2 {
		margin: 0.4rem 0 0.6rem;
		font-family: var(--font-display);
		font-weight: 400;
		font-size: clamp(1.25rem, 2.6vw, 1.75rem);
		letter-spacing: 0.03em;
		line-height: 1.25;
	}
	.with {
		color: var(--muted);
		font-weight: 400;
	}
	.summary {
		margin: 0 0 0.6rem;
		font-size: 1.05rem;
	}
	h3 {
		margin: 0 0 0.5rem;
		font-family: var(--font-mono);
		font-size: 0.75rem;
		font-weight: 400;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--muted);
	}
	h3.pick {
		margin: 0.6rem 0 0.4rem;
		font-family: var(--font-display);
		font-size: 1rem;
		letter-spacing: 0.04em;
		color: var(--text);
	}
	p {
		margin: 0 0 0.4rem;
	}
	ul {
		margin: 0;
		padding-left: 1.1rem;
	}
	.muted {
		color: var(--muted);
		font-size: 0.9rem;
	}
	.tiers {
		display: inline-flex;
		flex-wrap: wrap;
		max-width: 100%;
		gap: 4px;
		padding: 4px;
		background: var(--recess);
		border: 1px solid var(--line);
		border-radius: 8px;
	}
	.download {
		min-height: 44px;
		margin-top: 0.4rem;
		padding: 0 1rem;
		font-family: var(--font-mono);
		font-size: 0.8rem;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		border-radius: 8px;
		border: 1px solid var(--line);
		background: transparent;
		color: var(--text);
		cursor: pointer;
	}
	.download:hover:not(:disabled) {
		border-color: var(--accent);
	}
	.download:disabled {
		opacity: 0.6;
		cursor: progress;
	}
	.colophon {
		display: none;
	}
	.tiers > span {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		padding: 0.25rem 0.6rem;
		border-radius: 5px;
		font-family: var(--font-mono);
		font-size: 0.75rem;
		text-transform: uppercase;
		color: var(--muted);
	}
	.tiers > span.on {
		background: var(--line);
		color: var(--text);
	}
	.picks,
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
		gap: 1rem;
	}
	.momentum {
		display: grid;
		gap: 0.5rem;
	}
	.picks > div,
	.grid > div,
	.momentum {
		padding: 0.9rem 1rem;
		border: 1px solid var(--line);
		border-radius: 12px;
		background: var(--panel);
	}
	.partners {
		list-style: none;
		padding: 0;
		display: grid;
		gap: 0.5rem;
	}
	.partners li {
		display: grid;
		grid-template-columns: 7rem 1fr;
		gap: 0.2rem 0.75rem;
	}
	.partners li span:last-child {
		grid-column: 2;
		color: var(--muted);
	}
	/* Sticky jump bar; anchors leave room for it when scrolled to. */
	.jump {
		position: sticky;
		top: 0;
		z-index: 3;
		display: flex;
		gap: 0.35rem;
		overflow-x: auto;
		padding: 0.5rem 0;
		margin: -0.5rem 0;
		background: var(--bg);
		scrollbar-width: none;
	}
	.jump a {
		flex: none;
		padding: 0.3rem 0.7rem;
		border: 1px solid var(--line);
		border-radius: 999px;
		font-family: var(--font-mono);
		font-size: 0.72rem;
		letter-spacing: 0.05em;
		text-transform: uppercase;
		color: var(--muted);
		text-decoration: none;
	}
	.jump a:hover,
	.jump a:focus-visible {
		color: var(--text);
		border-color: var(--accent);
	}
	.anchor {
		scroll-margin-top: 3.25rem;
	}
	.no-offer {
		display: grid;
		justify-items: start;
		gap: 0.6rem;
		padding: 0.9rem 1rem;
		border: 1px dashed var(--line);
		border-radius: 12px;
	}
	.no-offer p {
		margin: 0;
	}
	.no-offer .label {
		margin-right: 0.4rem;
	}
	.routing {
		display: grid;
		gap: 0.5rem;
		padding: 0.9rem 1rem;
		border: 1px solid var(--line);
		border-radius: 12px;
		background: var(--panel);
	}
	.routing h3 {
		margin: 0;
	}
	.routing-head {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem 1rem;
	}
	.fit {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		font-family: var(--font-mono);
		font-size: 0.75rem;
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}
	.clashes {
		list-style: none;
		padding: 0;
		display: grid;
		gap: 0.6rem;
	}
	.clashes li {
		display: grid;
		grid-template-columns: 7rem 1fr;
		gap: 0.15rem 0.75rem;
	}
	.clashes .where,
	.clashes .why {
		grid-column: 2;
		color: var(--muted);
	}
	.clashes .where {
		font-family: var(--font-mono);
		font-size: 0.8rem;
	}
	.threat {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		font-family: var(--font-mono);
		font-size: 0.75rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--muted);
	}
	.kind {
		color: var(--accent-text);
		font-family: var(--font-mono);
		font-size: 0.75rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}
	details summary {
		cursor: pointer;
		color: var(--muted);
		font-family: var(--font-mono);
		font-size: 0.85rem;
	}
	td code,
	a.ref {
		font-family: var(--font-mono);
		color: var(--accent-text);
	}
	a.ref {
		display: inline-block;
		min-width: 2.75rem;
		padding: 0.15rem 0;
	}
	table {
		width: 100%;
		margin-top: 0.6rem;
		border-collapse: collapse;
		font-size: 0.9rem;
	}
	th,
	td {
		text-align: left;
		padding: 0.4rem;
		border-bottom: 1px solid var(--line);
		vertical-align: top;
	}
	@media print {
		.jump {
			display: none;
		}
		/* Block flow fragments across pages more predictably than grid. */
		.memo {
			display: block;
		}
		.memo > * + * {
			margin-top: 1.25rem;
		}
		.download {
			display: none;
		}
		.tiers,
		.verdict {
			print-color-adjust: exact;
		}
		/* The trace isn't printed, so refs are plain labels on paper. */
		a.ref {
			text-decoration: none;
		}
		section,
		.picks > div,
		.grid > div,
		.partners li,
		.clashes li,
		tr {
			break-inside: avoid;
		}
		h3,
		summary {
			break-after: avoid;
		}
		details summary {
			list-style: none;
			color: var(--text);
			font-weight: 600;
		}
		details summary::-webkit-details-marker {
			display: none;
		}
		.colophon {
			display: block;
			padding-top: 0.6rem;
			border-top: 1px solid var(--line);
			color: var(--muted);
			font-size: 0.75rem;
		}
	}
	@media (max-width: 520px) {
		.partners li,
		.clashes li {
			grid-template-columns: 1fr;
		}
		.partners li span:last-child,
		.clashes .where,
		.clashes .why {
			grid-column: 1;
		}
	}
</style>
