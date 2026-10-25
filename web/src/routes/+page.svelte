<script lang="ts">
	import { onMount } from 'svelte';
	import { getHealth, getVenues, runAgent } from '#lib/api.ts';
	import type { Health, Memo, Mode, TraceEvent, Venue } from '#lib/types.ts';
	import Trace from '#lib/components/Trace.svelte';
	import MemoView from '#lib/components/MemoView.svelte';
	import VenueMap from '#lib/components/VenueMap.svelte';
	import Logo from '#lib/components/Logo.svelte';

	let health = $state<Health | null>(null);
	let venues = $state<Venue[]>([]);
	let venueId = $state('');
	let mode = $state<Mode>('fill_date');
	let date = $state('');
	let offerArtist = $state('');
	let notes = $state('');

	const scenarioNote = {
		full: '',
		sparse: 'thin coverage, many artists missing heatmap, trend or audience data',
		degraded: 'heatmap down and some Qloo calls failing',
		empty: 'every Qloo call returns nothing'
	};

	let events = $state<TraceEvent[]>([]);
	// The mode of the run on screen, which the form may have moved on from.
	let runMode = $state<Mode>('fill_date');
	let memo = $state<Memo | null>(null);
	let running = $state(false);
	let error = $state('');
	let controller: AbortController | null = null;

	let venue = $derived(venues.find((v) => v.id === venueId));
	let geo = $derived(events.flatMap((e) => (e.type === 'geo' ? [e.data] : [])));
	let trends = $derived(events.flatMap((e) => (e.type === 'trend' ? [e.data] : [])));
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
		runMode = mode;
		error = '';
		running = true;
		controller = new AbortController();
		const t0 = performance.now();
		try {
			await runAgent(
				{ venue_id: venueId, mode, date: date || undefined, offer_artist: offerArtist || undefined, notes: notes || undefined },
				(e) => {
					events.push({ ...e, at: performance.now() - t0 });
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
		<div class="brand">
			<h1><Logo /></h1>
			<p class="tag">Live Nation has a data team. Your room has Blasting Room.</p>
		</div>
		{#if health}
			<div class="status">
				<span class="lamp">
					<span class="led" class:ok={health.qloo_mode === 'live'} class:on={health.qloo_mode === 'mock'}></span>
					Qloo {health.qloo_mode === 'mock' ? 'mock' : 'live'}
				</span>
				<span class="lamp">
					<span class="led" class:ok={health.agent_ready} class:bad={!health.agent_ready}></span>
					Agent {health.agent_ready ? 'ready' : 'off'}
				</span>
			</div>
		{/if}
	</header>

	{#if health?.qloo_mode === 'mock'}
		<p class="banner">
			<span class="led on"></span>
			Running on synthetic placeholder data. Results show the workflow, not real taste insights.
			{#if health.mock_scenario && health.mock_scenario !== 'full'}
				<span class="scenario">Scenario: {health.mock_scenario} · {scenarioNote[health.mock_scenario]}</span>
			{/if}
		</p>
	{/if}

	<form class="setup" onsubmit={start}>
		<fieldset class="venues">
			<legend class="label">Your room</legend>
			{#each venues as v, i (v.id)}
				<label class="venue" class:selected={v.id === venueId}>
					<input type="radio" name="venue" value={v.id} bind:group={venueId} />
					<span class="tape">
						<span class="led" class:on={v.id === venueId}></span>
						Ch {String(i + 1).padStart(2, '0')} · {v.neighborhood}
					</span>
					<span class="body">
						<strong>{v.name}</strong>
						<span>{v.city} · {v.capacity} cap</span>
						<small>{v.vibe}</small>
					</span>
				</label>
			{/each}
		</fieldset>

		<fieldset class="task">
			<legend class="label">What do you need?</legend>
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
				<button type="submit" class="run" disabled={!canRun}>
					<span class="led" class:live={running}></span>
					{running ? 'Working…' : 'Run the booking agent'}
				</button>
				{#if running}<button type="button" class="ghost" onclick={() => controller?.abort()}>Stop</button>{/if}
			</div>
		</fieldset>
	</form>

	{#if error}<p class="error" role="alert">{error}</p>{/if}

	{#if events.length || running}
		<main class="results">
			<section class="trace-col">
				<Trace {events} {running} />
			</section>
			<section class="memo-col">
				{#if memo && venue}
					<MemoView {memo} {venue} {geo} {trends} mode={runMode} />
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
		gap: 1.5rem;
	}
	.top {
		display: flex;
		flex-wrap: wrap;
		justify-content: space-between;
		align-items: end;
		gap: 1rem;
		padding-bottom: 1.25rem;
		border-bottom: 1px solid var(--line);
	}
	h1 {
		margin: 0;
		font-size: inherit;
		font-weight: 400;
	}
	.tag {
		margin: 0.5rem 0 0;
		color: var(--muted);
	}
	.status {
		display: flex;
		gap: 1.25rem;
		flex-wrap: wrap;
	}
	.lamp {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		font-family: var(--font-mono);
		font-size: 0.8rem;
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}
	.banner {
		margin: 0;
		display: flex;
		align-items: center;
		gap: 0.7rem;
		padding: 0.6rem 0.9rem;
		border-radius: 10px;
		border: 1px solid var(--line);
		background: var(--panel);
		font-size: 0.9rem;
	}
	.scenario {
		font-family: var(--font-mono);
		font-size: 0.8rem;
		color: var(--accent-text);
	}
	fieldset {
		border: 1px solid var(--line);
		border-radius: 14px;
		padding: 1.1rem;
		margin: 0;
		min-width: 0;
		background: var(--panel);
	}
	legend {
		padding: 0 0.4rem;
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
		border: 1px solid var(--line);
		border-radius: 10px;
		overflow: hidden;
		cursor: pointer;
		background: var(--recess);
		transition: border-color 0.15s ease, box-shadow 0.15s ease;
	}
	.venue .tape {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.venue .body {
		display: grid;
		gap: 0.2rem;
		padding: 0.8rem 0.9rem 0.9rem;
	}
	.venue strong {
		font-family: var(--font-display);
		font-weight: 400;
		font-size: 0.95rem;
		letter-spacing: 0.04em;
		text-transform: uppercase;
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
	.venue strong {
		color: var(--text);
	}
	.venue .tape {
		color: var(--on-tape);
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
		display: inline-flex;
		flex-wrap: wrap;
		gap: 4px;
		padding: 4px;
		margin-bottom: 1rem;
		background: var(--recess);
		border: 1px solid var(--line);
		border-radius: 10px;
	}
	.modes label {
		padding: 0.5rem 0.9rem;
		border-radius: 7px;
		cursor: pointer;
		font-family: var(--font-mono);
		font-size: 0.85rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--muted);
	}
	.modes label.selected {
		background: var(--line);
		color: var(--text);
		border-color: transparent !important;
		box-shadow: none;
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
		background: var(--recess);
		border: 1px solid var(--line);
		border-radius: 8px;
		padding: 0.55rem 0.65rem;
		min-width: 0;
	}
	input:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 1px;
	}
	.actions {
		display: flex;
		gap: 0.6rem;
		margin-top: 1.1rem;
	}
	button {
		font: inherit;
		min-height: 44px;
		padding: 0 1.3rem;
		border-radius: 8px;
		cursor: pointer;
	}
	button.run {
		display: inline-flex;
		align-items: center;
		gap: 0.65rem;
		font-family: var(--font-display);
		font-size: 0.8rem;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		border: 0;
		background: var(--accent);
		color: var(--on-accent);
		box-shadow: inset 0 -3px 0 rgb(0 0 0 / 0.25);
	}
	button.run .led {
		background: var(--on-accent);
		box-shadow: none;
	}
	button:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}
	button.ghost {
		font-family: var(--font-mono);
		font-size: 0.85rem;
		background: transparent;
		color: var(--text);
		border: 1px solid var(--line);
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
	.memo-col {
		display: grid;
		gap: 1rem;
	}
	.placeholder {
		padding: 3rem 1rem;
		text-align: center;
		color: var(--muted);
		border: 1px dashed var(--line);
		border-radius: 14px;
		background: var(--recess);
	}
	footer {
		color: var(--muted);
		font-size: 0.8rem;
	}
	footer a {
		color: inherit;
	}
	/* Print the memo alone. */
	@media print {
		.top,
		.banner,
		.setup,
		.error,
		.trace-col,
		footer {
			display: none;
		}
		.page {
			display: block;
			max-width: none;
			padding: 0;
		}
		.results {
			display: block;
		}
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
