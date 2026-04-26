<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { t } from 'svelte-i18n';
	import { createConnectionsStream, api } from '$lib/api/client';
	import { formatBytes } from '$lib/stores';
	import type { ClashConnection } from '$lib/types';

	type Dimension = 'source' | 'domain' | 'chain';

	let connections = $state<ClashConnection[]>([]);
	let stream: { close: () => void } | null = null;
	let loading = $state(true);

	// Active filters per dimension. Each dimension can hold multiple values (OR within,
	// AND across) — e.g. source=192.168.1.14 AND (domain=a.com OR domain=b.com).
	let filters = $state<Record<Dimension, string | null>>({
		source: null,
		domain: null,
		chain: null
	});

	let expandedPanels = $state<Record<Dimension, boolean>>({
		source: false,
		domain: false,
		chain: false
	});

	function togglePanelExpand(dim: Dimension) {
		expandedPanels = { ...expandedPanels, [dim]: !expandedPanels[dim] };
	}

	function keyOf(conn: ClashConnection, dim: Dimension): string {
		switch (dim) {
			case 'source':
				return conn.metadata.sourceIP || 'unknown';
			case 'domain':
				return conn.metadata.host || conn.metadata.destinationIP || '-';
			case 'chain':
				return conn.chains && conn.chains.length > 0 ? conn.chains.join(' → ') : '-';
		}
	}

	function matchesFilters(conn: ClashConnection, except?: Dimension): boolean {
		for (const dim of ['source', 'domain', 'chain'] as Dimension[]) {
			if (dim === except) continue;
			const v = filters[dim];
			if (v === null) continue;
			if (keyOf(conn, dim) !== v) return false;
		}
		return true;
	}

	function toggleFilter(dim: Dimension, value: string) {
		filters = { ...filters, [dim]: filters[dim] === value ? null : value };
	}

	function clearFilters() {
		filters = { source: null, domain: null, chain: null };
	}

	interface Bucket {
		key: string;
		upload: number;
		download: number;
		total: number;
		connCount: number;
		primaryIp?: string;
		ipCount?: number;
		allIps?: string[];
	}

	// For each dimension we aggregate using the filter-set from the OTHER two dimensions,
	// so clicking on a source narrows the domain/chain panels but leaves the source panel
	// showing all sources (otherwise you'd lose the ability to compare).
	function aggregate(dim: Dimension): Bucket[] {
		const map = new Map<string, Bucket>();
		const ipsByKey = dim === 'domain' ? new Map<string, Set<string>>() : null;
		for (const conn of connections) {
			if (!matchesFilters(conn, dim)) continue;
			const key = keyOf(conn, dim);
			let b = map.get(key);
			if (!b) {
				b = { key, upload: 0, download: 0, total: 0, connCount: 0 };
				map.set(key, b);
			}
			b.upload += conn.upload;
			b.download += conn.download;
			b.total += conn.upload + conn.download;
			b.connCount += 1;
			if (ipsByKey) {
				const ip = conn.metadata.destinationIP;
				if (ip) {
					let set = ipsByKey.get(key);
					if (!set) { set = new Set(); ipsByKey.set(key, set); }
					set.add(ip);
					if (!b.primaryIp) b.primaryIp = ip;
				}
			}
		}
		if (ipsByKey) {
			for (const [key, set] of ipsByKey) {
				const b = map.get(key)!;
				b.ipCount = set.size;
				b.allIps = Array.from(set);
			}
		}
		return Array.from(map.values()).sort((a, b) => b.total - a.total);
	}

	const sourceBuckets = $derived(aggregate('source'));
	const domainBuckets = $derived(aggregate('domain'));
	const chainBuckets = $derived(aggregate('chain'));

	const filteredTotal = $derived.by(() => {
		let up = 0;
		let down = 0;
		let count = 0;
		for (const conn of connections) {
			if (!matchesFilters(conn)) continue;
			up += conn.upload;
			down += conn.download;
			count += 1;
		}
		return { up, down, count };
	});

	const hasAnyFilter = $derived(
		filters.source !== null || filters.domain !== null || filters.chain !== null
	);

	function startStream() {
		stream = createConnectionsStream(
			(data) => {
				connections = data.connections || [];
				loading = false;
			},
			() => {
				// fallback poll if ws fails
				fallbackPoll();
			}
		);
	}

	let poll: ReturnType<typeof setInterval> | null = null;
	async function fallbackPoll() {
		if (poll) return;
		const tick = async () => {
			try {
				const data = await api.getConnections();
				connections = data.connections || [];
				loading = false;
			} catch {
				/* ignore */
			}
		};
		tick();
		poll = setInterval(tick, 2000);
	}

	onMount(() => {
		startStream();
	});

	onDestroy(() => {
		stream?.close();
		if (poll) clearInterval(poll);
	});

	function barWidth(b: Bucket, buckets: Bucket[]): string {
		const top = buckets[0]?.total ?? 1;
		const pct = top > 0 ? (b.total / top) * 100 : 0;
		return `${pct.toFixed(1)}%`;
	}
