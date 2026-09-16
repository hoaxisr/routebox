import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { resolve } from 'path';

export default defineConfig({
	plugins: [svelte({ hot: !process.env.VITEST })],
	test: {
		environment: 'jsdom',
		include: ['src/**/*.{test,spec}.{js,ts}'],
		setupFiles: ['./vitest-setup.ts'],
		globals: true,
		alias: {
			$lib: resolve(__dirname, './src/lib'),
			'$app/environment': resolve(__dirname, './src/test-stubs/app-environment.ts')
		}
	},
	resolve: {
		// Svelte 5 ships separate server and client builds; without the browser
		// condition the component tests get the server one and mount() throws
		// lifecycle_function_unavailable.
		conditions: ['browser'],
		alias: {
			$lib: resolve(__dirname, './src/lib')
		}
	}
});
