<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, unsavedChanges } from '$lib/stores';
	import type { RouteRule, Outbound, Endpoint, RuleSet, Inbound, RouteSettings, DnsRule, DnsServer, DnsSettings } from '$lib/types';
	import { VisualBuilder, DnsVisualBuilder } from '$lib/components/visual-builder';
	import RuleEditPanel from '$lib/components/visual-builder/panels/RuleEditPanel.svelte';
	import DnsRuleEditPanel from '$lib/components/visual-builder/panels/DnsRuleEditPanel.svelte';

	// Tab state
	type Tab = 'routes' | 'dns';
	let activeTab = $state<Tab>('routes');

	// Routes state
	let rules = $state<RouteRule[]>([]);
	let outbounds = $state<Outbound[]>([]);
	let endpoints = $state<Endpoint[]>([]);
	let ruleSets = $state<RuleSet[]>([]);
	let inbounds = $state<Inbound[]>([]);
	let routeSettings = $state<RouteSettings | null>(null);

	// DNS state
	let dnsRules = $state<DnsRule[]>([]);
	let dnsServers = $state<DnsServer[]>([]);
	let dnsSettings = $state<DnsSettings | null>(null);

	let loading = $state(true);
	let operating = $state(false);

	// Derived values
	let finalOutbound = $derived(routeSettings?.final);
	let finalDnsServer = $derived(dnsSettings?.final);

	let allOutbounds = $derived([
		...outbounds,
		...endpoints.map((e) => ({ tag: e.tag, type: e.type }) as Outbound)
	]);

	// Selected rule index for side panels
	let selectedRuleIndex = $state<number | null>(null);
	let selectedDnsRuleIndex = $state<number | null>(null);

	let selectedRule = $derived(
		selectedRuleIndex !== null ? rules[selectedRuleIndex] : null
	);
	let selectedDnsRule = $derived(
		selectedDnsRuleIndex !== null ? dnsRules[selectedDnsRuleIndex] : null
	);

	async function fetchData() {
		try {
			const [
				rulesData, outboundsData, endpointsData, ruleSetsData, inboundsData, routeSettingsData,
				dnsRulesData, dnsServersData, dnsSettingsData
			] = await Promise.all([
				api.listRules(),
				api.listOutbounds(),
				api.listEndpoints(),
				api.listRuleSets(),
				api.listInbounds(),
				api.getRouteSettings(),
				api.listDnsRules(),
				api.listDnsServers(),
				api.getDnsSettings()
			]);
			rules = rulesData;
			outbounds = outboundsData;
			endpoints = endpointsData;
			ruleSets = ruleSetsData;
			inbounds = inboundsData;
			routeSettings = routeSettingsData;
			dnsRules = dnsRulesData;
			dnsServers = dnsServersData;
			dnsSettings = dnsSettingsData;
		} catch (e) {
			notifications.error(`Failed to load: ${e}`);
		} finally {
			loading = false;
		}
	}

	// === Routes handlers ===
	function handleRuleSelect(index: number | null) {
		selectedRuleIndex = index;
	}

	async function handleRuleSave(index: number, updatedRule: RouteRule) {
		operating = true;
		try {
			await api.updateRule(index, updatedRule);
			rules[index] = updatedRule;
			rules = [...rules];
			unsavedChanges.refresh();
			notifications.success($t('routes.ruleUpdated'));
		} catch (e) {
			notifications.error(`Failed to update rule: ${e}`);
		} finally {
			operating = false;
		}
	}

	async function handleRuleDelete(index: number) {
		operating = true;
		try {
			await api.deleteRule(index);
			rules.splice(index, 1);
			rules = [...rules];
			selectedRuleIndex = null;
			unsavedChanges.refresh();
			notifications.success($t('routes.ruleDeleted'));
		} catch (e) {
			notifications.error(`Failed to delete rule: ${e}`);
		} finally {
			operating = false;
		}
	}

	async function handleCreateRule() {
		operating = true;
		const newRule: RouteRule = {
			action: 'route',
			outbound: allOutbounds[0]?.tag || 'direct'
		};
		try {
			await api.createRule(newRule);
			rules = [...rules, newRule];
			selectedRuleIndex = rules.length - 1;
			unsavedChanges.refresh();
			notifications.success($t('routes.ruleCreated'));
		} catch (e) {
			notifications.error(`Failed to create rule: ${e}`);
		} finally {
			operating = false;
		}
	}

	function handleClosePanel() {
		selectedRuleIndex = null;
	}

	async function handleRuleOutboundChange(index: number, newOutbound: string) {
		const rule = rules[index];
		if (!rule || rule.outbound === newOutbound) return;

		operating = true;
		const updatedRule: RouteRule = { ...rule, outbound: newOutbound };
		try {
			await api.updateRule(index, updatedRule);
			rules[index] = updatedRule;
			rules = [...rules];
			unsavedChanges.refresh();
			notifications.success($t('routes.ruleUpdated'));
		} catch (e) {
			notifications.error(`Failed to update rule: ${e}`);
		} finally {
			operating = false;
		}
	}

	async function handleRuleMove(fromIndex: number, toIndex: number) {
		if (fromIndex === toIndex) return;

		operating = true;
		try {
			await api.reorderRules(fromIndex, toIndex);
			const [movedRule] = rules.splice(fromIndex, 1);
			rules.splice(toIndex, 0, movedRule);
			rules = [...rules];

			if (selectedRuleIndex === fromIndex) {
				selectedRuleIndex = toIndex;
			} else if (selectedRuleIndex !== null) {
				if (fromIndex < toIndex && selectedRuleIndex > fromIndex && selectedRuleIndex <= toIndex) {
					selectedRuleIndex--;
				} else if (fromIndex > toIndex && selectedRuleIndex >= toIndex && selectedRuleIndex < fromIndex) {
					selectedRuleIndex++;
				}
			}
			unsavedChanges.refresh();
		} catch (e) {
			notifications.error(`Failed to reorder rule: ${e}`);
		} finally {
			operating = false;
		}
	}

	// === DNS handlers ===
	function handleDnsRuleSelect(index: number | null) {
		selectedDnsRuleIndex = index;
	}

	async function handleDnsRuleSave(index: number, updatedRule: DnsRule) {
		operating = true;
		try {
			await api.updateDnsRule(index, updatedRule);
			dnsRules[index] = updatedRule;
			dnsRules = [...dnsRules];
			unsavedChanges.refresh();
			notifications.success($t('dns.ruleUpdated'));
		} catch (e) {
			notifications.error(`Failed to update DNS rule: ${e}`);
		} finally {
			operating = false;
		}
	}

	async function handleDnsRuleDelete(index: number) {
		operating = true;
		try {
			await api.deleteDnsRule(index);
			dnsRules.splice(index, 1);
			dnsRules = [...dnsRules];
			selectedDnsRuleIndex = null;
			unsavedChanges.refresh();
			notifications.success($t('dns.ruleDeleted'));
		} catch (e) {
			notifications.error(`Failed to delete DNS rule: ${e}`);
		} finally {
			operating = false;
		}
	}

	async function handleCreateDnsRule() {
		operating = true;
		const newRule: DnsRule = {
			action: 'route',
			server: dnsServers[0]?.tag || ''
		};
		try {
			await api.createDnsRule(newRule);
			dnsRules = [...dnsRules, newRule];
			selectedDnsRuleIndex = dnsRules.length - 1;
			unsavedChanges.refresh();
			notifications.success($t('dns.ruleCreated'));
		} catch (e) {
			notifications.error(`Failed to create DNS rule: ${e}`);
		} finally {
			operating = false;
		}
	}

	function handleCloseDnsPanel() {
		selectedDnsRuleIndex = null;
	}

	async function handleDnsRuleServerChange(index: number, newServer: string) {
		const rule = dnsRules[index];
		if (!rule || rule.server === newServer) return;

		operating = true;
		const updatedRule: DnsRule = { ...rule, server: newServer };
		try {
			await api.updateDnsRule(index, updatedRule);
			dnsRules[index] = updatedRule;
			dnsRules = [...dnsRules];
			unsavedChanges.refresh();
			notifications.success($t('dns.ruleUpdated'));
		} catch (e) {
			notifications.error(`Failed to update DNS rule: ${e}`);
		} finally {
			operating = false;
		}
	}

	async function handleDnsRuleMove(fromIndex: number, toIndex: number) {
		if (fromIndex === toIndex) return;

		operating = true;
		try {
			await api.reorderDnsRules(fromIndex, toIndex);
			const [movedRule] = dnsRules.splice(fromIndex, 1);
			dnsRules.splice(toIndex, 0, movedRule);
			dnsRules = [...dnsRules];

			if (selectedDnsRuleIndex === fromIndex) {
				selectedDnsRuleIndex = toIndex;
			} else if (selectedDnsRuleIndex !== null) {
				if (fromIndex < toIndex && selectedDnsRuleIndex > fromIndex && selectedDnsRuleIndex <= toIndex) {
					selectedDnsRuleIndex--;
				} else if (fromIndex > toIndex && selectedDnsRuleIndex >= toIndex && selectedDnsRuleIndex < fromIndex) {
					selectedDnsRuleIndex++;
				}
			}
			unsavedChanges.refresh();
		} catch (e) {
			notifications.error(`Failed to reorder DNS rule: ${e}`);
		} finally {
			operating = false;
		}
	}

	// Keyboard handlers
	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') {
			if (activeTab === 'routes' && selectedRuleIndex !== null) {
				selectedRuleIndex = null;
			} else if (activeTab === 'dns' && selectedDnsRuleIndex !== null) {
				selectedDnsRuleIndex = null;
			}
			return;
		}

		if ((event.key === 'Delete' || event.key === 'Backspace')) {
			if (document.activeElement?.tagName !== 'INPUT' && document.activeElement?.tagName !== 'TEXTAREA') {
				if (activeTab === 'routes' && selectedRuleIndex !== null) {
					handleRuleDelete(selectedRuleIndex);
				} else if (activeTab === 'dns' && selectedDnsRuleIndex !== null) {
					handleDnsRuleDelete(selectedDnsRuleIndex);
				}
			}
		}
	}

	// Clear selection when switching tabs
	function switchTab(tab: Tab) {
		activeTab = tab;
		selectedRuleIndex = null;
		selectedDnsRuleIndex = null;
	}

	onMount(() => {
		fetchData();
	});
