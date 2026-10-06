<script lang="ts">
	import { areaPaths } from '$lib/utils/sparkline';

	// Two series on one scale, the dashboard's look: ↓ coral, ↑ blue. Values are
	// already rates (B/s); the caller resamples (seriesRates) or passes a live trend.
	interface Props {
		down: number[];
		up: number[];
		class?: string;
	}
	let { down, up, class: cls = 'h-24' }: Props = $props();
	const W = 600;
	const H = 88;
	let max = $derived(Math.max(1, ...down, ...up) * 1.15);
	let d = $derived(areaPaths(down, max, W, H));
	let u = $derived(areaPaths(up, max, W, H));
</script>

<svg viewBox="0 0 {W} {H}" preserveAspectRatio="none" class="block w-full {cls}" aria-hidden="true">
	{#if d.line || u.line}
		<path d={d.area} fill="var(--ctp-primary)" opacity="0.14" />
		<path d={u.area} fill="var(--ctp-upload)" opacity="0.14" />
		<path d={d.line} fill="none" stroke="var(--ctp-primary)" stroke-width="1.5" stroke-linejoin="round" vector-effect="non-scaling-stroke" />
		<path d={u.line} fill="none" stroke="var(--ctp-upload)" stroke-width="1.5" stroke-linejoin="round" vector-effect="non-scaling-stroke" />
	{:else}
		<line x1="0" y1={H - 0.5} x2={W} y2={H - 0.5} stroke="var(--ctp-surface2)" stroke-width="1" vector-effect="non-scaling-stroke" />
	{/if}
</svg>
