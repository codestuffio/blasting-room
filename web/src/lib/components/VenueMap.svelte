<script lang="ts">
	import { flushSync, onMount } from 'svelte';
	import type { GeoJSONSource, Map as MapLibreMap } from 'maplibre-gl';
	import type { Geo, GeoPlace, HeatCell, Venue } from '#lib/types.ts';

	let {
		venue,
		geo,
		names = {},
		focus = '',
		highlight = []
	}: {
		venue: Venue;
		geo: Geo[];
		/** entity_id → display name for the heatmap picker */
		names?: Record<string, string>;
		/** entity_id whose heatmap to show first (the headliner) */
		focus?: string;
		/** partner names the memo recommends; their labels win collisions */
		highlight?: string[];
	} = $props();

	// Free OpenStreetMap vector tiles from OpenFreeMap; no key needed.
	const styles = {
		light: 'https://tiles.openfreemap.org/styles/positron',
		dark: 'https://tiles.openfreemap.org/styles/dark'
	};
	// One-hue sequential ramps (low → high affinity). Each theme has its own steps:
	// light goes pale → deep, dark goes dim → bright.
	const ramps = {
		light: ['#fbe9c4', '#f5c46a', '#e09a1a', '#a86a00', '#6a4200'],
		dark: ['#3a2a10', '#7a5212', '#c98a12', '#ffb224', '#ffe2a0']
	};

	// Last heatmap per artist: the focus artist first, then in the order they were fetched.
	let heatmaps = $derived.by(() => {
		const byId = new Map<string, HeatCell[]>();
		for (const g of geo) if (g.kind === 'heatmap' && g.cells?.length) byId.set(g.entity_ids[0], g.cells);
		return [...byId]
			.map(([id, cells], i) => ({ id, cells, label: names[id] ?? `Artist ${i + 1}` }))
			.sort((a, b) => Number(b.id === focus) - Number(a.id === focus));
	});
	let places = $derived.by(() => {
		const byId = new Map<string, GeoPlace>();
		for (const g of geo) for (const p of g.places ?? []) byId.set(p.entity_id, p);
		return [...byId.values()];
	});

	let picked = $state('');
	let active = $derived(
		heatmaps.find((h) => h.id === picked) ??
			heatmaps.find((h) => h.id === focus) ??
			heatmaps[heatmaps.length - 1]
	);

	// Where the venue's own cell ranks for the active artist.
	let venueRank = $derived.by(() => {
		if (!active) return null;
		const d = (c: HeatCell) => (c.lat - venue.lat) ** 2 + (c.lon - venue.lon) ** 2;
		const near = active.cells.reduce((a, b) => (d(b) < d(a) ? b : a));
		const rank = 1 + active.cells.filter((c) => c.affinity > near.affinity).length;
		return { rank, of: active.cells.length, affinity: near.affinity };
	});

	// Per-theme colors for things drawn inside the WebGL canvas, where CSS variables don't reach.
	const ink = {
		light: { text: '#1a1206', halo: '#f8f4ea', partner: '#1f7a4a' },
		dark: { text: '#efe6d2', halo: '#141311', partner: '#66e08f' }
	};

	let container: HTMLDivElement;
	let map: MapLibreMap | undefined;
	let ml: typeof import('maplibre-gl') | undefined;
	let theme = $state<'light' | 'dark'>('dark');
	let styleReady = $state(0); // bumps on every style load so effects re-apply layers
	let failed = $state('');

	function setTheme(t: 'light' | 'dark') {
		if (t === theme) return;
		theme = t;
		if (!map) return;
		styleReady = 0; // pause the layer effects until the new style has loaded
		map.setStyle(styles[t], { diff: false });
	}

	onMount(() => {
		const mq = window.matchMedia('(prefers-color-scheme: dark)');
		theme = mq.matches ? 'dark' : 'light';
		const onTheme = () => setTheme(mq.matches ? 'dark' : 'light');
		mq.addEventListener('change', onTheme);

		let cancelled = false;
		(async () => {
			try {
				// MapLibre 6 runs tile parsing in a separate module worker; let Vite bundle and emit it.
				const [lib, worker] = await Promise.all([
					import('maplibre-gl'),
					import('maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url'),
					import('maplibre-gl/dist/maplibre-gl.css')
				]);
				if (cancelled) return;
				ml = lib;
				ml.setWorkerUrl(worker.default);
				map = new ml.Map({
					container,
					style: styles[theme],
					center: [venue.lon, venue.lat],
					zoom: 11.5,
					cooperativeGestures: true,
					// Keeps the last frame readable so a print can snapshot it synchronously.
					canvasContextAttributes: { preserveDrawingBuffer: true },
					attributionControl: { compact: true }
				});
				map.addControl(new ml.NavigationControl({ showCompass: false }), 'top-right');
				map.on('style.load', () => styleReady++);
				// Compact attribution starts expanded; on a phone that covers the map. Keep it one tap away.
				map.once('load', () => {
					if (container.clientWidth < 520) {
						container.querySelector('.maplibregl-ctrl-attrib')?.classList.remove('maplibregl-compact-show');
					}
				});
				addTooltips(map, ml);
			} catch (e) {
				failed = `Map unavailable: ${(e as Error).message}`;
			}
		})();

		return () => {
			cancelled = true;
			mq.removeEventListener('change', onTheme);
			map?.remove();
		};
	});

	const points = <P,>(items: { lat: number; lon: number }[], props: (i: number) => P) => ({
		type: 'FeatureCollection' as const,
		features: items.map((it, i) => ({
			type: 'Feature' as const,
			geometry: { type: 'Point' as const, coordinates: [it.lon, it.lat] },
			properties: props(i)
		}))
	});

	// Sources and layers are re-added after each style load, since setStyle drops them.
	// Pins are layers rather than DOM markers so MapLibre resolves label collisions.
	$effect(() => {
		if (!styleReady || !map) return;
		try {
			drawLayers(map);
		} catch (e) {
			console.warn('map layers', e); // a map glitch must never take the memo down
		}
	});

	function drawLayers(map: MapLibreMap) {
		const cells = active?.cells ?? [];
		const heat = points(cells, (i) => ({ affinity: cells[i].affinity }));
		const pins = points([venue, ...places], (i) => {
			const p = i === 0 ? null : places[i - 1];
			return {
				kind: p ? 'partner' : 'venue',
				name: p ? p.name : venue.name,
				detail: p?.address ?? '',
				// Lower sorts first: the venue, then memo picks, then other partners.
				rank: p ? (highlight.includes(p.name) ? 1 : 2) : 0
			};
		});
		const heatSrc = map.getSource('heat') as GeoJSONSource | undefined;
		const pinSrc = map.getSource('pins') as GeoJSONSource | undefined;
		if (heatSrc && pinSrc) {
			heatSrc.setData(heat);
			pinSrc.setData(pins);
			return;
		}
		const ramp = ramps[theme];
		const c = ink[theme];
		map.addSource('heat', { type: 'geojson', data: heat });
		map.addSource('pins', { type: 'geojson', data: pins });
		map.addLayer({
			id: 'heat-glow',
			type: 'heatmap',
			source: 'heat',
			paint: {
				'heatmap-weight': ['get', 'affinity'],
				'heatmap-radius': ['interpolate', ['linear'], ['zoom'], 10, 25, 14, 90],
				'heatmap-opacity': 0.45,
				'heatmap-color': ['interpolate', ['linear'], ['heatmap-density'], 0, 'rgba(0,0,0,0)', 0.2, ramp[1], 0.6, ramp[2], 1, ramp[4]]
			}
		});
		map.addLayer({
			id: 'heat-cells',
			type: 'circle',
			source: 'heat',
			paint: {
				'circle-radius': ['interpolate', ['linear'], ['get', 'affinity'], 0, 3, 1, 12],
				'circle-color': ['interpolate', ['linear'], ['get', 'affinity'], ...ramp.flatMap((hex, i) => [i / (ramp.length - 1), hex])] as never,
				'circle-opacity': 0.85,
				'circle-stroke-width': 1,
				'circle-stroke-color': c.halo
			}
		});
		map.addLayer({
			id: 'pin-dots',
			type: 'circle',
			source: 'pins',
			paint: {
				'circle-radius': ['match', ['get', 'kind'], 'venue', 7, 5],
				'circle-color': ['match', ['get', 'kind'], 'venue', c.text, c.partner],
				'circle-stroke-width': ['match', ['get', 'kind'], 'venue', 3, 2],
				'circle-stroke-color': c.halo
			}
		});
		map.addLayer({
			id: 'pin-labels',
			type: 'symbol',
			source: 'pins',
			layout: {
				'text-field': ['get', 'name'],
				'text-font': ['Noto Sans Regular'],
				'text-size': ['match', ['get', 'kind'], 'venue', 14, 12],
				'text-variable-anchor': ['left', 'right', 'top', 'bottom'],
				'text-radial-offset': 0.9,
				'text-justify': 'auto',
				'symbol-sort-key': ['get', 'rank']
			},
			paint: { 'text-color': c.text, 'text-halo-color': c.halo, 'text-halo-width': 1.5 }
		});
	}

	// Frame the venue, the active heatmap and the partners whenever they change.
	$effect(() => {
		if (!styleReady || !map || !ml) return;
		const b = new ml.LngLatBounds([venue.lon, venue.lat], [venue.lon, venue.lat]);
		for (const c of active?.cells ?? []) b.extend([c.lon, c.lat]);
		for (const p of places) b.extend([p.lon, p.lat]);
		map.fitBounds(b, { padding: 36, maxZoom: 13.5, duration: 600 });
	});

	// WebGL canvases often print blank, so print shows a PNG snapshot of the map instead.
	let snapshot = $state('');
	let attribution = $state('');
	let snapshotImg = $state<HTMLImageElement>();
	let restore: (() => void) | null = null;

	function capture() {
		if (!map) return;
		try {
			snapshot = map.getCanvas().toDataURL('image/png');
			attribution = container.querySelector('.maplibregl-ctrl-attrib-inner')?.textContent ?? '';
		} catch (e) {
			console.warn('map snapshot', e);
		}
		flushSync(); // the print layout follows right after, so the <img> must be in the DOM now
	}

	const rendered = (m: MapLibreMap) =>
		new Promise<void>((resolve) => {
			const timer = setTimeout(resolve, 8000); // print what we have if tiles stall
			m.once('idle', () => {
				clearTimeout(timer);
				resolve();
			});
			m.triggerRepaint();
		});

	/** Re-renders the map light and at print resolution, then snapshots it. It reverts after printing. */
	export async function prepareForPrint() {
		if (!map || failed) return;
		const was = { theme, ratio: map.getPixelRatio() };
		restore = () => {
			restore = null;
			map?.setPixelRatio(was.ratio);
			setTheme(was.theme);
		};
		map.setPixelRatio(Math.max(2, was.ratio));
		setTheme('light');
		await rendered(map);
		capture();
		await snapshotImg?.decode().catch(() => {});
	}

	// Hover a heat cell for its affinity; hover or tap a pin for its name and address.
	function addTooltips(m: MapLibreMap, lib: typeof import('maplibre-gl')) {
		const tip = new lib.Popup({ closeButton: false, closeOnClick: false, offset: 10, className: 'map-tip' });
		const show = (layer: string, text: (p: Record<string, unknown>) => string) => {
			m.on('mousemove', layer, (e) => {
				const f = e.features?.[0];
				if (!f) return;
				m.getCanvas().style.cursor = 'pointer';
				tip.setLngLat(e.lngLat).setText(text(f.properties)).addTo(m);
			});
			m.on('mouseleave', layer, () => {
				m.getCanvas().style.cursor = '';
				tip.remove();
			});
		};
		show('heat-cells', (p) => `Fan affinity ${Number(p.affinity).toFixed(2)}`);
		show('pin-dots', (p) => [p.name, p.detail].filter(Boolean).join(' · '));
	}
