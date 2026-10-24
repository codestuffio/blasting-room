<script lang="ts">
	// An analog VU meter. value runs 0 (needle far left) to 1 (pinned right); the face is cream in both themes.
	let { value, label, width = 180 }: { value: number; label: string; width?: number } = $props();

	let angle = $derived(-55 + Math.min(1, Math.max(0, value)) * 105);

	// Tick marks every 10° from -60° to +60°, pivoting on (140, 150).
	const ticks = Array.from({ length: 13 }, (_, i) => {
		const a = ((-60 + i * 10 - 90) * Math.PI) / 180;
		const outer = i % 2 === 0 ? 122 : 114;
		return {
			x1: 140 + 104 * Math.cos(a),
			y1: 150 + 104 * Math.sin(a),
			x2: 140 + outer * Math.cos(a),
			y2: 150 + outer * Math.sin(a),
			hot: i >= 9
		};
	});
</script>

<svg class="vu" viewBox="0 0 280 170" {width} role="img" aria-label={label}>
	<rect width="280" height="170" rx="12" fill="var(--face)" />
	<path d="M55.1 101A98 98 0 0 1 189 65.1" fill="none" stroke="var(--face-ink)" stroke-width="2" />
	<path d="M189 65.1A98 98 0 0 1 224.9 101" fill="none" stroke="#1f8a57" stroke-width="9" />
	{#each ticks as t, i (i)}
		<line x1={t.x1} y1={t.y1} x2={t.x2} y2={t.y2} stroke={t.hot ? '#1f8a57' : 'var(--face-ink)'} stroke-width="2.5" stroke-linecap="round" />
	{/each}
	<text x="62" y="134" class="scale">LOW</text>
	<text x="218" y="134" class="scale" text-anchor="end">HIGH</text>
	<g class="needle" style:transform={`rotate(${angle}deg)`}>
		<line x1="140" y1="150" x2="140" y2="42" stroke="var(--face-ink)" stroke-width="3" stroke-linecap="round" />
	</g>
	<circle cx="140" cy="150" r="10" fill="var(--face-ink)" />
</svg>

<style>
	.vu {
		display: block;
		height: auto;
		flex: none;
	}
	.scale {
		font-family: var(--font-mono);
		font-size: 13px;
		fill: var(--face-ink);
	}
	.needle {
		transform-origin: 140px 150px;
		transition: transform 0.6s cubic-bezier(0.3, 1.6, 0.5, 1);
	}
</style>
