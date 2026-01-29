<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';
	import type { RouteRule, Outbound, Endpoint } from '$lib/types';
	import { VisualBuilder } from '$lib/components/visual-builder';

	let rules = $state<RouteRule[]>([]);
	let outbounds = $state<Outbound[]>([]);
	let endpoints = $state<Endpoint[]>([]);
	let loading = $state(true);

	// Combined outbounds: outbounds + endpoints
	let allOutbounds = $derived([
		...outbounds,
		...endpoints.map((e) => ({ tag: e.tag, type: e.type }) as Outbound)
	]);

	// Selected rule index for side panel (future feature)
	let selectedRuleIndex = $state<number | null>(null);

	async function fetchData() {
		try {
			const [rulesData, outboundsData, endpointsData] = await Promise.all([
				api.listRules(),
				api.listOutbounds(),
				api.listEndpoints()
			]);
			rules = rulesData;
			outbounds = outboundsData;
			endpoints = endpointsData;
		} catch (e) {
			notifications.error(`Failed to load: ${e}`);
		} finally {
			loading = false;
		}
	}

	function handleRuleSelect(index: number | null) {
		selectedRuleIndex = index;
	}

	onMount(() => {
		fetchData();
	});
</script>

<svelte:head>
	<title>{$t('visualBuilder.title')} - RouteBox</title>
</svelte:head>

<div class="page-header">
	<div class="flex items-center gap-3">
		<h1 class="text-2xl font-semibold text-[var(--ctp-text)]">{$t('visualBuilder.title')}</h1>
		<span class="status-badge info">{$t('visualBuilder.beta')}</span>
	</div>
	<p class="text-[var(--ctp-subtext1)] mt-1">{$t('visualBuilder.description')}</p>
</div>

{#if loading}
	<div class="flex items-center justify-center py-20">
		<svg class="w-8 h-8 animate-spin text-[var(--ctp-primary)]" fill="none" viewBox="0 0 24 24">
			<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"
			></circle>
			<path
				class="opacity-75"
				fill="currentColor"
				d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
			></path>
		</svg>
	</div>
{:else if rules.length === 0}
	<div class="empty-state">
		<svg class="w-12 h-12 text-[var(--ctp-overlay1)] mx-auto" fill="none" stroke="currentColor" viewBox="0 0 24 24">
			<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l4.553 2.276A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7" />
		</svg>
		<h3 class="mt-4 text-lg font-medium text-[var(--ctp-text)]">{$t('visualBuilder.noRules')}</h3>
		<p class="mt-2 text-[var(--ctp-subtext1)]">{$t('visualBuilder.noRulesHint')}</p>
		<a href="/config/routes" class="btn-primary mt-4 inline-block">
			{$t('visualBuilder.goToRules')}
		</a>
	</div>
{:else}
	<div class="mt-6">
		<div class="stats-bar">
			<div class="stat">
				<span class="stat-value">{rules.length}</span>
				<span class="stat-label">{$t('visualBuilder.rules')}</span>
			</div>
			<div class="stat">
				<span class="stat-value">{allOutbounds.length}</span>
				<span class="stat-label">{$t('visualBuilder.outbounds')}</span>
			</div>
		</div>

		<VisualBuilder {rules} outbounds={allOutbounds} onRuleSelect={handleRuleSelect} />

		<div class="help-text mt-4">
			<p>{$t('visualBuilder.helpText')}</p>
		</div>
	</div>
{/if}

<style>
	.page-header {
		margin-bottom: 1.5rem;
	}

	.empty-state {
		text-align: center;
		padding: 4rem 2rem;
		background: var(--ctp-mantle);
		border-radius: 0.5rem;
		border: 1px solid var(--ctp-surface0);
	}

	.stats-bar {
		display: flex;
		gap: 2rem;
		margin-bottom: 1rem;
		padding: 0.75rem 1rem;
		background: var(--ctp-mantle);
		border-radius: 0.5rem;
		border: 1px solid var(--ctp-surface0);
	}

	.stat {
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
	}

	.stat-value {
		font-size: 1.5rem;
		font-weight: 600;
		color: var(--ctp-text);
	}

	.stat-label {
		color: var(--ctp-subtext1);
		font-size: 0.875rem;
	}

	.help-text {
		color: var(--ctp-overlay1);
		font-size: 0.875rem;
		text-align: center;
	}

	.btn-primary {
		padding: 0.5rem 1rem;
		background: var(--ctp-primary);
		color: white;
		border-radius: 0.375rem;
		font-weight: 500;
		text-decoration: none;
		transition: background 0.15s ease;
	}

	.btn-primary:hover {
		filter: brightness(1.1);
	}
</style>
