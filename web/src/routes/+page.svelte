<script lang="ts">
	import { onMount } from 'svelte';
	import { getHealth, getVenues, runAgent } from '#lib/api.ts';
	import type { AgentEvent, Health, Memo, Mode, Venue } from '#lib/types.ts';
	import Trace from '#lib/components/Trace.svelte';
	import MemoView from '#lib/components/MemoView.svelte';
	import VenueMap from '#lib/components/VenueMap.svelte';

	let health = $state<Health | null>(null);
	let venues = $state<Venue[]>([]);
	let venueId = $state('');
	let mode = $state<Mode>('fill_date');
	let date = $state('');
	let offerArtist = $state('');
	let notes = $state('');

	let events = $state<AgentEvent[]>([]);
	let memo = $state<Memo | null>(null);
	let running = $state(false);
	let error = $state('');
	let controller: AbortController | null = null;

	let venue = $derived(venues.find((v) => v.id === venueId));
	let geo = $derived(events.flatMap((e) => (e.type === 'geo' ? [e.data] : [])));
	let canRun = $derived(
		!!health?.agent_ready && !!venue && !running && (mode === 'fill_date' || offerArtist.trim() !== '')
	);

	onMount(async () => {
		try {
			[health, venues] = await Promise.all([getHealth(), getVenues()]);
			venueId = venues[0]?.id ?? '';
		} catch (e) {
			error = `Could not reach the Blasting Room API (${(e as Error).message}).`;
		}
	});

	async function start(ev: SubmitEvent) {
		ev.preventDefault();
		if (!canRun) return;
		events = [];
		memo = null;
		error = '';
		running = true;
		controller = new AbortController();
		try {
			await runAgent(
				{ venue_id: venueId, mode, date: date || undefined, offer_artist: offerArtist || undefined, notes: notes || undefined },
				(e) => {
					events.push(e);
					if (e.type === 'memo') memo = e.data;
					if (e.type === 'error') error = e.data.message;
				},
				controller.signal
			);
		} catch (e) {
			if ((e as Error).name !== 'AbortError') error = (e as Error).message;
		} finally {
			running = false;
			controller = null;
		}
	}
</script>

<svelte:head>
	<title>Blasting Room · booking agent for independent venues</title>
	<meta name="description" content="An AI talent buyer grounded in Qloo's cultural taste graph." />
</svelte:head>

