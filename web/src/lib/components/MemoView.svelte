<script lang="ts">
	import { flushSync } from 'svelte';
	import type { Geo, Memo, Venue } from '#lib/types.ts';
	import VenueMap from '#lib/components/VenueMap.svelte';

	let { memo, venue, geo = [] }: { memo: Memo; venue: Venue; geo?: Geo[] } = $props();

	let names = $derived(
		Object.fromEntries(
			[memo.headliner, memo.support, ...memo.alternates].map((p) => [p.entity_id, p.name])
		)
	);
	let partnerNames = $derived(memo.co_promo.filter((c) => c.kind === 'place').map((c) => c.name));

	const kindLabel = { place: 'Nearby spot', podcast: 'Podcast', brand: 'Brand' };

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
		document.title = `Booking memo - ${memo.headliner.name} with ${memo.support.name} - ${venue.name}`;
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
	<header>
		<p class="eyebrow">Booking memo · {venue.name}, {venue.neighborhood}</p>
		<h2>{memo.headliner.name} <span class="with">with</span> {memo.support.name}</h2>
		<p class="summary">{memo.summary}</p>
		<div class="meta">
			<span class="badge {memo.confidence}">{memo.confidence} confidence</span>
			<button type="button" class="download" onclick={download} disabled={preparing}>
				{preparing ? 'Preparing…' : 'Download PDF'}
			</button>
		</div>
	</header>

	<section class="picks">
		{#each [{ role: 'Headliner', pick: memo.headliner }, { role: 'Support', pick: memo.support }] as p (p.role)}
			<div>
				<h3>{p.role}: {p.pick.name}</h3>
				<ul>
					{#each p.pick.rationale as r, i (i)}<li>{r}</li>{/each}
				</ul>
			</div>
		{/each}
	</section>

	{#if geo.length}
		<section>
			<h3>Where the fans are</h3>
			<VenueMap bind:this={mapView} {venue} {geo} {names} focus={memo.headliner.entity_id} highlight={partnerNames} />
		</section>
	{/if}

	<section class="grid">
		<div>
			<h3>Why it works in this room</h3>
			<p>{memo.local_fit}</p>
		</div>
		<div>
			<h3>Momentum</h3>
			<p>{memo.momentum}</p>
		</div>
		<div>
			<h3>Who's coming</h3>
			<p>{memo.audience.summary}</p>
			<p class="muted">Age skew: {memo.audience.age_skew}</p>
			<p class="muted">Suggested ticket tier: <strong>{memo.audience.ticket_tier}</strong></p>
		</div>
	</section>

	{#if memo.co_promo.length}
		<section>
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

	<section class="grid">
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
		<details bind:open={evidenceOpen}>
			<summary>Evidence trail ({memo.evidence.length} claims)</summary>
			<table>
				<thead><tr><th>Claim</th><th>Source tool</th></tr></thead>
				<tbody>
					{#each memo.evidence as ev, i (i)}
						<tr><td>{ev.claim}</td><td><code>{ev.tool}</code></td></tr>
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
	.eyebrow {
		margin: 0;
		color: var(--accent);
		font-size: 0.8rem;
		letter-spacing: 0.08em;
		text-transform: uppercase;
	}
	h2 {
		margin: 0.3rem 0;
		font-size: clamp(1.5rem, 3vw, 2.1rem);
		line-height: 1.15;
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
		margin: 0 0 0.4rem;
		font-size: 0.95rem;
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
	.meta {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 0.6rem;
	}
	.download {
		font: inherit;
		font-size: 0.85rem;
		font-weight: 600;
		padding: 0.35rem 0.9rem;
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
	.badge {
		display: inline-block;
		padding: 0.15rem 0.6rem;
		border-radius: 999px;
		font-size: 0.8rem;
		border: 1px solid currentColor;
	}
	.badge.high {
		color: var(--ok);
	}
	.badge.medium {
		color: var(--accent);
	}
	.badge.low {
		color: var(--bad);
	}
	.picks,
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
		gap: 1rem;
	}
	.picks > div,
	.grid > div {
		padding: 0.9rem 1rem;
		border: 1px solid var(--line);
		border-radius: 8px;
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
	.kind {
		color: var(--accent);
		font-size: 0.8rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}
	details summary {
		cursor: pointer;
		color: var(--muted);
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
		section,
		.picks > div,
		.grid > div,
		.partners li,
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
		.partners li {
			grid-template-columns: 1fr;
		}
		.partners li span:last-child {
			grid-column: 1;
		}
	}
</style>
