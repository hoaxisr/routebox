<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';
	import type { ClashProxy } from '$lib/types';
	import ProxyCard from '$lib/components/monitor/ProxyCard.svelte';

	// State
	let proxies = $state<ClashProxy[]>([]);
	let loading = $state(true);
	let testingAll = $state(false);

	// Filters
	let filterType = $state('all');
	let sortBy = $state<'name' | 'delay'>('name');

	// Auto-refresh latency
	let autoRefresh = $state(false);
	let refreshInterval = $state(30); // seconds
	let autoRefreshTimer: ReturnType<typeof setInterval> | null = null;

	async function fetchProxies() {
		loading = true;
		try {
			const data = await api.getProxies();
			// Convert from object to array
			proxies = Object.values(data.proxies || {}).filter(
				(p): p is ClashProxy => p && typeof p === 'object' && 'name' in p
			);
		} catch (err) {
			notifications.error(`${$t('errors.loadFailed')}: ${err}`);
		} finally {
			loading = false;
		}
	}

	async function testAllProxies() {
		testingAll = true;
		const testableProxies = proxies.filter(p =>
			!['Direct', 'Reject', 'Block'].includes(p.type)
		);

		for (const proxy of testableProxies) {
			try {
				await api.testLatency(proxy.name);
			} catch {
				// Continue testing others
			}
		}

		await fetchProxies();
		testingAll = false;
		notifications.success($t('proxies.allProxiesTested'));
	}

	const filteredProxies = $derived(() => {
		let result = proxies;

		// Filter by type
		if (filterType !== 'all') {
			result = result.filter(p => {
				if (filterType === 'selector') return p.type === 'Selector' || p.type === 'URLTest';
				if (filterType === 'direct') return p.type === 'Direct' || p.type === 'Block' || p.type === 'Reject';
				if (filterType === 'endpoint') return !['Selector', 'URLTest', 'Direct', 'Block', 'Reject'].includes(p.type);
				return true;
			});
		}

		// Sort
		result = [...result].sort((a, b) => {
			if (sortBy === 'name') {
				return a.name.localeCompare(b.name);
			} else {
				const delayA = a.history?.[0]?.delay ?? Infinity;
				const delayB = b.history?.[0]?.delay ?? Infinity;
				return delayA - delayB;
			}
		});

		return result;
	});

	function startAutoRefresh() {
		if (autoRefreshTimer) return;
		autoRefreshTimer = setInterval(async () => {
			await testAllProxies();
		}, refreshInterval * 1000);
	}

	function stopAutoRefresh() {
		if (autoRefreshTimer) {
			clearInterval(autoRefreshTimer);
			autoRefreshTimer = null;
		}
	}

	$effect(() => {
		if (autoRefresh) {
			startAutoRefresh();
		} else {
			stopAutoRefresh();
		}
	});

	onMount(() => {
		fetchProxies();
	});

	onDestroy(() => {
		stopAutoRefresh();
	});
</script>

<svelte:head>
	<title>{$t('proxies.title')} - RouteBox</title>
</svelte:head>

<div class="p-6 max-w-6xl mx-auto">
	<!-- Header -->
	<div class="flex items-center justify-between mb-6">
		<div>
			<h1 class="text-2xl font-semibold text-[var(--ctp-text)]">{$t('proxies.title')}</h1>
			<p class="text-sm text-[var(--ctp-overlay1)] mt-1">
				{proxies.length === 1
					? $t('proxies.proxyCount', { values: { count: proxies.length } })
					: $t('proxies.proxyCountPlural', { values: { count: proxies.length } })}
			</p>
		</div>
		<div class="flex items-center gap-3">
			<!-- Auto-refresh toggle -->
			<div class="flex items-center gap-2 px-3 py-2 bg-[var(--ctp-surface0)] rounded-lg">
				<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)] cursor-pointer">
					<input
						type="checkbox"
						bind:checked={autoRefresh}
						class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
					/>
					{$t('proxies.autoRefresh')}
				</label>
				{#if autoRefresh}
					<select
						bind:value={refreshInterval}
						class="px-2 py-1 text-sm bg-[var(--ctp-surface1)] border border-[var(--ctp-surface2)] rounded text-[var(--ctp-text)] focus:outline-none focus:ring-1 focus:ring-[var(--ctp-primary)]"
					>
						<option value={15}>15s</option>
						<option value={30}>30s</option>
						<option value={60}>60s</option>
						<option value={120}>2m</option>
					</select>
				{/if}
			</div>

			<button
				onclick={fetchProxies}
				disabled={loading}
				class="p-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors disabled:opacity-50"
				title={$t('common.refresh')}
			>
				<svg class="w-5 h-5 {loading ? 'animate-spin' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
				</svg>
			</button>
			<button
				onclick={testAllProxies}
				disabled={testingAll || loading}
				class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 disabled:opacity-50 transition-opacity flex items-center gap-2"
			>
				{#if testingAll}
					<svg class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
						<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
						<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
					</svg>
					{$t('proxies.testing')}
				{:else}
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
					</svg>
					{$t('proxies.testAll')}
				{/if}
			</button>
		</div>
	</div>

	<!-- Filters -->
	<div class="flex items-center gap-4 mb-6">
		<div class="flex items-center gap-2">
			<label for="filter-type" class="text-sm text-[var(--ctp-subtext1)]">{$t('proxies.filterType')}:</label>
			<select
				id="filter-type"
				bind:value={filterType}
				class="px-3 py-1.5 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
			>
				<option value="all">{$t('proxies.filterAll')}</option>
				<option value="selector">{$t('proxies.filterSelectors')}</option>
				<option value="endpoint">{$t('proxies.filterEndpoints')}</option>
				<option value="direct">{$t('proxies.filterDirect')}</option>
			</select>
		</div>

		<div class="flex items-center gap-2">
			<label for="sort-by" class="text-sm text-[var(--ctp-subtext1)]">{$t('proxies.sortBy')}:</label>
			<select
				id="sort-by"
				bind:value={sortBy}
				class="px-3 py-1.5 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
			>
				<option value="name">{$t('proxies.sortByName')}</option>
				<option value="delay">{$t('proxies.sortByDelay')}</option>
			</select>
		</div>
	</div>

	<!-- Proxies Grid -->
	{#if loading}
		<div class="flex items-center justify-center py-12">
			<svg class="animate-spin h-8 w-8 text-[var(--ctp-primary)]" fill="none" viewBox="0 0 24 24">
				<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
				<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
			</svg>
		</div>
	{:else if filteredProxies().length === 0}
		<div class="text-center py-12 text-[var(--ctp-overlay0)]">
			{filterType === 'all' ? $t('proxies.noProxies') : $t('proxies.noProxiesMatchFilter')}
		</div>
	{:else}
		<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
			{#each filteredProxies() as proxy (proxy.name)}
				<ProxyCard {proxy} onUpdate={fetchProxies} />
			{/each}
		</div>
	{/if}
</div>
