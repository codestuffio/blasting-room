<script lang="ts">
	import type { Trend, TrendPoint } from '#lib/types.ts';

	// picks are the memo's artists in role order (headliner, support, alternates);
	// a pick keeps its color slot whether or not it has data.
	let { trends, picks }: { trends: Trend[]; picks: { entity_id: string; name: string; role: string }[] } = $props();

	// The agent may trend an artist more than once; the latest series wins.
	let byEntity = $derived(new Map(trends.flatMap((t) => t.series.map((s) => [s.entity_id, s.points] as const))));

	let lines = $derived(
		picks
			.slice(0, 5)
			.map((p, slot) => ({ ...p, slot: slot + 1, points: byEntity.get(p.entity_id) ?? [] }))
			.filter((l) => l.points.length > 1)
	);
	let missing = $derived(picks.slice(0, 2).filter((p) => p.entity_id && !(byEntity.get(p.entity_id)?.length ?? 0)));
	// Qloo's hackathon data often repeats one value for the whole window.
	let flat = $derived(lines.length > 0 && lines.every((l) => l.points.every((p) => p.percentile === l.points[0].percentile)));

	// Every date any series has, oldest first: the crosshair snaps to these.
	let dates = $derived([...new Set(lines.flatMap((l) => l.points.map((p) => p.date)))].sort());

	let width = $state(0);
	const height = 220;
	const m = { top: 14, right: 48, bottom: 26, left: 34 };
	let plotW = $derived(Math.max(0, width - m.left - m.right));
	const plotH = height - m.top - m.bottom;

	let t0 = $derived(dates.length ? Date.parse(dates[0]) : 0);
	let t1 = $derived(dates.length ? Date.parse(dates[dates.length - 1]) : 1);
	// The y domain snaps to the tick step so the top and bottom gridlines are labeled.
	let scale = $derived.by(() => {
		const all = lines.flatMap((l) => l.points.map((p) => p.percentile * 100));
		const min = Math.min(...all) - 3;
		const max = Math.max(...all) + 3;
		const step = max - min > 40 ? 20 : 10;
		const lo = Math.max(0, Math.floor(min / step) * step);
		const hi = Math.min(100, Math.ceil(max / step) * step);
		const ticks = [];
		for (let v = lo; v <= hi; v += step) ticks.push(v);
		return { domain: [lo, hi], ticks };
	});
	let domain = $derived(scale.domain);
	let ticks = $derived(scale.ticks);

	const x = (date: string) => m.left + (t1 === t0 ? 0 : ((Date.parse(date) - t0) / (t1 - t0)) * plotW);
	const y = (pct: number) => m.top + plotH - ((pct * 100 - domain[0]) / (domain[1] - domain[0])) * plotH;
	const path = (pts: TrendPoint[]) => pts.map((p, i) => `${i ? 'L' : 'M'}${x(p.date).toFixed(1)},${y(p.percentile).toFixed(1)}`).join('');

	const fmtDate = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric', timeZone: 'UTC' });
	const day = (d: string) => fmtDate.format(new Date(Date.parse(d)));
	const pct = (p: number) => Math.round(p * 100);
	const signed = (n: number) => (n > 0 ? `+${n}` : `${n}`);

	function change(points: TrendPoint[]) {
		return pct(points[points.length - 1].percentile) - pct(points[0].percentile);
	}

	// End labels only where they won't collide; the legend carries every end value anyway.
	let endLabels = $derived.by(() => {
		const ends = lines.map((l) => ({ id: l.entity_id, y: y(l.points[l.points.length - 1].percentile) }));
		return new Set(ends.filter((e) => ends.every((o) => o.id === e.id || Math.abs(o.y - e.y) >= 13)).map((e) => e.id));
	});

	// Hover and keyboard share one index into dates.
	let active = $state<number | null>(null);
	function nearest(points: TrendPoint[], date: string) {
		const t = Date.parse(date);
		return points.reduce((a, b) => (Math.abs(Date.parse(b.date) - t) < Math.abs(Date.parse(a.date) - t) ? b : a));
	}
	function readout(i: number) {
		return `${day(dates[i])}: ${lines.map((l) => `${l.name} p${pct(nearest(l.points, dates[i]).percentile)}`).join(', ')}`;
	}
	function onMove(e: PointerEvent) {
		const rect = (e.currentTarget as SVGElement).getBoundingClientRect();
		const px = e.clientX - rect.left;
		let best = 0;
		dates.forEach((d, i) => {
			if (Math.abs(x(d) - px) < Math.abs(x(dates[best]) - px)) best = i;
		});
		active = best;
	}
	function onKey(e: KeyboardEvent) {
		if (!dates.length) return;
		const last = dates.length - 1;
		const step: Record<string, number> = { ArrowLeft: -1, ArrowRight: 1 };
		if (e.key in step) {
			active = Math.min(last, Math.max(0, (active ?? last) + step[e.key]));
			e.preventDefault();
		} else if (e.key === 'Home' || e.key === 'End') {
			active = e.key === 'Home' ? 0 : last;
			e.preventDefault();
		}
	}