</script>

<svelte:window onkeydown={handleKeydown} />

<svelte:head>
	<title>{$t('visualBuilder.title')} - RouteBox</title>
</svelte:head>

<div class="page-header">
	<div class="flex items-center justify-between">
		<div class="flex items-center gap-3">
			<h1 class="text-2xl font-semibold text-[var(--ctp-text)]">{$t('visualBuilder.title')}</h1>
			<span class="status-badge info">{$t('visualBuilder.beta')}</span>
		</div>
		{#if !loading}
			{#if activeTab === 'routes' && rules.length > 0}
				<button class="btn-primary" onclick={handleCreateRule} disabled={operating}>
					+ {$t('routes.addRule')}
				</button>
			{:else if activeTab === 'dns' && dnsRules.length > 0}
				<button class="btn-primary btn-dns" onclick={handleCreateDnsRule} disabled={operating}>
					+ {$t('dns.addRule')}
				</button>
			{/if}
		{/if}
	</div>
	<p class="text-[var(--ctp-subtext1)] mt-1">{$t('visualBuilder.description')}</p>
</div>

{#if loading}
	<div class="flex items-center justify-center py-20">
		<svg class="w-8 h-8 animate-spin text-[var(--ctp-primary)]" fill="none" viewBox="0 0 24 24">
			<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
			<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
		</svg>
	</div>
{:else}
	<div class="mt-4">
		<!-- Tabs -->
		<div class="tabs-header">
			<button
				class="tab-btn"
				class:active={activeTab === 'routes'}
				onclick={() => switchTab('routes')}
			>
				<span class="tab-label">{$t('nav.routes')}</span>
				<span class="tab-count">{rules.length}</span>
			</button>
			<button
				class="tab-btn tab-dns"
				class:active={activeTab === 'dns'}
				onclick={() => switchTab('dns')}
			>
				<span class="tab-label">DNS</span>
				<span class="tab-count">{dnsRules.length}</span>
			</button>
		</div>

		<!-- Routes Tab -->
		{#if activeTab === 'routes'}
			{#if rules.length === 0}
				<div class="empty-state">
					<svg class="w-12 h-12 text-[var(--ctp-overlay1)] mx-auto" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l4.553 2.276A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7" />
					</svg>
					<h3 class="mt-4 text-lg font-medium text-[var(--ctp-text)]">{$t('visualBuilder.noRules')}</h3>
					<p class="mt-2 text-[var(--ctp-subtext1)]">{$t('visualBuilder.noRulesHint')}</p>
					<div class="mt-4 flex gap-3 justify-center">
						<button class="btn-primary" onclick={handleCreateRule} disabled={operating}>
							+ {$t('routes.addRule')}
						</button>
						<a href="/config/routes" class="btn-secondary">
							{$t('visualBuilder.goToRules')}
						</a>
					</div>
				</div>
			{:else}
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

				<div class="builder-container" class:with-panel={selectedRule !== null}>
					<div class="builder-main">
						<VisualBuilder
							{rules}
							outbounds={allOutbounds}
							{finalOutbound}
							onRuleSelect={handleRuleSelect}
							onRuleOutboundChange={handleRuleOutboundChange}
							onRuleMove={handleRuleMove}
							onRuleCreate={handleCreateRule}
							onRuleDelete={handleRuleDelete}
						/>
					</div>

					{#if selectedRule !== null && selectedRuleIndex !== null}
						<div class="builder-panel">
							<RuleEditPanel
								rule={selectedRule}
								ruleIndex={selectedRuleIndex}
								outbounds={allOutbounds}
								{ruleSets}
								{inbounds}
								{operating}
								onSave={handleRuleSave}
								onDelete={handleRuleDelete}
								onClose={handleClosePanel}
							/>
						</div>
					{/if}
				</div>
			{/if}
		{/if}

		<!-- DNS Tab -->
		{#if activeTab === 'dns'}
			{#if dnsRules.length === 0 && dnsServers.length === 0}
				<div class="empty-state">
					<svg class="w-12 h-12 text-[var(--ctp-overlay1)] mx-auto" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9m-9 9a9 9 0 019-9" />
					</svg>
					<h3 class="mt-4 text-lg font-medium text-[var(--ctp-text)]">{$t('dns.noRules')}</h3>
					<p class="mt-2 text-[var(--ctp-subtext1)]">{$t('dns.noRulesHint')}</p>
					<div class="mt-4 flex gap-3 justify-center">
						<button class="btn-primary btn-dns" onclick={handleCreateDnsRule} disabled={operating || dnsServers.length === 0}>
							+ {$t('dns.addRule')}
						</button>
						<a href="/config/dns" class="btn-secondary">
							{$t('dns.goToConfig')}
						</a>
					</div>
				</div>
			{:else}
				<div class="stats-bar">
					<div class="stat">
						<span class="stat-value">{dnsRules.length}</span>
						<span class="stat-label">{$t('dns.rules')}</span>
					</div>
					<div class="stat">
						<span class="stat-value">{dnsServers.length}</span>
						<span class="stat-label">{$t('dns.servers')}</span>
					</div>
				</div>

				<div class="builder-container" class:with-panel={selectedDnsRule !== null}>
					<div class="builder-main">
						<DnsVisualBuilder
							rules={dnsRules}
							servers={dnsServers}
							finalServer={finalDnsServer}
							onRuleSelect={handleDnsRuleSelect}
							onRuleServerChange={handleDnsRuleServerChange}
							onRuleMove={handleDnsRuleMove}
							onRuleCreate={handleCreateDnsRule}
							onRuleDelete={handleDnsRuleDelete}
						/>
					</div>

					{#if selectedDnsRule !== null && selectedDnsRuleIndex !== null}
						<div class="builder-panel">
							<DnsRuleEditPanel
								rule={selectedDnsRule}
								ruleIndex={selectedDnsRuleIndex}
								{dnsServers}
								{ruleSets}
								outbounds={allOutbounds}
								{operating}
								onSave={handleDnsRuleSave}
								onDelete={handleDnsRuleDelete}
								onClose={handleCloseDnsPanel}
							/>
						</div>
					{/if}
				</div>
			{/if}
		{/if}

		<div class="help-text mt-4">
			<p>{$t('visualBuilder.helpText')}</p>
			<p class="shortcuts-hint">{$t('visualBuilder.shortcuts')}</p>
		</div>
	</div>
{/if}

<style>
	.page-header {
		margin-bottom: 1.5rem;
	}

	.tabs-header {
		display: flex;
		gap: 0.5rem;
		margin-bottom: 1rem;
	}

	.tab-btn {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.625rem 1rem;
		background: var(--ctp-surface0);
		border: 1px solid var(--ctp-surface1);
		border-radius: 0.5rem;
		color: var(--ctp-subtext1);
		font-weight: 500;
		transition: all 0.15s ease;
	}

	.tab-btn:hover {
		background: var(--ctp-surface1);
		color: var(--ctp-text);
	}

	.tab-btn.active {
		background: var(--ctp-primary);
		border-color: var(--ctp-primary);
		color: white;
	}

	.tab-btn.tab-dns.active {
		background: var(--ctp-sapphire);
		border-color: var(--ctp-sapphire);
	}

	.tab-count {
		padding: 0.125rem 0.5rem;
		background: rgba(255, 255, 255, 0.2);
		border-radius: 0.25rem;
		font-size: 0.75rem;
	}

	.tab-btn:not(.active) .tab-count {
		background: var(--ctp-surface1);
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

	.builder-container {
		display: flex;
		gap: 0;
		border: 1px solid var(--ctp-surface0);
		border-radius: 0.5rem;
		overflow: hidden;
		height: 600px;
	}

	.builder-main {
		flex: 1;
		min-width: 0;
	}

	.builder-main :global(.visual-builder),
	.builder-main :global(.dns-visual-builder) {
		border: none;
		border-radius: 0;
		height: 100%;
	}

	.builder-panel {
		width: 380px;
		flex-shrink: 0;
		border-left: 1px solid var(--ctp-surface0);
	}

	.help-text {
		color: var(--ctp-overlay1);
		font-size: 0.875rem;
		text-align: center;
	}

	.help-text .shortcuts-hint {
		margin-top: 0.25rem;
		font-size: 0.75rem;
		opacity: 0.7;
	}

	.btn-primary {
		padding: 0.5rem 1rem;
		background: var(--ctp-primary);
		color: white;
		border-radius: 0.375rem;
		font-weight: 500;
		text-decoration: none;
		transition: filter 0.15s ease;
	}

	.btn-primary:hover:not(:disabled) {
		filter: brightness(1.1);
	}

	.btn-primary:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.btn-primary.btn-dns {
		background: var(--ctp-sapphire);
	}

	.btn-secondary {
		padding: 0.5rem 1rem;
		background: var(--ctp-surface0);
		color: var(--ctp-text);
		border-radius: 0.375rem;
		font-weight: 500;
		text-decoration: none;
		transition: background 0.15s ease;
	}

	.btn-secondary:hover {
		background: var(--ctp-surface1);
	}
</style>
