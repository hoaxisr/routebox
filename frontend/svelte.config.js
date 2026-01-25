import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),

	compilerOptions: {
		// Suppress warnings that are intentional or less critical for this admin tool
		warningFilter: (warning) => {
			// Form components intentionally capture initial prop values for editing
			if (warning.code === 'state_referenced_locally') return false;
			// Element bindings don't need $state
			if (warning.code === 'non_reactive_update' && warning.message?.includes('fileInput')) return false;
			// A11y warnings for internal admin tool - we prioritize functionality
			if (warning.code?.startsWith('a11y_')) return false;
			return true;
		}
	},

	kit: {
		adapter: adapter({
			pages: 'build',
			assets: 'build',
			fallback: 'index.html',
			precompress: false,
			strict: true
		}),
		alias: {
			$lib: './src/lib'
		}
	}
};

export default config;
