<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import RouteInspector from '$lib/components/config/RouteInspector.svelte';
	import type { RouteRule, RouteSettings } from '$lib/types';

	let rules = $state<RouteRule[]>([]);
	let settings = $state<RouteSettings | null>(null);
	let loading = $state(true);
	let error = $state('');

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
	<title>{$t('nav.routeInspector')} | RouteBox</title>
</svelte:head>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-[var(--ctp-text)]">{$t('nav.routeInspector')}</h1>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<div class="text-[var(--ctp-overlay0)]">{$t('common.loading')}</div>
		</div>
	{:else if error}
		<div class="bg-[var(--ctp-red)] bg-opacity-10 border border-[var(--ctp-red)] rounded-lg p-4">
			<p class="text-[var(--ctp-red)]">{error}</p>
		</div>
	{:else if settings}
		<RouteInspector {rules} finalOutbound={settings.final} />
	{/if}
</div>
