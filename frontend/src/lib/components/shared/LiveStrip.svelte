<script lang="ts">
	import { areaPaths } from '$lib/utils/sparkline';
	// One dashboard strip: a label, a big number with its unit, a one-line
	// note and a sparkline of the last N samples drawn against a fixed max.
	let {
		label,
		value,
		unit = '',
		sub = '',
		values = [],
		max = 0,
		color = 'var(--ctp-primary)'
	}: {
		label: string;
		value: string;
		unit?: string;
		sub?: string;
		values?: number[];
		max?: number;
		color?: string;
	} = $props();
	const W = 220;
	const H = 44;
	let paths = $derived(areaPaths(values, max, W, H));
</script>

<div class="flex flex-col gap-2.5 min-w-0">
	<div class="text-xs uppercase tracking-wide text-[var(--ctp-overlay1)] truncate">{label}</div>
	<div class="flex items-baseline gap-2 min-w-0">
		<span class="text-[28px] leading-none font-semibold text-[var(--ctp-text)] tabular-nums">{value}</span>
		<span class="text-xs text-[var(--ctp-overlay1)] truncate">{unit}{unit && sub ? ' · ' : ''}{sub}</span>
	</div>
	<svg viewBox="0 0 {W} {H}" preserveAspectRatio="none" class="block w-full h-11" aria-hidden="true">
		{#if paths.line}
			<path d={paths.area} fill={color} opacity="0.14" />
			<path d={paths.line} fill="none" stroke={color} stroke-width="1.5" stroke-linejoin="round" vector-effect="non-scaling-stroke" />
		{:else}
			<line x1="0" y1={H - 0.5} x2={W} y2={H - 0.5} stroke="var(--ctp-surface2)" stroke-width="1" vector-effect="non-scaling-stroke" />
		{/if}
	</svg>
</div>