</script>

<!-- A print from prepareForPrint already has its snapshot; a plain Cmd+P takes one of the map as it is. -->
<svelte:window
	onbeforeprint={() => {
		if (!restore) capture();
	}}
	onafterprint={() => restore?.()}
/>

<figure class="venue-map" class:has-snapshot={!!snapshot} style:--partner={ink[theme].partner}>
	{#if heatmaps.length > 1}
		<div class="picker" role="radiogroup" aria-label="Show heatmap for">
			{#each heatmaps as h (h.id)}
				<button type="button" role="radio" aria-checked={active?.id === h.id} onclick={() => (picked = h.id)}>
					{h.label}
				</button>
			{/each}
		</div>
	{/if}
	<div
		class="canvas"
		bind:this={container}
		role="img"
		aria-label={`Map around ${venue.name}${active ? ` showing where ${active.label}'s fans over-index` : ''}${places.length ? ` and ${places.length} nearby co-promo places` : ''}.`}
	>
		{#if failed}<p class="failed">{failed}</p>{/if}
	</div>
	{#if snapshot}
		<img class="snapshot" src={snapshot} alt={`Map around ${venue.name}`} bind:this={snapshotImg} />
	{/if}
	<figcaption>
		{#if active}
			<span class="ramp" style:--ramp={`linear-gradient(90deg, ${ramps[theme].join(', ')})`}>
				<span>Fan affinity</span><i aria-hidden="true"></i><span>low → high</span>
			</span>
		{/if}
		<span class="key"><i class="dot venue" aria-hidden="true"></i>{venue.name}</span>
		{#if places.length}<span class="key"><i class="dot partner" aria-hidden="true"></i>Co-promo place</span>{/if}
		{#if venueRank}
			<span class="rank">
				{active?.label}: the venue's cell ranks <strong>#{venueRank.rank}</strong> of {venueRank.of}
				(affinity {venueRank.affinity.toFixed(2)})
			</span>
		{/if}
		{#if snapshot && attribution}<span class="attribution">Map: {attribution}</span>{/if}
	</figcaption>
</figure>

<style>
	.venue-map {
		margin: 0;
		display: grid;
		gap: 0.5rem;
	}
	.canvas {
		position: relative;
		height: clamp(260px, 45vw, 380px);
		border: 1px solid var(--line);
		border-radius: 12px;
		overflow: hidden;
		background: var(--panel);
	}
	.failed {
		margin: 0;
		padding: 1rem;
		color: var(--muted);
	}
	.picker {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
	}
	.picker button {
		font-family: var(--font-mono);
		font-size: 0.8rem;
		padding: 0.25rem 0.7rem;
		border-radius: 6px;
		border: 1px solid var(--line);
		background: transparent;
		color: var(--text);
		cursor: pointer;
	}
	.picker button[aria-checked='true'] {
		border-color: var(--accent);
		box-shadow: 0 0 0 1px var(--accent);
	}
	figcaption {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.4rem 1rem;
		font-size: 0.8rem;
		color: var(--muted);
	}
	.ramp,
	.key {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
	}
	.ramp i {
		width: 5rem;
		height: 0.5rem;
		border-radius: 4px;
		background: var(--ramp);
	}
	.rank {
		flex-basis: 100%;
		color: var(--text);
	}
	.dot {
		display: inline-block;
		border-radius: 50%;
	}
	.dot.venue {
		width: 12px;
		height: 12px;
		background: var(--text);
		box-shadow: 0 0 0 2px var(--panel), 0 0 0 3px var(--line);
	}
	.dot.partner {
		width: 9px;
		height: 9px;
		background: var(--partner);
	}
	.snapshot,
	.attribution {
		display: none;
	}
	@media print {
		.venue-map {
			break-inside: avoid;
		}
		.picker,
		.has-snapshot .canvas {
			display: none;
		}
		.snapshot {
			display: block;
			width: 100%;
			height: auto;
			border: 1px solid var(--line);
			border-radius: 8px;
		}
		.attribution {
			display: inline;
			flex-basis: 100%;
			font-size: 0.7rem;
		}
		figcaption {
			print-color-adjust: exact; /* keep the legend ramp and dots, which are backgrounds */
		}
	}
	.venue-map :global(.map-tip .maplibregl-popup-content) {
		background: var(--panel);
		color: var(--text);
		font-size: 0.8rem;
		padding: 0.35rem 0.6rem;
		border: 1px solid var(--line);
	}
	.venue-map :global(.map-tip .maplibregl-popup-tip) {
		display: none;
	}
</style>
