<script lang="ts">
	// The "pinned" mark: a VU needle in the red. The tile stays dark in both themes, like an app icon.
	let { size = 44 }: { size?: number } = $props();
</script>

<span class="logo">
	<svg viewBox="0 0 100 100" width={size} height={size} aria-hidden="true">
		<rect width="100" height="100" rx="22" fill="#1e1c19" />
		<path d="M16 70A34 34 0 0 1 74 46" fill="none" stroke="#efe6d2" stroke-width="6" stroke-linecap="round" />
		<path d="M74 46A34 34 0 0 1 84 70" fill="none" stroke="#ff5a47" stroke-width="6" stroke-linecap="round" />
		<path d="M50 70 82.8 47.1" stroke="#ffb224" stroke-width="5" stroke-linecap="round" />
		<circle cx="50" cy="70" r="7" fill="#ffb224" />
	</svg>
	<span class="word">Blasting Room</span>
</span>

<style>
	.logo {
		display: inline-flex;
		align-items: center;
		gap: 0.85rem;
	}
	.word {
		font-family: var(--font-display);
		font-size: clamp(1.25rem, 3.2vw, 1.75rem);
		letter-spacing: 0.08em;
		text-transform: uppercase;
		line-height: 1.1;
	}
</style>
