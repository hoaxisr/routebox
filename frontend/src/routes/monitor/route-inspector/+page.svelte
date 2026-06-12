<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import RouteInspector from '$lib/components/config/RouteInspector.svelte';
	import ConnectionTest from '$lib/components/monitor/ConnectionTest.svelte';
	import type { RouteRule, RouteSettings } from '$lib/types';

	let rules = $state<RouteRule[]>([]);
	let settings = $state<RouteSettings | null>(null);
	let loading = $state(true);
	let error = $state('');
	let activeTab = $state<'route' | 'connection'>('route');

	onMount(async () => {
		try {
			const [rulesData, settingsData] = await Promise.all([
				api.listRules(),
				api.getRouteSettings()
			]);
			rules = rulesData;
			settings = settingsData;
		} catch (e) {
			error = `${e}`;
		} finally {
			loading = false;
		}
	});
</script>

<svelte:head>
	<title>{$t('nav.routeInspector')} - RouteBox</title>
</svelte:head>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-[var(--ctp-text)]">{$t('nav.routeInspector')}</h1>
	</div>

	<!-- Tabs -->
	<div class="flex gap-2">
		<button
			onclick={() => (activeTab = 'route')}
			class="px-4 py-2 rounded-lg transition-colors {activeTab === 'route'
				? 'bg-[var(--ctp-primary)] text-white'
				: 'bg-[var(--ctp-surface0)] text-[var(--ctp-text)] hover:bg-[var(--ctp-surface1)]'}"
		>
			{$t('routes.inspector.routeTest')}
		</button>
		<button
			onclick={() => (activeTab = 'connection')}
			class="px-4 py-2 rounded-lg transition-colors {activeTab === 'connection'
				? 'bg-[var(--ctp-primary)] text-white'
				: 'bg-[var(--ctp-surface0)] text-[var(--ctp-text)] hover:bg-[var(--ctp-surface1)]'}"
		>
			{$t('routes.inspector.connectionTest')}
		</button>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<div class="text-[var(--ctp-overlay0)]">{$t('common.loading')}</div>
		</div>
	{:else if error}
		<div class="bg-[var(--ctp-red)] bg-opacity-10 border border-[var(--ctp-red)] rounded-lg p-4">
			<p class="text-[var(--ctp-red)]">{error}</p>
		</div>
	{:else if activeTab === 'route' && settings}
		<RouteInspector {rules} finalOutbound={settings.final} />
	{:else if activeTab === 'connection'}
		<ConnectionTest />
	{/if}
</div>
