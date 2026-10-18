<script lang="ts">
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
</script>

<article class="memo">
	<header>
		<p class="eyebrow">Booking memo · {venue.name}, {venue.neighborhood}</p>
		<h2>{memo.headliner.name} <span class="with">with</span> {memo.support.name}</h2>
		<p class="summary">{memo.summary}</p>
		<span class="badge {memo.confidence}">{memo.confidence} confidence</span>
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
			<VenueMap {venue} {geo} {names} focus={memo.headliner.entity_id} highlight={partnerNames} />
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
		<details>
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
	@media (max-width: 520px) {
		.partners li {
			grid-template-columns: 1fr;
		}
		.partners li span:last-child {
			grid-column: 1;
		}
	}
</style>
