import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { resolve } from 'path';

export default defineConfig({
	plugins: [svelte({ hot: !process.env.VITEST })],
	test: {
		environment: 'jsdom',
		include: ['src/**/*.{test,spec}.{js,ts}'],
		globals: true,
		alias: {
			$lib: resolve(__dirname, './src/lib')
		}
	},
	resolve: {
		alias: {
			$lib: resolve(__dirname, './src/lib')
		}
	}
});
