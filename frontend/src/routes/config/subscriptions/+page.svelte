<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, unsavedChanges } from '$lib/stores';
	import Modal from '$lib/components/shared/Modal.svelte';
	import type { Subscription } from '$lib/types';

	let subs = $state<Subscription[]>([]);
	let loading = $state(true);
	let now = $state(Date.now());
	let revealed = $state<Record<string, boolean>>({});
	let busy = $state<Record<string, boolean>>({});
	let showAdd = $state(false);
	let addName = $state('');
	let addUrl = $state('');
	let addInterval = $state(12);
	let adding = $state(false);

	const intervalOptions = [0, 6, 12, 24];

	async function load() {
		try {
			subs = await api.getSubscriptions();
		} catch (e) {
			notifications.error(`${$t('subscriptions.loadFailed')}: ${e}`);
		} finally {
			loading = false;
		}
	}

	function maskUrl(url: string): string {
		try {
			const u = new URL(url);
			return `${u.protocol}//${u.host}/•••`;
		} catch {
			return '•••••';
		}
	}

	function relativeTime(unix: number): string {
		void now;
		if (!unix) return $t('subscriptions.never');
		const sec = Math.floor(Date.now() / 1000) - unix;
		if (sec < 60) return $t('subscriptions.timeAgo.justNow');
		if (sec < 3600) return $t('subscriptions.timeAgo.minutes', { values: { n: Math.floor(sec / 60) } });
		if (sec < 86400) return $t('subscriptions.timeAgo.hours', { values: { n: Math.floor(sec / 3600) } });
		return $t('subscriptions.timeAgo.days', { values: { n: Math.floor(sec / 86400) } });
	}

	function intervalLabel(hrs: number): string {
		return hrs === 0 ? $t('subscriptions.intervalOff') : $t('subscriptions.intervalHours', { values: { n: hrs } });
	}

	async function changeInterval(sub: Subscription, value: number) {
		try {
			const updated = await api.updateSubscription(sub.id, { url: sub.url, interval_hrs: value });
			subs = subs.map((s) => (s.id === sub.id ? updated : s));
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	async function refresh(sub: Subscription) {
		busy = { ...busy, [sub.id]: true };
		try {
			const updated = await api.refreshSubscription(sub.id);
			subs = subs.map((s) => (s.id === sub.id ? updated : s));
			now = Date.now();
			notifications.success($t('subscriptions.refreshed', { values: { name: sub.name } }));
			unsavedChanges.markChanged('Subscriptions', `Refreshed "${sub.name}"`);
		} catch (e) {
			notifications.error(`${e}`);
			await load();
		} finally {
			busy = { ...busy, [sub.id]: false };
		}
	}

	async function remove(sub: Subscription) {
		if (!confirm($t('subscriptions.deleteConfirm', { values: { name: sub.name } }))) return;
		try {
			await api.deleteSubscription(sub.id);
			subs = subs.filter((s) => s.id !== sub.id);
			unsavedChanges.markChanged('Subscriptions', `Removed "${sub.name}"`);
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	async function submitAdd() {
		if (!addName.trim() || !addUrl.trim()) return;
		adding = true;
		try {
			const created = await api.createSubscription({ name: addName.trim(), url: addUrl.trim(), interval_hrs: addInterval });
			subs = [...subs, created].sort((a, b) => a.name.localeCompare(b.name));
			showAdd = false;
			addName = '';
			addUrl = '';
			addInterval = 12;
			notifications.success($t('subscriptions.created', { values: { name: created.name } }));
			unsavedChanges.markChanged('Subscriptions', `Added subscription "${created.name}"`);
		} catch (e) {
			notifications.error(`${e}`);
		} finally {
			adding = false;
		}
	}

	function closeAdd() {
		showAdd = false;
	}

	onMount(() => {
		load();
		const clock = setInterval(() => (now = Date.now()), 30_000);
		return () => clearInterval(clock);
	});
</script>

<svelte:head><title>{$t('subscriptions.title')} - RouteBox</title></svelte:head>

<div class="space-y-4 max-w-4xl">
	<div class="flex items-center justify-between flex-wrap gap-3">
		<div>
			<h1 class="text-2xl font-bold text-[var(--ctp-text)]">{$t('subscriptions.title')}</h1>
			<p class="text-sm text-[var(--ctp-overlay1)] mt-1">{$t('subscriptions.description')}</p>
		</div>
		<button onclick={() => (showAdd = true)} class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity">
			{$t('subscriptions.add')}
		</button>
	</div>

	{#if loading}
		<div class="text-[var(--ctp-overlay0)]">{$t('common.loading')}</div>
	{:else if subs.length === 0}
		<div class="bg-[var(--ctp-surface0)] rounded-xl p-8 text-center text-[var(--ctp-overlay1)]">{$t('subscriptions.empty')}</div>
	{:else}
		<div class="space-y-3">
			{#each subs as sub (sub.id)}
				<section class="bg-[var(--ctp-mantle)] rounded-lg p-5 border border-[var(--ctp-surface0)]">
					<div class="flex items-start justify-between gap-4 flex-wrap">
						<div class="min-w-0">
							<h2 class="text-lg font-medium text-[var(--ctp-text)]">{sub.name}</h2>
							<button type="button" onclick={() => (revealed = { ...revealed, [sub.id]: !revealed[sub.id] })}
								class="mt-1 text-sm font-mono text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] break-all text-left" title={$t('subscriptions.revealUrl')}>
								{revealed[sub.id] ? sub.url : maskUrl(sub.url)}
							</button>
							<div class="mt-2 text-sm text-[var(--ctp-overlay1)]">
								{$t('subscriptions.nodeCount', { values: { n: sub.node_count } })}
								<span class="mx-2">·</span>
								{sub.last_updated ? $t('subscriptions.updated', { values: { time: relativeTime(sub.last_updated) } }) : $t('subscriptions.never')}
							</div>
							{#if sub.last_error}
								<div class="mt-2 text-sm text-[var(--ctp-red)]">{sub.last_error}</div>
							{/if}
						</div>
						<div class="flex items-center gap-2 flex-wrap">
							<select value={sub.interval_hrs} onchange={(e) => changeInterval(sub, parseInt(e.currentTarget.value, 10))}
								class="px-3 py-1.5 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded text-sm text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]">
								{#each intervalOptions as hrs}
									<option value={hrs}>{intervalLabel(hrs)}</option>
								{/each}
							</select>
							<button onclick={() => refresh(sub)} disabled={busy[sub.id]}
								class="px-3 py-1.5 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50 text-sm">
								{busy[sub.id] ? $t('subscriptions.refreshing') : $t('subscriptions.refresh')}
							</button>
							<button onclick={() => remove(sub)}
								class="px-3 py-1.5 text-[var(--ctp-red)] hover:bg-[var(--ctp-red)]/10 rounded-lg transition-colors text-sm">
								{$t('subscriptions.delete')}
							</button>
						</div>
					</div>
				</section>
			{/each}
		</div>
	{/if}
</div>

<Modal open={showAdd} title={$t('subscriptions.addTitle')} size="md" onClose={closeAdd}>
	<div class="space-y-3">
		<label class="block">
			<span class="text-sm text-[var(--ctp-subtext1)]">{$t('subscriptions.name')}</span>
			<input bind:value={addName} type="text" placeholder={$t('subscriptions.namePlaceholder')}
				class="mt-1 w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded text-[var(--ctp-text)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		</label>
		<label class="block">
			<span class="text-sm text-[var(--ctp-subtext1)]">{$t('subscriptions.url')}</span>
			<input bind:value={addUrl} type="url" placeholder="https://…"
				class="mt-1 w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded text-[var(--ctp-text)] text-sm font-mono focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		</label>
		<label class="block">
			<span class="text-sm text-[var(--ctp-subtext1)]">{$t('subscriptions.interval')}</span>
			<select bind:value={addInterval}
				class="mt-1 w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded text-[var(--ctp-text)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]">
				{#each intervalOptions as hrs}
					<option value={hrs}>{intervalLabel(hrs)}</option>
				{/each}
			</select>
		</label>
	</div>

	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<button onclick={closeAdd} class="px-4 py-2 text-[var(--ctp-subtext1)] hover:bg-[var(--ctp-surface0)] rounded-lg transition-colors">
				{$t('common.cancel')}
			</button>
			<button onclick={submitAdd} disabled={adding || !addName.trim() || !addUrl.trim()}
				class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50">
				{adding ? $t('subscriptions.adding') : $t('subscriptions.add')}
			</button>
		</div>
	{/snippet}
</Modal>