</script>

<svelte:head>
	<title>{$t('breakdown.title')} - RouteBox</title>
</svelte:head>

<div class="p-6 max-w-[1600px] mx-auto space-y-6">
	<!-- Header -->
	<div>
		<h1 class="text-2xl font-semibold text-[var(--ctp-text)]">{$t('breakdown.title')}</h1>
		<p class="text-sm text-[var(--ctp-overlay1)] mt-1">{$t('breakdown.subtitle')}</p>
	</div>

	<!-- Filter chips + totals -->
	<div class="bg-[var(--ctp-surface0)] rounded-lg border border-[var(--ctp-surface2)] p-4 flex flex-wrap items-center gap-3">
		<div class="text-sm text-[var(--ctp-overlay1)]">{$t('breakdown.totalTraffic')}:</div>
		<div class="text-sm text-[var(--ctp-text)]">
			<span class="text-[var(--ctp-overlay0)]">↑</span> {formatBytes(filteredTotal.up)}
			<span class="mx-2 text-[var(--ctp-overlay0)]">·</span>
			<span class="text-[var(--ctp-overlay0)]">↓</span> {formatBytes(filteredTotal.down)}
			<span class="mx-2 text-[var(--ctp-overlay0)]">·</span>
			<span class="text-[var(--ctp-overlay1)]">{filteredTotal.count} conn</span>
		</div>
		{#if hasAnyFilter}
			<div class="w-px h-5 bg-[var(--ctp-surface2)]"></div>
			<div class="text-sm text-[var(--ctp-overlay1)]">{$t('breakdown.filters')}:</div>
			<div class="flex flex-wrap gap-1.5">
				{#each ['source', 'domain', 'chain'] as Dimension[] as dim}
					{#if filters[dim] !== null}
						<button
							onclick={() => toggleFilter(dim, filters[dim]!)}
							class="flex items-center gap-1.5 px-2 py-0.5 text-xs rounded-full border border-[var(--ctp-primary)] bg-[color-mix(in_srgb,var(--ctp-primary)_10%,transparent)] text-[var(--ctp-primary)] hover:bg-[color-mix(in_srgb,var(--ctp-primary)_20%,transparent)]"
							title="Remove filter"
						>
							<span class="text-[10px] uppercase tracking-wide opacity-70">{dim}</span>
							<span>{filters[dim]}</span>
							<svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
							</svg>
						</button>
					{/if}
				{/each}
			</div>
			<button
				onclick={clearFilters}
				class="ml-auto px-3 py-1 text-xs text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)] border border-[var(--ctp-surface2)] rounded-lg hover:bg-[var(--ctp-surface1)]"
			>
				{$t('breakdown.clearFilters')}
			</button>
		{/if}
	</div>

	<!-- Three-panel breakdown -->
	{#if loading}
		<div class="flex items-center justify-center py-12">
			<svg class="animate-spin h-8 w-8 text-[var(--ctp-primary)]" fill="none" viewBox="0 0 24 24">
				<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
				<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
			</svg>
		</div>
	{:else}
		<div class="grid grid-cols-1 lg:grid-cols-3 gap-4">
			{@render panel('source', $t('breakdown.bySource'), sourceBuckets)}
			{@render panel('domain', $t('breakdown.byDomain'), domainBuckets)}
			{@render panel('chain', $t('breakdown.byChain'), chainBuckets)}
		</div>
	{/if}
</div>

{#snippet panel(dim: Dimension, title: string, buckets: Bucket[])}
	<div class="bg-[var(--ctp-surface0)] rounded-lg border border-[var(--ctp-surface2)] flex flex-col min-h-[24rem]">
		<div class="px-4 py-3 border-b border-[var(--ctp-surface2)] flex items-center justify-between">
			<h2 class="text-sm font-semibold text-[var(--ctp-subtext1)] uppercase tracking-wide">{title}</h2>
			<span class="text-xs text-[var(--ctp-overlay0)]">{buckets.length}</span>
		</div>
		<div class="flex-1 overflow-y-auto divide-y divide-[var(--ctp-surface2)]">
			{#if buckets.length === 0}
				<div class="px-4 py-8 text-center text-sm text-[var(--ctp-overlay0)]">
					{$t('breakdown.noMatches')}
				</div>
			{:else}
				{@const visible = expandedPanels[dim] ? buckets : buckets.slice(0, 10)}
				{#each visible as b (b.key)}
					{@const active = filters[dim] === b.key}
					<button
						onclick={() => toggleFilter(dim, b.key)}
						class="w-full text-left px-4 py-2 hover:bg-[var(--ctp-surface1)] transition-colors relative overflow-hidden"
						class:bg-[var(--ctp-surface1)]={active}
					>
						<!-- Background bar -->
						<div
							class="absolute inset-y-0 left-0 transition-all"
							class:bg-[color-mix(in_srgb,var(--ctp-primary)_18%,transparent)]={active}
							class:bg-[color-mix(in_srgb,var(--ctp-primary)_7%,transparent)]={!active}
							style="width: {barWidth(b, buckets)};"
						></div>
						<div class="relative flex items-baseline justify-between gap-3">
							<div class="min-w-0 flex-1 truncate text-sm" class:font-medium={active}
								class:text-[var(--ctp-primary)]={active}
								class:text-[var(--ctp-text)]={!active}
								title={b.key}>
								{b.key}
							</div>
							<div class="flex items-baseline gap-2 flex-shrink-0 font-mono tabular-nums text-xs">
								<span class="text-[var(--ctp-subtext1)]">{formatBytes(b.total)}</span>
								<span class="text-[var(--ctp-overlay0)]">·</span>
								<span class="text-[var(--ctp-overlay0)]">{b.connCount}</span>
							</div>
						</div>
						{#if dim === 'domain' && b.primaryIp}
							<div
								class="relative mt-0.5 text-[10px] font-mono text-[var(--ctp-overlay0)]"
								title={b.allIps?.join(', ')}
							>
								{b.primaryIp}{#if b.ipCount && b.ipCount > 1} (+{b.ipCount - 1}){/if}
							</div>
						{/if}
					</button>
				{/each}
				{#if buckets.length > 10}
					<button
						onclick={() => togglePanelExpand(dim)}
						class="w-full text-center px-4 py-2 text-xs text-[var(--ctp-overlay1)] hover:text-[var(--ctp-primary)] hover:bg-[var(--ctp-surface1)] transition-colors"
					>
						{expandedPanels[dim] ? $t('breakdown.showLess') : `${$t('breakdown.showAll')} (${buckets.length})`}
					</button>
				{/if}
			{/if}
		</div>
	</div>
{/snippet}