<div class="page">
	<header class="top">
		<div>
			<h1>Blasting Room</h1>
			<p class="tag">Live Nation has a data team. Your room has Blasting Room.</p>
		</div>
		{#if health}
			<div class="status">
				<span class="pill" class:warn={health.qloo_mode === 'mock'}>
					Qloo: {health.qloo_mode === 'mock' ? 'mock data' : 'live'}
				</span>
				<span class="pill" class:warn={!health.agent_ready}>
					Agent: {health.agent_ready ? 'ready' : 'not configured'}
				</span>
			</div>
		{/if}
	</header>

	{#if health?.qloo_mode === 'mock'}
		<p class="banner">
			Running on synthetic placeholder data. Results show the workflow, not real taste insights.
		</p>
	{/if}

	<form class="setup" onsubmit={start}>
		<fieldset class="venues">
			<legend>Your room</legend>
			{#each venues as v (v.id)}
				<label class="venue" class:selected={v.id === venueId}>
					<input type="radio" name="venue" value={v.id} bind:group={venueId} />
					<strong>{v.name}</strong>
					<span>{v.neighborhood}, {v.city} · {v.capacity} cap</span>
					<small>{v.vibe}</small>
				</label>
			{/each}
		</fieldset>

		<fieldset class="task">
			<legend>What do you need?</legend>
			<div class="modes" role="radiogroup">
				<label class:selected={mode === 'fill_date'}>
					<input type="radio" name="mode" value="fill_date" bind:group={mode} /> Fill an open date
				</label>
				<label class:selected={mode === 'vet_offer'}>
					<input type="radio" name="mode" value="vet_offer" bind:group={mode} /> Vet an offer
				</label>
			</div>
			<div class="fields">
				{#if mode === 'vet_offer'}
					<label>Artist offered <input bind:value={offerArtist} placeholder="e.g. the band an agent pitched you" required /></label>
				{/if}
				<label>Date <input type="date" bind:value={date} /></label>
				<label class="wide">Notes for the agent <input bind:value={notes} placeholder="e.g. Thursday, want a younger crowd, under $20 tickets" /></label>
			</div>
			<div class="actions">
				<button type="submit" disabled={!canRun}>{running ? 'Working…' : 'Run the booking agent'}</button>
				{#if running}<button type="button" class="ghost" onclick={() => controller?.abort()}>Stop</button>{/if}
			</div>
		</fieldset>
	</form>

	{#if error}<p class="error" role="alert">{error}</p>{/if}

	{#if events.length || running}
		<main class="results">
			<section class="trace-col">
				<h2>Agent trace</h2>
				<Trace {events} {running} />
			</section>
			<section class="memo-col">
				{#if memo && venue}
					<MemoView {memo} {venue} {geo} />
				{:else}
					{#if venue && geo.length}
						<VenueMap {venue} {geo} />
					{/if}
					<div class="placeholder">The booking memo appears here when the agent finishes.</div>
				{/if}
			</section>
		</main>
	{/if}

	<footer>
		Taste data from <a href="https://www.qloo.com" rel="noopener">Qloo</a>. Aggregate taste signals only, no fan personal data.
		Open source under the MIT license.
	</footer>
</div>

<style>
	.page {
		max-width: 1200px;
		margin: 0 auto;
		padding: 1.5rem 1rem 3rem;
		display: grid;
		gap: 1.25rem;
	}
	.top {
		display: flex;
		flex-wrap: wrap;
		justify-content: space-between;
		align-items: end;
		gap: 1rem;
	}
	h1 {
		margin: 0;
		font-size: clamp(2rem, 5vw, 3rem);
		letter-spacing: -0.03em;
	}
	.tag {
		margin: 0.2rem 0 0;
		color: var(--muted);
	}
	.status {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
	}
	.pill {
		padding: 0.2rem 0.7rem;
		border-radius: 999px;
		border: 1px solid var(--ok);
		color: var(--ok);
		font-size: 0.8rem;
	}
	.pill.warn {
		border-color: var(--accent);
		color: var(--accent);
	}
	.banner {
		margin: 0;
		padding: 0.6rem 0.9rem;
		border-radius: 8px;
		border: 1px dashed var(--accent);
		color: var(--accent);
		font-size: 0.9rem;
	}
	fieldset {
		border: 1px solid var(--line);
		border-radius: 10px;
		padding: 1rem;
		margin: 0;
		min-width: 0;
	}
	legend {
		padding: 0 0.4rem;
		color: var(--muted);
		font-size: 0.85rem;
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}
	.setup {
		display: grid;
		gap: 1rem;
	}
	.venues {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(15rem, 1fr));
		gap: 0.75rem;
	}
	.venue {
		display: grid;
		gap: 0.2rem;
		padding: 0.8rem 0.9rem;
		border: 1px solid var(--line);
		border-radius: 8px;
		cursor: pointer;
		background: var(--panel);
	}
	.venue input,
	.modes input {
		position: absolute;
		opacity: 0;
		pointer-events: none;
	}
	.venue span,
	.venue small {
		color: var(--muted);
	}
	.selected {
		border-color: var(--accent) !important;
		box-shadow: 0 0 0 1px var(--accent);
	}
	.venue:has(input:focus-visible),
	.modes label:has(input:focus-visible) {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}
	.modes {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
		margin-bottom: 0.9rem;
	}
	.modes label {
		padding: 0.45rem 0.9rem;
		border: 1px solid var(--line);
		border-radius: 999px;
		cursor: pointer;
	}
	.fields {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(12rem, 1fr));
		gap: 0.75rem;
	}
	.fields label {
		display: grid;
		gap: 0.3rem;
		font-size: 0.85rem;
		color: var(--muted);
	}
	.fields .wide {
		grid-column: 1 / -1;
	}
	input {
		font: inherit;
		color: var(--text);
		background: var(--bg);
		border: 1px solid var(--line);
		border-radius: 6px;
		padding: 0.5rem 0.6rem;
		min-width: 0;
	}
	.actions {
		display: flex;
		gap: 0.6rem;
		margin-top: 1rem;
	}
	button {
		font: inherit;
		font-weight: 600;
		padding: 0.6rem 1.2rem;
		border-radius: 8px;
		border: 1px solid var(--accent);
		background: var(--accent);
		color: var(--on-accent);
		cursor: pointer;
	}
	button:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}
	button.ghost {
		background: transparent;
		color: var(--text);
		border-color: var(--line);
	}
	.error {
		margin: 0;
		color: var(--bad);
	}
	.results {
		display: grid;
		grid-template-columns: minmax(0, 2fr) minmax(0, 3fr);
		gap: 1.5rem;
		align-items: start;
	}
	.results h2 {
		font-size: 0.85rem;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--muted);
		margin: 0 0 0.6rem;
	}
	.memo-col {
		display: grid;
		gap: 1rem;
	}
	.placeholder {
		padding: 3rem 1rem;
		text-align: center;
		color: var(--muted);
		border: 1px dashed var(--line);
		border-radius: 10px;
	}
	footer {
		color: var(--muted);
		font-size: 0.8rem;
	}
	footer a {
		color: inherit;
	}
	@media (max-width: 820px) {
		.results {
			grid-template-columns: 1fr;
		}
		.memo-col {
			order: -1;
		}
	}
</style>
