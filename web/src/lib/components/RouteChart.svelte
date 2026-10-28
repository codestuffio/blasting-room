<script lang="ts">
	import type { Route, RouteStop } from '#lib/types.ts';

	// The headliner's tour against distance from the room: dates run left to
	// right, miles from the venue bottom to top, so a dip toward the baseline is
	// the tour passing by. One series, so slot 1 and no legend box.
	let { route, date = '' }: { route: Route; date?: string } = $props();

	const DAY = 86_400_000;
	const RADIUS = 100; // radius-clause zone, miles; matches the server's marketMiles
	const t = (d: string) => Date.parse(d);

	// Stops within six weeks of the booking date, or the first two months without one.
	let view = $derived.by(() => {
		const stops = route.stops;
		if (!stops.length) return { stops, hidden: 0 };
		const center = date ? t(date) : t(stops[0].date) + 30 * DAY;
		const near = stops.filter((s) => Math.abs(t(s.date) - center) <= 42 * DAY);
		const shown = near.length ? near : stops;
		return { stops: shown, hidden: stops.length - shown.length };
	});

	let width = $state(0);
	const height = 230;
	const m = { top: 22, right: 16, bottom: 26, left: 44 };
	let plotW = $derived(Math.max(0, width - m.left - m.right));
	const plotH = height - m.top - m.bottom;

	let span = $derived.by(() => {
		const times = view.stops.map((s) => t(s.date));
		if (date) times.push(t(date));
		return [Math.min(...times) - 2 * DAY, Math.max(...times) + 2 * DAY] as const;
	});
	// sqrt keeps nearby stops readable next to cross-country legs.
	let top = $derived.by(() => {
		const far = Math.max(RADIUS * 2, ...view.stops.map((s) => s.miles));
		return [250, 500, 1000, 1500, 2500, 4000, 8000].find((n) => n >= far) ?? far;
	});
	let ticks = $derived([0, RADIUS, 500, 1000, 2500, 5000].filter((v) => v <= top && (v === 0 || v === RADIUS || v * 4 >= top)));

	const x = (d: string) => m.left + ((t(d) - span[0]) / (span[1] - span[0])) * plotW;
	const y = (miles: number) => m.top + plotH - Math.sqrt(Math.min(miles, top) / top) * plotH;
	let path = $derived(view.stops.map((s, i) => `${i ? 'L' : 'M'}${x(s.date).toFixed(1)},${y(s.miles).toFixed(1)}`).join(''));

	// When the date fits the tour, a dashed leg runs from the stops either side down to the room.
	let proposed = $derived.by(() => {
		if (!date || (route.fit !== 'on_the_way' && route.fit !== 'detour')) return '';
		const prev = view.stops.findLast((s) => t(s.date) < t(date));
		const next = view.stops.find((s) => t(s.date) > t(date));
		const pts = [prev, { date, miles: 0 }, next].filter((p) => p !== undefined);
		return pts.length > 1 ? pts.map((p, i) => `${i ? 'L' : 'M'}${x(p.date).toFixed(1)},${y(p.miles).toFixed(1)}`).join('') : '';
	});

	// Open windows in view, as bands a day wide at each end.
	let windows = $derived(
		route.windows
			.filter((w) => t(w.to) >= span[0] && t(w.from) <= span[1])
			.map((w) => ({ ...w, x0: x(w.from) - (plotW * DAY) / 2 / (span[1] - span[0]), x1: x(w.to) + (plotW * DAY) / 2 / (span[1] - span[0]) }))
	);

	// City labels only where they won't collide: nearby stops first, then the ends.
	let labeled = $derived.by(() => {
		const ranked = view.stops
			.map((s, i) => ({ s, i, score: s.miles + (i === 0 || i === view.stops.length - 1 ? -200 : 0) }))
			.sort((a, b) => a.score - b.score);
		const kept: { i: number; px: number; py: number }[] = [];
		for (const r of ranked) {
			const px = x(r.s.date);
			const py = y(r.s.miles);
			if (kept.every((k) => Math.abs(k.px - px) > 78 || Math.abs(k.py - py) > 16)) kept.push({ i: r.i, px, py });
		}
		return new Set(kept.map((k) => k.i));
	});

	const fmt = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric', timeZone: 'UTC' });
	const day = (d: string) => fmt.format(new Date(t(d)));
	const mi = (n: number) => `${Math.round(n).toLocaleString('en-US')} mi`;
	const where = (s: RouteStop) => (s.region ? `${s.city}, ${s.region}` : s.city);

	let hover = $state<number | null>(null);
	let tip = $derived(hover === null ? null : view.stops[hover]);