</script>

{#if lines.length}
	<div class="momentum">
		<ul class="legend">
			{#each lines as l (l.entity_id)}
				{@const c = change(l.points)}
				<li>
					<i class="key" style:--c={`var(--series-${l.slot})`} aria-hidden="true"></i>
					<span class="who">{l.name}</span>
					<span class="role">{l.role}</span>
					<span class="val">p{pct(l.points[l.points.length - 1].percentile)} <span class="muted">({signed(c)})</span></span>
				</li>
			{/each}
		</ul>

		<div class="plot" bind:clientWidth={width}>
			{#if width}
				<svg
					{width}
					{height}
					role="slider"
					aria-label={`Popularity percentile over the last 6 months for ${lines.map((l) => l.name).join(', ')}. Use the arrow keys to step through dates.`}
					aria-valuemin={0}
					aria-valuemax={dates.length - 1}
					aria-valuenow={active ?? dates.length - 1}
					aria-valuetext={readout(active ?? dates.length - 1)}
					tabindex="0"
					onpointermove={onMove}
					onpointerleave={() => (active = null)}
					onkeydown={onKey}
					onblur={() => (active = null)}
				>
					{#each ticks as t (t)}
						<line class="grid" x1={m.left} x2={m.left + plotW} y1={y(t / 100)} y2={y(t / 100)} />
						<text class="tick" x={m.left - 6} y={y(t / 100)} text-anchor="end" dominant-baseline="middle">{t}</text>
					{/each}
					{#each [0, Math.floor((dates.length - 1) / 2), dates.length - 1] as i, k (k)}
						<text class="tick" x={x(dates[i])} y={height - 6} text-anchor={k === 0 ? 'start' : k === 2 ? 'end' : 'middle'}>{day(dates[i])}</text>
					{/each}

					{#if active !== null}
						<line class="cross" x1={x(dates[active])} x2={x(dates[active])} y1={m.top} y2={m.top + plotH} />
					{/if}

					<!-- Supporting lines first so the headliner draws on top. -->
					{#each [...lines].reverse() as l (l.entity_id)}
						{@const end = l.points[l.points.length - 1]}
						<path class="line" d={path(l.points)} style:stroke={`var(--series-${l.slot})`} />
						<circle class="dot" cx={x(end.date)} cy={y(end.percentile)} r="4" style:fill={`var(--series-${l.slot})`} />
						{#if endLabels.has(l.entity_id)}
							<text class="end" x={x(end.date) + 8} y={y(end.percentile)} dominant-baseline="middle">{pct(end.percentile)}</text>
						{/if}
						{#if active !== null}
							{@const p = nearest(l.points, dates[active])}
							<circle class="dot" cx={x(p.date)} cy={y(p.percentile)} r="4" style:fill={`var(--series-${l.slot})`} />
						{/if}
					{/each}
				</svg>

				{#if active !== null}
					{@const left = x(dates[active])}
					<div class="tip" class:flip={left > width / 2} style:left={`${left}px`} role="status">
						<p class="date">{day(dates[active])}</p>
						{#each lines as l (l.entity_id)}
							{@const p = nearest(l.points, dates[active])}
							<p>
								<i class="key" style:--c={`var(--series-${l.slot})`} aria-hidden="true"></i>
								<strong>p{pct(p.percentile)}</strong>
								<span class="muted">{l.name}</span>
								{#if p.velocity !== undefined}<span class="muted">· ×{p.velocity.toFixed(2)}</span>{/if}
							</p>
						{/each}
					</div>
				{/if}
			{/if}
		</div>

		<p class="caption muted">
			Qloo popularity percentile over the last 6 months: p80 means more popular than 80% of artists.
			{#if flat}Qloo reports no movement over this window, so it says nothing about momentum.{/if}
			{#if missing.length}No trend data for {missing.map((p) => p.name).join(' or ')}.{/if}
		</p>

		<details class="table">
			<summary>Data table</summary>
			<table>
				<thead>
					<tr>
						<th>Date</th>
						{#each lines as l (l.entity_id)}<th>{l.name}</th>{/each}
					</tr>
				</thead>
				<tbody>
					{#each dates as d (d)}
						<tr>
							<td>{day(d)}</td>
							{#each lines as l (l.entity_id)}
								{@const p = l.points.find((q) => q.date === d)}
								<td>{p ? pct(p.percentile) : '—'}</td>
							{/each}
						</tr>
					{/each}
				</tbody>
			</table>
		</details>
	</div>
{/if}

<style>
	.momentum {
		margin: 0;
		min-width: 0;
		display: grid;
		gap: 0.6rem;
	}
	.legend {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem 1.25rem;
		font-size: 0.9rem;
	}
	.legend li {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
	}
	.role {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--muted);
	}
	.val {
		font-family: var(--font-mono);
		font-size: 0.8rem;
		font-variant-numeric: tabular-nums;
	}
	.key {
		display: inline-block;
		flex: none;
		width: 14px;
		height: 2px;
		border-radius: 1px;
		background: var(--c);
		print-color-adjust: exact;
	}
	/* The svg is drawn at the measured width; taking it out of flow lets the box shrink. */
	.plot {
		position: relative;
		height: 220px;
		min-width: 0;
	}
	svg {
		position: absolute;
		inset: 0 auto auto 0;
		display: block;
		overflow: visible;
		touch-action: pan-y;
	}
	svg:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 4px;
		border-radius: 4px;
	}
	.grid {
		stroke: var(--line);
		stroke-width: 1;
	}
	.cross {
		stroke: var(--muted);
		stroke-width: 1;
	}
	.tick,
	.end {
		fill: var(--muted);
		font-family: var(--font-mono);
		font-size: 11px;
		font-variant-numeric: tabular-nums;
	}
	.end {
		fill: var(--text);
	}
	.line {
		fill: none;
		stroke-width: 2;
		stroke-linejoin: round;
		stroke-linecap: round;
	}
	.dot {
		stroke: var(--panel);
		stroke-width: 2;
	}
	.tip {
		position: absolute;
		top: 0;
		transform: translateX(12px);
		pointer-events: none;
		min-width: 11rem;
		padding: 0.5rem 0.65rem;
		border: 1px solid var(--line);
		border-radius: 8px;
		background: var(--bg);
		box-shadow: 0 6px 18px rgb(0 0 0 / 0.25);
		font-size: 0.8rem;
	}
	.tip.flip {
		transform: translateX(calc(-100% - 12px));
	}
	.tip p {
		margin: 0;
		display: flex;
		align-items: center;
		gap: 0.4rem;
		white-space: nowrap;
	}
	.tip .date {
		margin-bottom: 0.25rem;
		font-family: var(--font-mono);
		color: var(--muted);
	}
	.tip strong {
		font-family: var(--font-mono);
		font-variant-numeric: tabular-nums;
	}
	.muted {
		color: var(--muted);
	}
	.caption {
		margin: 0;
		font-size: 0.85rem;
	}
	.table summary {
		cursor: pointer;
		color: var(--muted);
		font-family: var(--font-mono);
		font-size: 0.8rem;
	}
	table {
		width: 100%;
		margin-top: 0.5rem;
		border-collapse: collapse;
		font-size: 0.85rem;
		font-variant-numeric: tabular-nums;
	}
	th,
	td {
		text-align: left;
		padding: 0.3rem 0.4rem;
		border-bottom: 1px solid var(--line);
	}
	@media print {
		.table,
		.tip,
		.cross {
			display: none;
		}
		.line,
		.dot {
			print-color-adjust: exact;
		}
	}
</style>
