<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, unsavedChanges } from '$lib/stores';
	import type { Outbound, Endpoint, RouteRule, DnsServer, RouteSettings, ClashProxy, Subscription } from '$lib/types';
	import { owningSubscription } from '$lib/utils/subscriptionTags';
	import Modal from '$lib/components/shared/Modal.svelte';
	import OutboundForm from '$lib/components/config/OutboundForm.svelte';

	let outbounds = $state<Outbound[]>([]);
	let endpoints = $state<Endpoint[]>([]);
	let rules = $state<RouteRule[]>([]);
	let dnsServers = $state<DnsServer[]>([]);
	let routeSettings = $state<RouteSettings | null>(null);
	let loading = $state(true);

	// Check if default_domain_resolver is configured
	let hasDefaultResolver = $derived(!!routeSettings?.default_domain_resolver);

	// Modal state
	let showModal = $state(false);
	let editingOutbound = $state<Outbound | undefined>(undefined);
	let saving = $state(false);

	// Service outbound types (groups/selectors)
	const serviceTypes = ['selector', 'urltest'];

	// Split outbounds into service and real
	let serviceOutbounds = $derived(outbounds.filter(o => serviceTypes.includes(o.type)));
	let realOutbounds = $derived(outbounds.filter(o => !serviceTypes.includes(o.type)));

	let clashProxies = $state<Record<string, ClashProxy>>({});

	// Subscriptions own their outbounds by tag and rewrite them wholesale on every
	// refresh, so deleting one here lasts only until the next one. The list said
	// nothing about that, which made the deletion look permanent (#80). A failed
	// load leaves the list unmarked rather than blocking the page.
	let subs = $state<Subscription[]>([]);

	async function fetchProxyDelays() {
		try {
			const data = await api.getProxies();
			clashProxies = data.proxies || {};
		} catch {
			// Clash API may not be available if sing-box is stopped
		}
	}

	async function testOutboundLatencies() {
		const tags = outbounds.filter(o => !['direct', 'block', 'dns'].includes(o.type)).map(o => o.tag);
		await Promise.allSettled(tags.map(tag => api.testLatency(tag)));
		await fetchProxyDelays();
	}

	function getStatusColor(tag: string): { bg: string; stroke: string } {
		const proxy = clashProxies[tag];
		let delay: number | null = null;
		if (proxy) {
			if ((proxy.type === 'Selector' || proxy.type === 'URLTest') && proxy.now) {
				const activeProxy = clashProxies[proxy.now];
				const lastHistory = activeProxy?.history?.[activeProxy.history.length - 1];
				delay = lastHistory?.delay ?? null;
			} else {
				const lastHistory = proxy.history?.[proxy.history.length - 1];
				delay = lastHistory?.delay ?? null;
			}
		}
		if (delay === 0) {
			return { bg: 'color-mix(in srgb, var(--ctp-red) 15%, transparent)', stroke: 'var(--ctp-red)' };
		}
		return { bg: 'color-mix(in srgb, var(--ctp-green) 15%, transparent)', stroke: 'var(--ctp-green)' };
	}

	// Count route rules usage for each outbound
	let routeUsage = $derived.by(() => {
		const usage = new Map<string, number>();
		for (const rule of rules) {
			if (rule.outbound) {
				usage.set(rule.outbound, (usage.get(rule.outbound) || 0) + 1);
			}
		}
		return usage;
	});

	// Find which service outbounds (selector/urltest) contain each real outbound
	let containedIn = $derived.by(() => {
		const map = new Map<string, string[]>();
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

	async function fetchData() {
		try {
			const [ob, ep, rl, dns, rs, sb] = await Promise.all([
				api.listOutbounds(),
				api.listEndpoints(),
				api.listRules(),
				api.listDnsServers(),
				api.getRouteSettings(),
				api.getSubscriptions().catch(() => [] as Subscription[])
			]);
			subs = sb;
			outbounds = ob;
			endpoints = ep;
			rules = rl;
			dnsServers = dns;
			routeSettings = rs;
		} catch (e) {
			notifications.error($t('errors.loadFailed') + `: ${e}`);
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
				notifications.success($t('outbounds.outboundUpdated'));
				unsavedChanges.markChanged($t('outbounds.title'), `${$t('common.update')} "${outbound.tag}"`);
			} else {
				await api.createOutbound(outbound);
				notifications.success($t('outbounds.outboundCreated'));
				unsavedChanges.markChanged($t('outbounds.title'), `${$t('common.create')} "${outbound.tag}"`);
			}
			closeModal();
			await fetchData();
		} catch (e) {
			notifications.error($t('errors.saveFailed') + `: ${e}`);
		} finally {
			saving = false;
		}
	}

	async function handleDelete(tag: string) {
		const owner = owningSubscription(tag, subs);
		const question = owner
			? $t('outbounds.confirmDeleteSubscribed', { values: { name: tag, subscription: owner.name } })
			: $t('outbounds.confirmDelete', { values: { name: tag } });
		if (!confirm(question)) return;

		try {
			await api.deleteOutbound(tag);
			notifications.success($t('outbounds.outboundDeleted'));
			unsavedChanges.markChanged($t('outbounds.title'), `${$t('common.delete')} "${tag}"`);
			await fetchData();
		} catch (e) {
			notifications.error($t('errors.deleteFailed') + `: ${e}`);
		}
	}

	function getTypeColor(type: string) {
		switch (type) {
			case 'selector':
				return 'var(--ctp-blue)';
			case 'urltest':
				return 'var(--ctp-teal)';
			case 'direct':
				return 'var(--ctp-green)';
			case 'block':
				return 'var(--ctp-red)';
			case 'dns':
				return 'var(--ctp-yellow)';
			default:
				return 'var(--ctp-primary)';
		}
	}

	function getTypeIcon(type: string) {
		switch (type) {
			case 'selector':
				return 'M4 6h16M4 12h16m-7 6h7'; // menu/list
			case 'urltest':
				return 'M13 10V3L4 14h7v7l9-11h-7z'; // lightning
			case 'direct':
				return 'M5 12h14'; // arrow right
			case 'block':
				return 'M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636'; // ban
			default:
				return 'M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9m-9 9a9 9 0 019-9'; // globe
		}
	}

	onMount(async () => {
		await fetchData();
		fetchProxyDelays();
		testOutboundLatencies();
	});
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-[var(--ctp-text)]">{$t('outbounds.title')}</h1>
		<div class="flex items-center gap-2">
			<button
				onclick={openCreate}
				class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
			>
				+ {$t('outbounds.addOutbound')}
			</button>
		</div>
	</div>

	{#if loading}
		<div class="text-[var(--ctp-overlay0)]">{$t('common.loading')}</div>
	{:else if outbounds.length === 0}
		<div class="bg-[var(--ctp-surface0)] rounded-xl p-8 text-center">
			<div class="text-[var(--ctp-overlay1)] mb-4">{$t('outbounds.noOutbounds')}</div>
			<button
				onclick={openCreate}
				class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
			>
				{$t('outbounds.addOutbound')}
			</button>
		</div>
	{:else}
		<!-- Service Outbounds (Selector, URLTest) -->
		{#if serviceOutbounds.length > 0}
			<section>
				<div class="flex items-center gap-2 mb-3">
					<svg class="w-5 h-5 text-[var(--ctp-blue)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
					</svg>
					<h2 class="text-lg font-semibold text-[var(--ctp-subtext1)]">{$t('outbounds.serviceOutbounds')}</h2>
					<span class="text-sm text-[var(--ctp-overlay0)]">({serviceOutbounds.length})</span>
				</div>
				<div class="space-y-2">
					{#each serviceOutbounds as outbound}
						{@const rulesCount = routeUsage.get(outbound.tag) || 0}
						{@const subOwner = owningSubscription(outbound.tag, subs)}
						{@const statusColor = getStatusColor(outbound.tag)}
						<div class="bg-[var(--ctp-surface0)] rounded-xl p-4 hover:bg-[var(--ctp-surface1)] transition-colors border-l-4" style="border-color: {getTypeColor(outbound.type)}">
							<div class="flex items-center justify-between">
								<div class="flex items-center gap-4">
									<div class="w-10 h-10 rounded-lg flex items-center justify-center" style="background-color: {statusColor.bg}">
										<svg class="w-5 h-5" fill="none" stroke={statusColor.stroke} viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={getTypeIcon(outbound.type)} />
										</svg>
									</div>
									<div>
										<div class="flex items-center gap-2 flex-wrap">
											<span class="font-semibold text-[var(--ctp-text)]">{outbound.tag}</span>
											<span class="px-2 py-0.5 text-xs rounded" style="background-color: color-mix(in srgb, {getTypeColor(outbound.type)} 20%, transparent); color: {getTypeColor(outbound.type)}">
												{outbound.type}
											</span>
											{#if subOwner}
												<span class="px-2 py-0.5 text-xs rounded" style="background-color: color-mix(in srgb, var(--ctp-primary) 15%, transparent); color: var(--ctp-primary)" title={$t('outbounds.fromSubscriptionHint')}>
													{$t('outbounds.fromSubscription', { values: { name: subOwner.name } })}
												</span>
											{/if}
											{#if rulesCount > 0}
												<span class="px-2 py-0.5 text-xs bg-[var(--ctp-surface2)] rounded text-[var(--ctp-subtext1)]" title="{$t('outbounds.usedByRules')}">
													{rulesCount} {rulesCount === 1 ? $t('outbounds.rule') : $t('outbounds.rules')}
												</span>
											{/if}
										</div>
										{#if outbound.outbounds && outbound.outbounds.length > 0}
											<div class="text-sm text-[var(--ctp-overlay1)] mt-1 flex items-center gap-1 flex-wrap">
												<span class="text-[var(--ctp-overlay0)]">{$t('outbounds.members')}:</span>
												{#each outbound.outbounds as member, i}
													<span class="px-1.5 py-0.5 text-xs bg-[var(--ctp-surface2)] rounded">{member}</span>
												{/each}
											</div>
										{/if}
									</div>
								</div>

								<div class="flex items-center gap-2">
									<button
										onclick={() => openEdit(outbound)}
										class="p-2 hover:bg-[var(--ctp-surface2)] rounded-lg transition-colors"
										title={$t('common.edit')}
									>
										<svg class="w-5 h-5 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
										</svg>
									</button>
									<button
										onclick={() => handleDelete(outbound.tag)}
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
			</section>
		{/if}

		<!-- Real Outbounds (Direct, VLess, etc.) -->
		{#if realOutbounds.length > 0}
			<section>
				<div class="flex items-center gap-2 mb-3">
					<svg class="w-5 h-5 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9m-9 9a9 9 0 019-9" />
					</svg>
					<h2 class="text-lg font-semibold text-[var(--ctp-subtext1)]">{$t('outbounds.realOutbounds')}</h2>
					<span class="text-sm text-[var(--ctp-overlay0)]">({realOutbounds.length})</span>
				</div>
				<div class="space-y-2">
					{#each realOutbounds as outbound}
						{@const rulesCount = routeUsage.get(outbound.tag) || 0}
						{@const groups = containedIn.get(outbound.tag) || []}
						{@const subOwner = owningSubscription(outbound.tag, subs)}
						{@const statusColor = getStatusColor(outbound.tag)}
						<div class="bg-[var(--ctp-surface0)] rounded-xl p-4 hover:bg-[var(--ctp-surface1)] transition-colors border-l-4" style="border-color: {getTypeColor(outbound.type)}">
							<div class="flex items-center justify-between">
								<div class="flex items-center gap-4">
									<div class="w-10 h-10 rounded-lg flex items-center justify-center" style="background-color: {statusColor.bg}">
										<svg class="w-5 h-5" fill="none" stroke={statusColor.stroke} viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={getTypeIcon(outbound.type)} />
										</svg>
									</div>
									<div>
										<div class="flex items-center gap-2 flex-wrap">
											<span class="font-semibold text-[var(--ctp-text)]">{outbound.tag}</span>
											<span class="px-2 py-0.5 text-xs rounded" style="background-color: color-mix(in srgb, {getTypeColor(outbound.type)} 20%, transparent); color: {getTypeColor(outbound.type)}">
												{outbound.type}
											</span>
											{#if subOwner}
												<span class="px-2 py-0.5 text-xs rounded" style="background-color: color-mix(in srgb, var(--ctp-primary) 15%, transparent); color: var(--ctp-primary)" title={$t('outbounds.fromSubscriptionHint')}>
													{$t('outbounds.fromSubscription', { values: { name: subOwner.name } })}
												</span>
											{/if}
											{#if rulesCount > 0}
												<span class="px-2 py-0.5 text-xs bg-[var(--ctp-surface2)] rounded text-[var(--ctp-subtext1)]" title="{$t('outbounds.usedByRules')}">
													{rulesCount} {rulesCount === 1 ? $t('outbounds.rule') : $t('outbounds.rules')}
												</span>
											{/if}
											{#if groups.length > 0}
												<span class="px-2 py-0.5 text-xs rounded flex items-center gap-1" style="background-color: color-mix(in srgb, var(--ctp-blue) 15%, transparent); color: var(--ctp-blue)" title="{$t('outbounds.memberOf')}">
													<svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
														<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6" />
													</svg>
													{groups.join(', ')}
												</span>
											{/if}
										</div>
										{#if outbound.server}
											<div class="text-sm text-[var(--ctp-overlay0)] mt-1">
												{outbound.server}{outbound.server_port ? `:${outbound.server_port}` : ''}
											</div>
										{/if}
									</div>
								</div>

								<div class="flex items-center gap-2">
									<button
										onclick={() => openEdit(outbound)}
										class="p-2 hover:bg-[var(--ctp-surface2)] rounded-lg transition-colors"
										title={$t('common.edit')}
									>
										<svg class="w-5 h-5 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
										</svg>
									</button>
									<button
										onclick={() => handleDelete(outbound.tag)}
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
			</section>
		{/if}
	{/if}
</div>

<!-- Modal -->
<Modal open={showModal} title={editingOutbound ? $t('outbounds.editOutbound') : $t('outbounds.addOutbound')} size="lg" onClose={closeModal}>
	<OutboundForm outbound={editingOutbound} {endpoints} {outbounds} {dnsServers} {hasDefaultResolver} onSave={handleSave} onCancel={closeModal} />
</Modal>
