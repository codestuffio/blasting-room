<script lang="ts">
	import type { AgentEvent, ToolResult } from '#lib/types.ts';

	let { events, running }: { events: AgentEvent[]; running: boolean } = $props();

	// Results arrive separately from calls; index them so each call row can show its outcome.
	let results = $derived(
		new Map(
			events
				.filter((e): e is Extract<AgentEvent, { type: 'tool_result' }> => e.type === 'tool_result')
				.map((e) => [e.data.id, e.data] as [string, ToolResult])
		)
	);

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
</script>

<ol class="trace" aria-live="polite">
	{#each events as e, i (i)}
		{#if e.type === 'tool_call'}
			{@const r = results.get(e.data.id)}
			<li class="call" class:err={r && !r.ok} class:pending={!r}>
				<span class="dot" aria-hidden="true"></span>
				<div>
					<div class="row">
						<strong>{labels[e.data.name] ?? e.data.name}</strong>
						{#if r}
							<span class="meta">{r.ok ? `${r.duration_ms} ms` : 'failed'}</span>
						{:else}
							<span class="meta">running…</span>
						{/if}
					</div>
					<code>{brief(e.data.input)}</code>
					{#if r?.error}<p class="error">{r.error}</p>{/if}
				</div>
			</li>
		{:else if e.type === 'progress' || e.type === 'text'}
			<li class="note"><p>{e.data}</p></li>
		{:else if e.type === 'error'}
			<li class="note error"><p>{e.data.message}</p></li>
		{:else if e.type === 'done'}
			<li class="note done"><p>Memo ready{e.data ? ` after ${e.data.turns} turns` : ''}.</p></li>
		{/if}
	{/each}
	{#if running}
		<li class="note thinking"><p>Agent is working…</p></li>
	{/if}
</ol>

<style>
	.trace {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		gap: 0.5rem;
	}
	.call {
		display: grid;
		grid-template-columns: 0.75rem 1fr;
		gap: 0.6rem;
		padding: 0.6rem 0.75rem;
		border: 1px solid var(--line);
		border-radius: 8px;
		background: var(--panel);
	}
	.dot {
		width: 0.6rem;
		height: 0.6rem;
		margin-top: 0.35rem;
		border-radius: 50%;
		background: var(--ok);
	}
	.pending .dot {
		background: var(--accent);
		animation: pulse 1s infinite alternate;
	}
	.err .dot {
		background: var(--bad);
	}
	.row {
		display: flex;
		justify-content: space-between;
		gap: 1rem;
	}
	.meta {
		color: var(--muted);
		font-size: 0.8rem;
		white-space: nowrap;
	}
	code {
		display: block;
		margin-top: 0.25rem;
		color: var(--muted);
		font-size: 0.75rem;
		overflow-wrap: anywhere;
	}
	.note p {
		margin: 0;
		padding: 0.25rem 0.75rem;
		color: var(--muted);
		font-size: 0.9rem;
		border-left: 2px solid var(--line);
	}
	.note.done p {
		color: var(--ok);
		border-color: var(--ok);
	}
	.error,
	.note.error p {
		color: var(--bad);
		border-color: var(--bad);
		margin: 0.25rem 0 0;
	}
	.thinking p {
		animation: pulse 1.2s infinite alternate;
	}
	@keyframes pulse {
		from {
			opacity: 0.45;
		}
		to {
			opacity: 1;
		}
	}
</style>
