<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, formatBytes } from '$lib/stores';
	import Sparkline from '$lib/components/shared/Sparkline.svelte';
	import type { PanelUser, UserTrafficResponse } from '$lib/types';

	let users = $state<PanelUser[]>([]);
	let sparks = $state<Record<string, number[]>>({});
	let loading = $state(true);

	async function load() {
		loading = true;
		try {
			users = (await api.getUsers()).filter((u) => !u.pending && u.id);
			await Promise.all(
				users.map(async (u) => {
					try {
						const r: UserTrafficResponse = await api.getUserTraffic(u.id, '24h');
						sparks[u.id] = r.history.map((p) => p.upload + p.download);
					} catch {
						/* fork without v2ray_api → no sparkline */
					}
				})
			);
			sparks = { ...sparks };
		} catch (e) {
			notifications.error(`${$t('monitor.usersLoadFailed')}: ${e}`);
		} finally {
			loading = false;
		}
	}
	onMount(load);
</script>

<svelte:head><title>{$t('monitor.perUserTitle')} - RouteBox</title></svelte:head>

<div class="space-y-4 max-w-4xl">
	<div>
		<h1 class="text-2xl font-bold text-[var(--ctp-text)]">{$t('monitor.perUserTitle')}</h1>
	</div>
	{#if loading}
		<div class="text-[var(--ctp-overlay0)]">{$t('common.loading')}</div>
	{:else if users.length === 0}
		<div class="bg-[var(--ctp-surface0)] rounded-xl p-8 text-center text-[var(--ctp-overlay1)]">{$t('monitor.usersEmpty')}</div>
	{:else}
		<div class="bg-[var(--ctp-mantle)] rounded-lg border border-[var(--ctp-surface0)] overflow-x-auto">
			<table class="w-full text-sm">
				<thead>
					<tr class="text-left text-[var(--ctp-overlay1)] border-b border-[var(--ctp-surface0)]">
						<th class="px-4 py-3 font-medium">{$t('users.name')}</th>
						<th class="px-4 py-3 font-medium">{$t('users.upload')}</th>
						<th class="px-4 py-3 font-medium">{$t('users.download')}</th>
						<th class="px-4 py-3 font-medium">{$t('users.usage')}</th>
					</tr>
				</thead>
				<tbody>
					{#each users as u (u.id)}
						<tr class="border-b border-[var(--ctp-surface0)] last:border-0">
							<td class="px-4 py-3 text-[var(--ctp-text)]">{u.name}</td>
							<td class="px-4 py-3 text-[var(--ctp-primary)] tabular-nums">↑ {formatBytes(u.upload ?? 0)}</td>
							<td class="px-4 py-3 text-[var(--ctp-overlay1)] tabular-nums">↓ {formatBytes(u.download ?? 0)}</td>
							<td class="px-4 py-3"><Sparkline values={sparks[u.id] ?? []} /></td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
