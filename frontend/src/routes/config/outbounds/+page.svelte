<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { notifications, unsavedChanges } from '$lib/stores';
	import type { Outbound, Endpoint } from '$lib/types';
	import Modal from '$lib/components/shared/Modal.svelte';
	import OutboundForm from '$lib/components/config/OutboundForm.svelte';

	let outbounds = $state<Outbound[]>([]);
	let endpoints = $state<Endpoint[]>([]);
	let loading = $state(true);

	// Modal state
	let showModal = $state(false);
	let editingOutbound = $state<Outbound | undefined>(undefined);
	let saving = $state(false);

	async function fetchData() {
		try {
			const [ob, ep] = await Promise.all([
				api.listOutbounds(),
				api.listEndpoints()
			]);
			outbounds = ob;
			endpoints = ep;
		} catch (e) {
			notifications.error(`Failed to load data: ${e}`);
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		editingOutbound = undefined;
		showModal = true;
	}

	function openEdit(outbound: Outbound) {
		editingOutbound = outbound;
		showModal = true;
	}

	function closeModal() {
		showModal = false;
		editingOutbound = undefined;
	}

	async function handleSave(outbound: Outbound) {
		saving = true;
		try {
			if (editingOutbound) {
				await api.updateOutbound(editingOutbound.tag, outbound);
				notifications.success(`Outbound "${outbound.tag}" updated`);
				unsavedChanges.markChanged('Outbounds', `Updated "${outbound.tag}"`);
			} else {
				await api.createOutbound(outbound);
				notifications.success(`Outbound "${outbound.tag}" created`);
				unsavedChanges.markChanged('Outbounds', `Created "${outbound.tag}"`);
			}
			closeModal();
			await fetchData();
		} catch (e) {
			notifications.error(`Failed to save: ${e}`);
		} finally {
			saving = false;
		}
	}

	async function handleDelete(tag: string) {
		if (!confirm(`Delete outbound "${tag}"?`)) return;

		try {
			await api.deleteOutbound(tag);
			notifications.success(`Outbound "${tag}" deleted`);
			unsavedChanges.markChanged('Outbounds', `Deleted "${tag}"`);
			await fetchData();
		} catch (e) {
			notifications.error(`Failed to delete: ${e}`);
		}
	}

	async function applyChanges() {
		try {
			const result = await api.applyConfig();
			notifications.success(result.message);
			unsavedChanges.clearChanges();
		} catch (e) {
			notifications.error(`Failed to apply: ${e}`);
		}
	}

	function getOutboundColor(type: string) {
		switch (type) {
			case 'direct':
				return 'var(--ctp-primary)';
			case 'selector':
				return 'var(--ctp-primary)';
			case 'urltest':
				return 'var(--ctp-primary)';
			case 'block':
				return 'var(--ctp-red)';
			case 'endpoint':
				return 'var(--ctp-primary)';
			default:
				return 'var(--ctp-primary)';
		}
	}

	onMount(fetchData);
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-[var(--ctp-text)]">Outbounds</h1>
		<div class="flex items-center gap-2">
			<button
				onclick={applyChanges}
				class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
			>
				Apply Changes
			</button>
			<button
				onclick={openCreate}
				class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
			>
				+ Add Outbound
			</button>
		</div>
	</div>

	{#if loading}
		<div class="text-[var(--ctp-overlay0)]">Loading...</div>
	{:else if outbounds.length === 0}
		<div class="bg-[var(--ctp-surface0)] rounded-xl p-8 text-center">
			<div class="text-[var(--ctp-overlay1)] mb-4">No outbounds configured</div>
			<button
				onclick={openCreate}
				class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
			>
				Add your first outbound
			</button>
		</div>
	{:else}
		<div class="space-y-3">
			{#each outbounds as outbound}
				<div class="bg-[var(--ctp-surface0)] rounded-xl p-4 hover:bg-[var(--ctp-surface1)] transition-colors">
					<div class="flex items-center justify-between">
						<div class="flex items-center gap-4">
							<div
								class="w-3 h-3 rounded-full"
								style="background-color: {getOutboundColor(outbound.type)}"
							></div>
							<div>
								<div class="flex items-center gap-2">
									<span class="font-semibold text-[var(--ctp-text)]">{outbound.tag}</span>
									<span class="px-2 py-0.5 text-xs bg-[var(--ctp-surface2)] rounded text-[var(--ctp-subtext1)]">
										{outbound.type}
									</span>
								</div>
								{#if outbound.outbounds && outbound.outbounds.length > 0}
									<div class="text-sm text-[var(--ctp-overlay1)] mt-1">
										{outbound.outbounds.join(' → ')}
									</div>
								{/if}
								{#if outbound.server}
									<div class="text-sm text-[var(--ctp-overlay0)] mt-1">
										{outbound.server}:{outbound.server_port}
									</div>
								{/if}
							</div>
						</div>

						<div class="flex items-center gap-2">
							<button
								onclick={() => openEdit(outbound)}
								class="p-2 hover:bg-[var(--ctp-surface2)] rounded-lg transition-colors"
								title="Edit"
							>
								<svg class="w-5 h-5 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
								</svg>
							</button>
							<button
								onclick={() => handleDelete(outbound.tag)}
								class="action-btn-danger"
								title="Delete"
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
<Modal open={showModal} title={editingOutbound ? 'Edit Outbound' : 'Add Outbound'} size="lg" onClose={closeModal}>
	<OutboundForm outbound={editingOutbound} {endpoints} {outbounds} onSave={handleSave} onCancel={closeModal} />
</Modal>
