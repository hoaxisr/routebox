<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, unsavedChanges } from '$lib/stores';
	import type { Endpoint, Outbound } from '$lib/types';
	import Modal from '$lib/components/shared/Modal.svelte';
	import EndpointForm from '$lib/components/config/EndpointForm.svelte';

	let endpoints = $state<Endpoint[]>([]);
	let outbounds = $state<Outbound[]>([]);
	let loading = $state(true);

	// Modal state
	let showModal = $state(false);
	let editingEndpoint = $state<Endpoint | undefined>(undefined);
	let saving = $state(false);

	// Service outbound types (groups/selectors)
	const serviceTypes = ['selector', 'urltest'];

	// Find which service outbounds (selector/urltest) contain each endpoint tag
	let containedIn = $derived.by(() => {
		const map = new Map<string, string[]>();
		const serviceOutbounds = outbounds.filter(o => serviceTypes.includes(o.type));
		for (const service of serviceOutbounds) {
			if (service.outbounds) {
				for (const memberTag of service.outbounds) {
					const existing = map.get(memberTag) || [];
					existing.push(service.tag);
					map.set(memberTag, existing);
				}
			}
		}
		return map;
	});

	async function fetchEndpoints() {
		try {
			const [ep, ob] = await Promise.all([
				api.listEndpoints(),
				api.listOutbounds()
			]);
			endpoints = ep;
			outbounds = ob;
		} catch (e) {
			notifications.error(`Failed to load endpoints: ${e}`);
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		editingEndpoint = undefined;
		showModal = true;
	}

	function openEdit(endpoint: Endpoint) {
		editingEndpoint = endpoint;
		showModal = true;
	}

	function closeModal() {
		showModal = false;
		editingEndpoint = undefined;
	}

	async function handleSave(endpoint: Endpoint) {
		saving = true;
		try {
			if (editingEndpoint) {
				await api.updateEndpoint(editingEndpoint.tag, endpoint);
				notifications.success(`Endpoint "${endpoint.tag}" updated`);
				unsavedChanges.markChanged('Endpoints', `Updated "${endpoint.tag}"`);
			} else {
				await api.createEndpoint(endpoint);
				notifications.success(`Endpoint "${endpoint.tag}" created`);
				unsavedChanges.markChanged('Endpoints', `Created "${endpoint.tag}"`);
			}
			closeModal();
			await fetchEndpoints();
		} catch (e) {
			notifications.error(`Failed to save: ${e}`);
		} finally {
			saving = false;
		}
	}

	async function handleDelete(tag: string) {
		if (!confirm(`Delete endpoint "${tag}"?`)) return;

		try {
			await api.deleteEndpoint(tag);
			notifications.success(`Endpoint "${tag}" deleted`);
			unsavedChanges.markChanged('Endpoints', `Deleted "${tag}"`);
			await fetchEndpoints();
		} catch (e) {
			notifications.error(`Failed to delete: ${e}`);
		}
	}

	function getEndpointIcon(type: string) {
		switch (type) {
			case 'awg':
				return 'M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z';
			case 'wireguard':
				return 'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z';
			default:
				return 'M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2';
		}
	}

	/**
	 * Determines AWG obfuscation version based on configured parameters.
	 * Returns 'AWG 2.0', 'AWG 1.5', 'AWG 1.0', or 'WG'
	 */
	function getAWGVersion(ep: Endpoint): string {
		// Check which parameter groups are configured
		const hasI = !!(ep.i1 || ep.i2 || ep.i3 || ep.i4 || ep.i5);
		const hasS3S4 = !!(ep.s3 || ep.s4);
		const hasJunk = !!(ep.jc || ep.jmin || ep.jmax);

		// Check if h1-h4 contain ranges (contain '-' character)
		const isRange = (val: string | undefined) => val && val.includes('-');
		const hasHRanges = isRange(ep.h1) || isRange(ep.h2) || isRange(ep.h3) || isRange(ep.h4);

		// AWG 2.0: s3-s4 configured (with or without i1-i5) OR h1-h4 as ranges
		if (hasS3S4 || hasHRanges) {
			return 'AWG 2.0';
		}

		// AWG 1.5: i1-i5 configured without s3-s4
		if (hasI) {
			return 'AWG 1.5';
		}

		// AWG 1.0: only junk packets (jc/jmin/jmax) configured
		if (hasJunk) {
			return 'AWG 1.0';
		}

		// WG: no obfuscation parameters configured
		return 'WG';
	}

	onMount(fetchEndpoints);
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-[var(--ctp-text)]">{$t('endpoints.title')}</h1>
		<button
			onclick={openCreate}
			class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
		>
			+ {$t('endpoints.addEndpoint')}
		</button>
	</div>

	{#if loading}
		<div class="text-[var(--ctp-overlay0)]">{$t('common.loading')}</div>
	{:else if endpoints.length === 0}
		<div class="bg-[var(--ctp-surface0)] rounded-xl p-8 text-center">
			<div class="text-[var(--ctp-overlay1)] mb-4">{$t('endpoints.noEndpoints')}</div>
			<button
				onclick={openCreate}
				class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
			>
				{$t('endpoints.noEndpointsHint')}
			</button>
		</div>
	{:else}
		<div class="space-y-4">
			{#each endpoints as endpoint}
				{@const groups = containedIn.get(endpoint.tag) || []}
				<div class="bg-[var(--ctp-surface0)] rounded-xl p-4 hover:bg-[var(--ctp-surface1)] transition-colors">
					<div class="flex items-start justify-between">
						<div class="flex items-start gap-4">
							<div class="icon-badge">
								<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={getEndpointIcon(endpoint.type)} />
								</svg>
							</div>
							<div>
								<div class="flex items-center gap-2 flex-wrap">
									<span class="font-semibold text-[var(--ctp-text)]">{endpoint.tag}</span>
									<span class="px-2 py-0.5 text-xs bg-[var(--ctp-surface2)] rounded text-[var(--ctp-subtext1)]">
										{endpoint.type.toUpperCase()}
									</span>
									{#if groups.length > 0}
										<span class="px-2 py-0.5 text-xs rounded flex items-center gap-1" style="background-color: color-mix(in srgb, var(--ctp-blue) 15%, transparent); color: var(--ctp-blue)" title="{$t('outbounds.memberOf')}">
											<svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
												<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6" />
											</svg>
											{groups.join(', ')}
										</span>
									{/if}
								</div>
								{#if endpoint.peers && endpoint.peers.length > 0}
									<div class="text-sm text-[var(--ctp-overlay1)] mt-1">
										{endpoint.peers[0].address}:{endpoint.peers[0].port}
									</div>
								{/if}
								{#if endpoint.address}
									<div class="text-sm text-[var(--ctp-overlay0)] mt-1">
										{endpoint.address.join(', ')}
									</div>
								{/if}
							</div>
						</div>

						<div class="flex items-center gap-2">
							<button
								onclick={() => openEdit(endpoint)}
								class="p-2 hover:bg-[var(--ctp-surface2)] rounded-lg transition-colors"
								title={$t('common.edit')}
							>
								<svg class="w-5 h-5 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
								</svg>
							</button>
							<button
								onclick={() => handleDelete(endpoint.tag)}
								class="action-btn-danger"
								title={$t('common.delete')}
							>
								<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
								</svg>
							</button>
						</div>
					</div>

					{#if endpoint.type === 'awg' || endpoint.type === 'wireguard'}
						{@const awgVersion = getAWGVersion(endpoint)}
						<div class="mt-3 pt-3 border-t border-[var(--ctp-surface2)]">
							<div class="flex items-center gap-2">
								<span class="text-xs text-[var(--ctp-overlay0)]">Obfuscation:</span>
								<span class="status-badge {
									awgVersion === 'AWG 2.0' ? 'success' :
									awgVersion === 'WG' ? 'info' : ''
								}">
									{awgVersion}
								</span>
							</div>
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>

<!-- Modal -->
<Modal open={showModal} title={editingEndpoint ? $t('endpoints.editEndpoint') : $t('endpoints.addEndpoint')} size="lg" onClose={closeModal}>
	<EndpointForm endpoint={editingEndpoint} onSave={handleSave} onCancel={closeModal} />
</Modal>
