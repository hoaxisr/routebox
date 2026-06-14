<script lang="ts">
	import { sparklinePath } from '$lib/utils/sparkline';
	let {
		values = [],
		width = 80,
		height = 20,
		color = 'var(--ctp-primary)'
	}: { values?: number[]; width?: number; height?: number; color?: string } = $props();
	let d = $derived(sparklinePath(values, width, height));
</script>

{#if d}
	<svg {width} {height} viewBox={`0 0 ${width} ${height}`} class="sparkline" aria-hidden="true">
		<path {d} fill="none" stroke={color} stroke-width="1.5" stroke-linejoin="round" stroke-linecap="round" />
	</svg>
{:else}
	<span class="sparkline-empty">—</span>
{/if}

<style>
	.sparkline { display: inline-block; vertical-align: middle; }
	.sparkline-empty { color: var(--ctp-overlay0); }
</style>
