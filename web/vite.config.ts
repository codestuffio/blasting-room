import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			// Built as a single-page app; the Go server serves build/ and falls back to 200.html.
			adapter: adapter({ fallback: '200.html' })
		})
	],
	// MapLibre's worker is an ES module that imports a shared chunk.
	worker: { format: 'es' },
	server: {
		// The Go API runs on :8080 during development.
		proxy: { '/api': 'http://localhost:8080' }
	}
});