</script>

{#if route.stops.length}
	<div class="route">
		<div class="plot" bind:clientWidth={width}>
			{#if width}
				<svg
					{width}
					{height}
					role="img"
					aria-label={`${route.artist}'s listed tour dates by distance from the venue. ${view.stops.map((s) => `${day(s.date)} ${where(s)}, ${mi(s.miles)}`).join('; ')}.`}
				>
					<!-- Radius-clause zone: the band nearest the room. -->
					<rect class="zone" x={m.left} y={y(RADIUS)} width={plotW} height={y(0) - y(RADIUS)} />
					{#each windows as w, i (i)}
						<rect class="window" x={w.x0} y={m.top} width={Math.max(4, w.x1 - w.x0)} height={plotH} />
						<text class="window-label" x={(w.x0 + w.x1) / 2} y={m.top - 6} text-anchor="middle">open</text>
					{/each}
					{#each ticks as v (v)}
						<line class="grid" class:base={v === 0} x1={m.left} x2={m.left + plotW} y1={y(v)} y2={y(v)} />
						<text class="tick" x={m.left - 6} y={y(v)} text-anchor="end" dominant-baseline="middle">{v === 0 ? 'Room' : v.toLocaleString('en-US')}</text>
					{/each}
					<text class="tick" x={m.left} y={height - 6}>{day(new Date(span[0] + 2 * DAY).toISOString())}</text>
					<text class="tick" x={m.left + plotW} y={height - 6} text-anchor="end">{day(new Date(span[1] - 2 * DAY).toISOString())}</text>

					{#if date}
						<line class="booking" x1={x(date)} x2={x(date)} y1={m.top} y2={m.top + plotH} />
						<text class="booking-label" x={x(date)} y={m.top + plotH - 6} text-anchor={x(date) > m.left + plotW - 60 ? 'end' : 'start'} dx={x(date) > m.left + plotW - 60 ? -5 : 5}>Your date</text>
					{/if}

					<path class="line" d={path} />
					{#if proposed}
						<path class="proposed" d={proposed} />
						<circle class="room" cx={x(date)} cy={y(0)} r="5" />
					{/if}
					{#each view.stops as s, i (s.date + s.city)}
						<circle class="dot" cx={x(s.date)} cy={y(s.miles)} r={hover === i ? 6 : 4.5} />
						{#if labeled.has(i)}
							<text class="city" x={x(s.date)} y={y(s.miles) - 10} text-anchor="middle">{s.city}</text>
						{/if}
					{/each}
					<!-- Hit targets bigger than the dots. -->
					{#each view.stops as s, i (s.date + s.city)}
						<circle
							class="hit"
							cx={x(s.date)}
							cy={y(s.miles)}
							r="14"
							role="presentation"
							onpointerenter={() => (hover = i)}
							onpointerleave={() => (hover = null)}
						/>
					{/each}
				</svg>
				{#if tip && hover !== null}
					{@const left = x(tip.date)}
					<div class="tip" class:flip={left > width / 2} style:left={`${left}px`} style:top={`${y(tip.miles)}px`} role="status">
						<p class="date">{day(tip.date)} · {mi(tip.miles)} away</p>
						<p><strong>{where(tip)}</strong></p>
						<p class="muted">{tip.venue}</p>
					</div>
				{/if}
			{/if}
		</div>
		<p class="caption muted">
			{route.artist}'s listed dates by miles from the venue (square-root scale). The shaded band is within {RADIUS} miles, where a radius clause can bite;
			"open" marks free nights between nearby stops{proposed ? ', and the dashed leg is your date added to the run' : ''}.
			{#if view.hidden}{view.hidden} more {view.hidden === 1 ? 'date is' : 'dates are'} outside this window.{/if}
		</p>
		<details class="table">
			<summary>Tour dates</summary>
			<table>
				<thead><tr><th>Date</th><th>City</th><th>Venue</th><th>Miles away</th></tr></thead>
				<tbody>
					{#each route.stops as s (s.date + s.city)}
						<tr class:in-zone={s.miles <= RADIUS}>
							<td>{day(s.date)}</td>
							<td>{where(s)}</td>
							<td>{s.venue}</td>
							<td class="num">{Math.round(s.miles).toLocaleString('en-US')}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</details>
	</div>
{/if}

<style>
	.route {
		margin: 0;
		display: grid;
		gap: 0.5rem;
		min-width: 0;
	}
	.plot {
		position: relative;
		height: 230px;
		min-width: 0;
	}
	svg {
		position: absolute;
		inset: 0 auto auto 0;
		display: block;
		overflow: visible;
	}
	.zone {
		fill: var(--line);
		opacity: 0.45;
	}
	.window {
		fill: var(--accent);
		opacity: 0.14;
	}
	.window-label,
	.booking-label {
		font-family: var(--font-mono);
		font-size: 10px;
		fill: var(--accent-text);
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}
	.grid {
		stroke: var(--line);
		stroke-width: 1;
	}
	.grid.base {
		stroke: var(--muted);
	}
	.tick {
		font-family: var(--font-mono);
		font-size: 10px;
		fill: var(--muted);
	}
	.booking {
		stroke: var(--accent);
		stroke-width: 2;
	}
	.line {
		fill: none;
		stroke: var(--series-1);
		stroke-width: 2;
		stroke-linejoin: round;
	}
	.proposed {
		fill: none;
		stroke: var(--series-1);
		stroke-width: 2;
		stroke-dasharray: 5 4;
		opacity: 0.85;
	}
	.room {
		fill: var(--panel);
		stroke: var(--series-1);
		stroke-width: 2.5;
	}
	.dot {
		fill: var(--series-1);
		stroke: var(--panel);
		stroke-width: 2;
	}
	.city {
		font-size: 11px;
		fill: var(--text);
		paint-order: stroke;
		stroke: var(--panel);
		stroke-width: 3px;
	}
	.hit {
		fill: transparent;
		cursor: pointer;
	}
	.tip {
		position: absolute;
		transform: translate(10px, -50%);
		padding: 0.4rem 0.6rem;
		border: 1px solid var(--line);
		border-radius: 8px;
		background: var(--panel);
		font-size: 0.8rem;
		pointer-events: none;
		white-space: nowrap;
		box-shadow: 0 4px 14px rgb(0 0 0 / 0.25);
	}
	.tip.flip {
		transform: translate(calc(-100% - 10px), -50%);
	}
	.tip p {
		margin: 0;
	}
	.tip .date {
		font-family: var(--font-mono);
		font-size: 0.72rem;
		color: var(--muted);
	}
	.muted {
		color: var(--muted);
	}
	.caption {
		margin: 0;
		font-size: 0.8rem;
	}
	details summary {
		cursor: pointer;
		color: var(--muted);
		font-family: var(--font-mono);
		font-size: 0.8rem;
	}
	table {
		width: 100%;
		margin-top: 0.4rem;
		border-collapse: collapse;
		font-size: 0.85rem;
	}
	th,
	td {
		text-align: left;
		padding: 0.3rem 0.4rem;
		border-bottom: 1px solid var(--line);
	}
	.num {
		font-variant-numeric: tabular-nums;
	}
	tr.in-zone td:first-child::after {
		content: ' · within 100 mi';
		color: var(--accent-text);
		font-size: 0.75rem;
	}
	@media print {
		.zone,
		.window,
		.line,
		.proposed,
		.room,
		.dot {
			print-color-adjust: exact;
		}
		.tip {
			display: none;
		}
	}
</style>
