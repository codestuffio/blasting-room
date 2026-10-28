<script lang="ts">
	import { onMount } from 'svelte';
	import type { TraceEvent } from '#lib/types.ts';

	// What the agent is doing, in a buyer's words, while the memo is being
	// written. Each stage lights from the tool calls that belong to it.
	let { events, running }: { events: TraceEvent[]; running: boolean } = $props();

	const stages: { label: string; tools: string[] }[] = [
		{ label: 'Read the room', tools: ['resolve_entity', 'find_genre_tags'] },
		{ label: 'Find candidates', tools: ['discover_artists', 'score_shortlist'] },
		{ label: 'Check local fit', tools: ['local_heatmap', 'audience_demographics', 'momentum'] },
		{ label: 'Tour and competing shows', tools: ['tour_dates', 'competing_shows'] },
		{ label: 'Pick the support act', tools: ['compare_audiences'] },
		{ label: 'Price the offer', tools: ['price_deal'] },
		{ label: 'Plan the co-promo', tools: ['co_promo_partners'] }
	];

	let status = $derived.by(() => {
		const calls = new Map<string, string>(); // call id → tool
		const done = new Set<string>();
		for (const e of events) {
			if (e.type === 'tool_call') calls.set(e.data.id, e.data.name);
			if (e.type === 'tool_result') done.add(e.data.id);
		}
		return stages.map((s) => {
			const ids = [...calls].filter(([, tool]) => s.tools.includes(tool)).map(([id]) => id);
			const pending = ids.filter((id) => !done.has(id)).length;
			return { ...s, calls: ids.length, state: !ids.length ? 'idle' : pending ? 'live' : 'done' };
		});
	});
	let stopped = $derived(!running && !events.some((e) => e.type === 'done'));
	// The agent's latest narration, trimmed to a line or two.
	let note = $derived.by(() => {
		const last = events.findLast((e) => e.type === 'progress' || e.type === 'text');
		if (!last || (last.type !== 'progress' && last.type !== 'text')) return '';
		const s = last.data.replace(/\s+/g, ' ').trim();
		return s.length > 220 ? s.slice(0, 217) + '…' : s;
	});

	let start = Date.now();
	let now = $state(Date.now());
	onMount(() => {
		start = Date.now();
		const id = setInterval(() => {
			if (running) now = Date.now();
		}, 250);
		return () => clearInterval(id);
	});
	let clock = $derived.by(() => {
		const s = Math.max(0, Math.floor((now - start) / 1000));
		return `${String(Math.floor(s / 60)).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`;
	});
</script>

<section class="sheet" aria-live="polite">
	<div class="head">
		<span class="led" class:live={running} class:bad={stopped}></span>
		<h3>{stopped ? 'Run stopped' : 'Working the room'}</h3>
		<span class="clock" aria-label="Elapsed time">{clock}</span>
	</div>
	<ol>
		{#each status as s (s.label)}
			<li class={s.state}>
				<span class="led" class:live={s.state === 'live' && running} class:ok={s.state === 'done'}></span>
				<span class="label">{s.label}</span>
				<span class="count">{s.calls ? `${s.calls} ${s.calls === 1 ? 'call' : 'calls'}` : ''}</span>
			</li>
		{/each}
	</ol>
	{#if note}<p class="note">{note}</p>{/if}
	{#if running}<p class="hint">A full run usually takes about a minute. The memo opens here when it's done.</p>{/if}
</section>

<style>
	.sheet {
		display: grid;
		gap: 0.75rem;
		padding: 1rem 1.1rem;
		border: 1px solid var(--line);
		border-radius: 14px;
		background: var(--panel);
	}
	.head {
		display: flex;
		align-items: center;
		gap: 0.65rem;
	}
	h3 {
		margin: 0;
		font-family: var(--font-display);
		font-weight: 400;
		font-size: 0.9rem;
		letter-spacing: 0.1em;
		text-transform: uppercase;
	}
	.clock {
		margin-left: auto;
		font-family: var(--font-mono);
		font-size: 1.1rem;
		font-variant-numeric: tabular-nums;
		color: var(--accent-text);
	}
	ol {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		gap: 2px;
		background: var(--line);
		border: 1px solid var(--line);
		border-radius: 10px;
		overflow: hidden;
	}
	li {
		display: grid;
		grid-template-columns: 0.6rem 1fr auto;
		align-items: center;
		gap: 0.75rem;
		padding: 0.55rem 0.8rem;
		background: var(--recess);
		font-size: 0.9rem;
	}
	li.idle .label {
		color: var(--muted);
	}
	li.live {
		background: color-mix(in srgb, var(--accent) 10%, var(--recess));
	}
	.count {
		font-family: var(--font-mono);
		font-size: 0.72rem;
		color: var(--muted);
	}
	.note {
		margin: 0;
		padding-left: 0.75rem;
		border-left: 2px solid var(--accent);
		color: var(--text);
		font-size: 0.9rem;
	}
	.hint {
		margin: 0;
		color: var(--muted);
		font-size: 0.8rem;
	}
</style>
