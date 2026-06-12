<script lang="ts">
	interface Slice {
		key: string;
		label: string;
		value: number;
		color: string;
	}

	interface Props {
		items: { key: string; label: string; value: number }[];
		centerNumber?: string | number;
		centerLabel?: string;
		topN?: number;
		size?: number;
		onSelect?: (key: string | null) => void;
		activeKey?: string | null;
	}

	let { items, centerNumber, centerLabel, topN = 3, size = 96, onSelect, activeKey = null }: Props = $props();

	const PALETTE = [
		'var(--ctp-primary)',
		'color-mix(in srgb, var(--ctp-primary) 55%, white)',
		'var(--ctp-subtext1)',
		'var(--ctp-overlay0)'
	];

	const slices = $derived.by((): Slice[] => {
		const sorted = [...items].sort((a, b) => b.value - a.value);
		const top = sorted.slice(0, topN);
		const rest = sorted.slice(topN);
		const out: Slice[] = top.map((it, i) => ({
			key: it.key,
			label: it.label,
			value: it.value,
			color: PALETTE[i] ?? PALETTE[PALETTE.length - 1]
		}));
		if (rest.length > 0) {
			out.push({
				key: '__other__',
				label: `other (${rest.length})`,
				value: rest.reduce((s, x) => s + x.value, 0),
				color: PALETTE[PALETTE.length - 1]
			});
		}
		return out;
	});

	const total = $derived(slices.reduce((s, x) => s + x.value, 0));
	const r = $derived((size - 14) / 2);
	const cx = $derived(size / 2);
	const cy = $derived(size / 2);
	const circumference = $derived(2 * Math.PI * r);

	function pct(v: number): string {
		if (total === 0) return '0%';
		return `${Math.round((v / total) * 100)}%`;
	}

	function handleSliceClick(slice: Slice) {
		if (slice.key === '__other__' || !onSelect) return;
		onSelect(activeKey === slice.key ? null : slice.key);
	}
</script>

<div class="flex flex-col sm:flex-row sm:items-center gap-3">
	<div class="relative flex-shrink-0" style="width:{size}px;height:{size}px">
		<svg width={size} height={size} viewBox="0 0 {size} {size}" style="transform: rotate(-90deg)">
			<circle cx={cx} cy={cy} r={r} fill="none" stroke="var(--ctp-surface2)" stroke-width="14"/>
			{#each slices as slice, i}
				{@const offset = -slices.slice(0, i).reduce((s, x) => s + (x.value / Math.max(1, total)) * circumference, 0)}
				{@const len = (slice.value / Math.max(1, total)) * circumference}
				<circle
					cx={cx} cy={cy} r={r}
					fill="none"
					stroke={slice.color}
					stroke-width="14"
					stroke-dasharray="{len} {circumference}"
					stroke-dashoffset={offset}
					style="cursor: {slice.key === '__other__' || !onSelect ? 'default' : 'pointer'}; opacity: {activeKey && activeKey !== slice.key && slice.key !== '__other__' ? 0.45 : 1}"
					onclick={() => handleSliceClick(slice)}
				/>
			{/each}
		</svg>
		{#if centerNumber !== undefined}
			<div class="absolute inset-0 flex flex-col items-center justify-center pointer-events-none">
				<div class="font-mono font-semibold text-sm text-[var(--ctp-text)]">{centerNumber}</div>
				{#if centerLabel}
					<div class="text-[9px] uppercase tracking-wide text-[var(--ctp-overlay0)]">{centerLabel}</div>
				{/if}
			</div>
		{/if}
	</div>
	<div class="w-full sm:w-auto sm:flex-1 min-w-0 flex flex-col gap-1 text-xs">
		{#each slices as slice}
			<button
				onclick={() => handleSliceClick(slice)}
				disabled={slice.key === '__other__' || !onSelect}
				class="flex items-center gap-1.5 text-left disabled:cursor-default"
			>
				<span class="w-2 h-2 rounded-sm flex-shrink-0" style="background:{slice.color}"></span>
				<span class="flex-1 truncate text-[var(--ctp-subtext1)]">{slice.label}</span>
				<span class="font-mono text-[10px] text-[var(--ctp-overlay0)]">{pct(slice.value)}</span>
			</button>
		{/each}
	</div>
</div>
