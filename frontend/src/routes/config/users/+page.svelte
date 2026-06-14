<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, unsavedChanges } from '$lib/stores';
	import Modal from '$lib/components/shared/Modal.svelte';
	import ShareLinkModal from '$lib/components/config/inbound/ShareLinkModal.svelte';
	import type { PanelUser, Inbound } from '$lib/types';

	let users = $state<PanelUser[]>([]);
	let serverInbounds = $state<{ tag: string; type: string }[]>([]);
	let loading = $state(true);

	let showAdd = $state(false);
	let addName = $state('');
	let addTag = $state('');
	let adding = $state(false);

	// Share modal state (registry id + chosen binding tag).
	let shareId = $state<string | null>(null);
	let shareTag = $state('');

	const serverTypes = ['vless', 'naive', 'hysteria2'];

	async function load() {
		loading = true;
		try {
			users = await api.getUsers();
			const inbounds = await api.listInbounds();
			serverInbounds = (inbounds as Inbound[])
				.filter((i) => serverTypes.includes(i.type))
				.map((i) => ({ tag: i.tag, type: i.type }));
			if (!addTag && serverInbounds.length > 0) addTag = serverInbounds[0].tag;
		} catch (e) {
			notifications.error(`${$t('users.loadFailed')}: ${e}`);
		} finally {
			loading = false;
		}
	}

	function protocolOf(tag: string): string {
		return serverInbounds.find((i) => i.tag === tag)?.type ?? 'vless';
	}

	async function submitAdd() {
		if (!addName.trim() || !addTag) return;
		adding = true;
		try {
			const protocol = protocolOf(addTag);
			await api.createUser({ name: addName.trim(), protocol, inbound_tag: addTag });
			notifications.success($t('users.created', { values: { name: addName.trim() } }));
			unsavedChanges.markChanged('Users', `Added user "${addName.trim()}"`);
			showAdd = false;
			addName = '';
			await load();
		} catch (e) {
			notifications.error(`${e}`);
		} finally {
			adding = false;
		}
	}

	async function remove(u: PanelUser) {
		if (!confirm($t('users.deleteConfirm', { values: { name: u.name } }))) return;
		try {
			await api.deleteUser(u.id);
			unsavedChanges.markChanged('Users', `Removed user "${u.name}"`);
			await load();
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	function openShare(u: PanelUser) {
		if (u.pending || u.bindings.length === 0) return;
		shareId = u.id;
		shareTag = u.bindings[0].inbound_tag;
	}

	onMount(load);
</script>

<svelte:head><title>{$t('users.title')} - RouteBox</title></svelte:head>

<div class="space-y-4 max-w-4xl">
	<div class="flex items-center justify-between flex-wrap gap-3">
		<div>
			<h1 class="text-2xl font-bold text-[var(--ctp-text)]">{$t('users.title')}</h1>
			<p class="text-sm text-[var(--ctp-overlay1)] mt-1">{$t('users.description')}</p>
		</div>
		<button onclick={() => (showAdd = true)} disabled={serverInbounds.length === 0}
			class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50">
			{$t('users.add')}
		</button>
	</div>

	{#if loading}
		<div class="text-[var(--ctp-overlay0)]">{$t('common.loading')}</div>
	{:else if serverInbounds.length === 0}
		<div class="bg-[var(--ctp-surface0)] rounded-xl p-8 text-center text-[var(--ctp-overlay1)]">{$t('users.noServerInbounds')}</div>
	{:else if users.length === 0}
		<div class="bg-[var(--ctp-surface0)] rounded-xl p-8 text-center text-[var(--ctp-overlay1)]">{$t('users.empty')}</div>
	{:else}
		<div class="space-y-3">
			{#each users as u, i (u.id || `${u.name} ${u.bindings[0]?.credential ?? ''} ${i}`)}
				<section class="bg-[var(--ctp-mantle)] rounded-lg p-5 border border-[var(--ctp-surface0)]">
					<div class="flex items-start justify-between gap-4 flex-wrap">
						<div class="min-w-0">
							<div class="flex items-center gap-2">
								<h2 class="text-lg font-medium text-[var(--ctp-text)]">{u.name || '(unnamed)'}</h2>
								{#if u.pending}
									<span class="status-badge info">{$t('users.pending')}</span>
								{:else}
									<span class="status-badge success">{$t('users.active')}</span>
								{/if}
							</div>
							<div class="mt-2 flex flex-wrap gap-1">
								{#each u.bindings as b}
									<span class="status-badge">{b.inbound_tag}</span>
								{/each}
							</div>
						</div>
						<div class="flex items-center gap-2 flex-wrap">
							{#if u.pending}
								<span class="text-xs text-[var(--ctp-overlay0)]">{$t('users.pendingNoLink')}</span>
							{:else}
								<button onclick={() => openShare(u)}
									class="px-3 py-1.5 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity text-sm">
									{$t('users.clientLink')}
								</button>
							{/if}
							{#if u.id}
								<button onclick={() => remove(u)}
									class="px-3 py-1.5 text-[var(--ctp-red)] hover:bg-[var(--ctp-red)]/10 rounded-lg transition-colors text-sm">
									{$t('users.delete')}
								</button>
							{/if}
						</div>
					</div>
				</section>
			{/each}
		</div>
	{/if}
</div>

<Modal open={showAdd} title={$t('users.addTitle')} size="md" onClose={() => (showAdd = false)}>
	<div class="space-y-3">
		<label class="block">
			<span class="text-sm text-[var(--ctp-subtext1)]">{$t('users.name')}</span>
			<input bind:value={addName} type="text"
				class="mt-1 w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded text-[var(--ctp-text)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		</label>
		<label class="block">
			<span class="text-sm text-[var(--ctp-subtext1)]">{$t('users.inbound')}</span>
			<select bind:value={addTag}
				class="mt-1 w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded text-[var(--ctp-text)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]">
				{#each serverInbounds as i}
					<option value={i.tag}>{i.tag} ({i.type})</option>
				{/each}
			</select>
		</label>
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<button onclick={() => (showAdd = false)} class="px-4 py-2 text-[var(--ctp-subtext1)] hover:bg-[var(--ctp-surface0)] rounded-lg transition-colors">
				{$t('common.cancel')}
			</button>
			<button onclick={submitAdd} disabled={adding || !addName.trim() || !addTag}
				class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50">
				{adding ? $t('common.loading') : $t('users.add')}
			</button>
		</div>
	{/snippet}
</Modal>

{#if shareId}
	<ShareLinkModal id={shareId} tag={shareTag} onClose={() => (shareId = null)} />
{/if}
