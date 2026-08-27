<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, unsavedChanges } from '$lib/stores';
	import { copyText } from '$lib/utils/clipboard';
	import type { Inbound, DestNaive } from '$lib/types';
	import Modal from '$lib/components/shared/Modal.svelte';
	import InboundForm from '$lib/components/config/InboundForm.svelte';
	import { inboundDisplayAddress } from '$lib/utils/inboundAddress';

	let inbounds = $state<Inbound[]>([]);
	let naive = $state<DestNaive | null>(null);
	let copied = $state<string>('');
	let loading = $state(true);
	// Server inbounds bind the wildcard; the list shows the address clients dial (#37).
	let publicHost = $state('');

	// Modal state
	let showModal = $state(false);
	let editingInbound = $state<Inbound | undefined>(undefined);
	let saving = $state(false);

	async function fetchInbounds() {
		try {
			inbounds = await api.listInbounds();
		} catch (e) {
			notifications.error($t('errors.loadFailed'));
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		editingInbound = undefined;
		showModal = true;
	}

	function openEdit(inbound: Inbound) {
		editingInbound = inbound;
		showModal = true;
	}

	function closeModal() {
		showModal = false;
		editingInbound = undefined;
	}

	async function handleSave(inbound: Inbound) {
		saving = true;
		try {
			if (editingInbound) {
				await api.updateInbound(editingInbound.tag, inbound);
				notifications.success($t('inbounds.inboundUpdated'));
				unsavedChanges.markChanged($t('inbounds.title'), `${$t('common.update')} "${inbound.tag}"`);
			} else {
				await api.createInbound(inbound);
				notifications.success($t('inbounds.inboundCreated'));
				unsavedChanges.markChanged($t('inbounds.title'), `${$t('common.create')} "${inbound.tag}"`);
			}
			closeModal();
			await fetchInbounds();
		} catch (e) {
			notifications.error($t('errors.saveFailed'));
		} finally {
			saving = false;
		}
	}

	async function handleDelete(tag: string) {
		if (!confirm($t('inbounds.confirmDelete', { values: { name: tag } }))) return;

		try {
			await api.deleteInbound(tag);
			notifications.success($t('inbounds.inboundDeleted'));
			unsavedChanges.markChanged($t('inbounds.title'), `${$t('common.delete')} "${tag}"`);
			await fetchInbounds();
		} catch (e) {
			notifications.error($t('errors.deleteFailed'));
		}
	}

	function getInboundColor(type: string) {
		switch (type) {
			case 'tun':
				return 'var(--ctp-primary)';
			case 'mixed':
				return 'var(--ctp-primary)';
			case 'socks':
				return 'var(--ctp-primary)';
			case 'http':
				return 'var(--ctp-primary)';
			default:
				return 'var(--ctp-overlay1)';
		}
	}

	function getInboundIcon(type: string) {
		switch (type) {
			case 'tun':
				return 'M13 10V3L4 14h7v7l9-11h-7z'; // lightning bolt for system-level
			case 'mixed':
				return 'M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z'; // overlapping squares
			default:
				return 'M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2';
		}
	}

	async function copyLink(name: string, link: string) {
		if (!link) return;
		if (await copyText(link)) {
			copied = name;
			setTimeout(() => (copied = ''), 1500);
		} else {
			notifications.error($t('common.copyFailed'));
		}
	}

	onMount(async () => {
		await fetchInbounds();
		try {
			publicHost = (await api.getSettings()).settings.server?.public_host ?? '';
		} catch {
			/* no public host → the list falls back to a bare wildcard */
		}
		try {
			// naive is served by dest, not sing-box, so it is not in the list above.
			// Without this card the operator sees four protocols on a server that
			// runs five, and has no way to hand anyone naive access.
			const n = await api.getDestNaive();
			naive = n.enabled ? n : null;
		} catch {
			/* an install with no dest simply has no naive node */
		}
	});
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-[var(--ctp-text)]">{$t('inbounds.title')}</h1>
		<button
			onclick={openCreate}
			class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
		>
			+ {$t('inbounds.addInbound')}
		</button>
	</div>

	{#if naive}
		<div class="bg-[var(--ctp-surface0)] rounded-xl p-4 border border-[var(--ctp-surface2)]">
			<div class="flex items-center gap-2 flex-wrap">
				<span class="font-semibold text-[var(--ctp-text)]">naive</span>
				<span class="px-2 py-0.5 text-xs bg-[var(--ctp-surface2)] rounded text-[var(--ctp-subtext1)]">
					{naive.host}{naive.port && naive.port !== 443 ? ':' + naive.port : ''}
				</span>
				<span class="text-xs text-[var(--ctp-overlay1)]">{$t('inbounds.naiveServedByDest')}</span>
			</div>
			<div class="mt-3 space-y-1.5">
				{#each naive.users as u}
					<div class="flex items-center gap-2 text-sm">
						<span class="text-[var(--ctp-subtext1)] min-w-[8rem] truncate">{u.name}</span>
						{#if u.link}
							<button
								onclick={() => copyLink(u.name, u.link)}
								class="px-2 py-1 text-xs rounded bg-[var(--ctp-surface2)] text-[var(--ctp-subtext1)] hover:bg-[var(--ctp-surface1)]"
							>
								{copied === u.name ? $t('common.copied') : $t('inbounds.naiveCopyLink')}
							</button>
						{:else}
							<span class="text-xs text-[var(--ctp-overlay0)]">{$t('inbounds.naiveNoLink')}</span>
						{/if}
					</div>
				{/each}
			</div>
		</div>
	{/if}

	{#if loading}
		<div class="text-[var(--ctp-overlay0)]">{$t('common.loading')}</div>
	{:else if inbounds.length === 0}
		<div class="bg-[var(--ctp-surface0)] rounded-xl p-8 text-center">
			<div class="text-[var(--ctp-overlay1)] mb-4">{$t('inbounds.noInbounds')}</div>
			<button
				onclick={openCreate}
				class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
			>
				{$t('inbounds.addInbound')}
			</button>
		</div>
	{:else}
		<div class="space-y-3">
			{#each inbounds as inbound}
				<div class="bg-[var(--ctp-surface0)] rounded-xl p-4 hover:bg-[var(--ctp-surface1)] transition-colors">
					<div class="flex items-center justify-between">
						<div class="flex items-center gap-4">
							<div
								class="w-10 h-10 rounded-lg flex items-center justify-center"
								style="background-color: color-mix(in srgb, {getInboundColor(inbound.type)} 20%, transparent)"
							>
								<svg class="w-5 h-5" style="color: {getInboundColor(inbound.type)}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={getInboundIcon(inbound.type)} />
								</svg>
							</div>
							<div>
								<div class="flex items-center gap-2">
									<span class="font-semibold text-[var(--ctp-text)]">{inbound.tag}</span>
									<span class="px-2 py-0.5 text-xs bg-[var(--ctp-surface2)] rounded text-[var(--ctp-subtext1)]">
										{inbound.type.toUpperCase()}
									</span>
								</div>
								<div class="text-sm text-[var(--ctp-overlay1)] mt-1">
									{#if inbound.type === 'tun'}
										{inbound.interface_name ?? 'sing0'}
										{#if inbound.auto_route}
											<span class="text-[var(--ctp-primary)]">• auto_route</span>
										{/if}
									{:else}
										{inboundDisplayAddress(inbound, publicHost)}
									{/if}
								</div>
							</div>
						</div>

						<div class="flex items-center gap-2">
							<button
								onclick={() => openEdit(inbound)}
								class="p-2 hover:bg-[var(--ctp-surface2)] rounded-lg transition-colors"
								title={$t('common.edit')}
							>
								<svg class="w-5 h-5 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
								</svg>
							</button>
							<button
								onclick={() => handleDelete(inbound.tag)}
								class="action-btn-danger"
								title={$t('common.delete')}
							>
								<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
								</svg>
							</button>
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<!-- Modal -->
<Modal open={showModal} title={editingInbound ? $t('inbounds.editInbound') : $t('inbounds.addInbound')} size="lg" onClose={closeModal}>
	<InboundForm inbound={editingInbound} onSave={handleSave} onCancel={closeModal} {publicHost} />
</Modal>
