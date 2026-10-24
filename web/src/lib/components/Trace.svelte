<script lang="ts">
	import type { AgentEvent, ToolResult, TraceEvent } from '#lib/types.ts';

	let { events, running }: { events: TraceEvent[]; running: boolean } = $props();

	// Results arrive separately from calls; index them so each call row can show its outcome.
	let results = $derived(
		new Map(
			events
				.filter((e): e is Extract<AgentEvent, { type: 'tool_result' }> & TraceEvent => e.type === 'tool_result')
				.map((e) => [e.data.id, e.data] as [string, ToolResult])
		)
	);
	let calls = $derived(events.filter((e) => e.type === 'tool_call').length);
	let done = $derived(events.some((e) => e.type === 'done'));
	let failed = $derived(!running && !done && events.some((e) => e.type === 'error'));

	const labels: Record<string, string> = {
		resolve_entity: 'Resolve entity',
		find_genre_tags: 'Find genre tags',
		discover_artists: 'Discover local artists',
		score_shortlist: 'Score shortlist',
		local_heatmap: 'Neighborhood heatmap',
		momentum: 'Trend momentum',
		compare_audiences: 'Compare audiences',
		audience_demographics: 'Audience demographics',
		co_promo_partners: 'Co-promo partners',
		submit_booking_memo: 'Write booking memo'
	};

	function brief(input: unknown): string {
		const s = JSON.stringify(input ?? {});
		return s.length > 140 ? s.slice(0, 140) + '…' : s;
	}

	// Timecode like a show clock: mm:ss.s since the run started.
	function clock(ms: number): string {
		const s = ms / 1000;
		return `${String(Math.floor(s / 60)).padStart(2, '0')}:${(s % 60).toFixed(1).padStart(4, '0')}`;
	}
</script>

<div class="head">
	<span class="led" class:live={running} class:ok={done} class:bad={failed}></span>
	<h2>Run of show</h2>
	<span class="state">{running ? 'agent working' : done ? 'memo ready' : failed ? 'stopped' : 'standby'}</span>
	<span class="count">{calls} {calls === 1 ? 'call' : 'calls'}</span>
</div>

<ol class="trace" aria-live="polite">
	{#each events as e, i (i)}
		{#if e.type === 'tool_call'}
			{@const r = results.get(e.data.id)}
			<li class="row call">
				<span class="time">{clock(e.at)}</span>
				<span class="led" class:ok={r?.ok} class:bad={r && !r.ok} class:live={!r && running}></span>
				<div class="main">
					<strong>{labels[e.data.name] ?? e.data.name}</strong>
					<code>{brief(e.data.input)}</code>
					{#if r?.error}<p class="error">{r.error}</p>{/if}
				</div>
				{#if r}
					<span class="tag" class:ok={r.ok} class:bad={!r.ok}>{r.ok ? `${r.duration_ms} ms` : 'Fail'}</span>
				{:else}
					<span class="tag live">{running ? 'Live' : '—'}</span>
				{/if}
			</li>
		{:else if e.type === 'progress' || e.type === 'text'}
			<li class="row note">
				<span class="time">{clock(e.at)}</span>
				<span></span>
				<p class="main">{e.data}</p>
			</li>
		{:else if e.type === 'error'}
			<li class="row note error">
				<span class="time">{clock(e.at)}</span>
				<span class="led bad"></span>
				<p class="main">{e.data.message}</p>
			</li>
		{:else if e.type === 'done'}
			<li class="row note done">
				<span class="time">{clock(e.at)}</span>
				<span class="led ok"></span>
				<p class="main">Memo ready{e.data ? ` after ${e.data.turns} turns` : ''}.</p>
			</li>
		{/if}
	{/each}
	{#if running}
		<li class="row note thinking">
			<span class="time"></span>
			<span class="led live"></span>
			<p class="main">Agent is working…</p>
		</li>
	{/if}
</ol>

<style>
	.head {
		display: flex;
		align-items: center;
		gap: 0.7rem;
		margin-bottom: 0.75rem;
	}
	h2 {
		margin: 0;
		font-family: var(--font-display);
		font-weight: 400;
		font-size: 0.95rem;
		letter-spacing: 0.12em;
		text-transform: uppercase;
	}
	.state,
	.count {
		font-family: var(--font-mono);
		font-size: 0.8rem;
		color: var(--muted);
	}
	.count {
		margin-left: auto;
	}
	.trace {
		list-style: none;
		margin: 0;
		padding: 0;
		border-top: 1px solid var(--line);
	}
	.row {
		display: grid;
		grid-template-columns: 4.2rem 0.6rem minmax(0, 1fr) auto;
		align-items: baseline;
		gap: 0.75rem;
		padding: 0.6rem 0.25rem;
		border-bottom: 1px solid var(--line);
	}
	.row .led {
		align-self: center;
	}
	.call:has(.tag.live) {
		background: color-mix(in srgb, var(--accent) 7%, transparent);
	}
	.time {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--muted);
	}
	.main {
		margin: 0;
		min-width: 0;
	}
	strong {
		font-weight: 600;
		font-size: 0.92rem;
	}
	code {
		display: block;
		margin-top: 0.15rem;
		font-family: var(--font-mono);
		color: var(--muted);
		font-size: 0.72rem;
		overflow-wrap: anywhere;
	}
	.tag {
		font-family: var(--font-mono);
		font-size: 0.72rem;
		font-weight: 500;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		white-space: nowrap;
		color: var(--muted);
	}
	.tag.ok {
		color: var(--ok);
	}
	.tag.bad {
		color: var(--bad);
	}
	.tag.live {
		color: var(--accent-text);
	}
	.note .main {
		grid-column: 3 / -1;
		color: var(--muted);
		font-size: 0.88rem;
	}
	.note.done .main {
		color: var(--ok);
	}
	.error {
		font-family: var(--font-mono);
		font-size: 0.75rem;
	}
	.error,
	.note.error .main {
		color: var(--bad);
		margin: 0.25rem 0 0;
	}
</style>
