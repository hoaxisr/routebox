<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, unsavedChanges } from '$lib/stores';
	import type { RouteRule, Outbound, Endpoint, RuleSet, Inbound } from '$lib/types';
	import { VisualBuilder } from '$lib/components/visual-builder';
	import RuleEditPanel from '$lib/components/visual-builder/panels/RuleEditPanel.svelte';

	let rules = $state<RouteRule[]>([]);
	let outbounds = $state<Outbound[]>([]);
	let endpoints = $state<Endpoint[]>([]);
	let ruleSets = $state<RuleSet[]>([]);
	let inbounds = $state<Inbound[]>([]);
	let loading = $state(true);
	let operating = $state(false); // For save/delete/create operations

	// Combined outbounds: outbounds + endpoints
	let allOutbounds = $derived([
		...outbounds,
		...endpoints.map((e) => ({ tag: e.tag, type: e.type }) as Outbound)
	]);

	// Selected rule index for side panel
	let selectedRuleIndex = $state<number | null>(null);

	// Get selected rule
	let selectedRule = $derived(
		selectedRuleIndex !== null ? rules[selectedRuleIndex] : null
	);

	async function fetchData() {
		try {
			const [rulesData, outboundsData, endpointsData, ruleSetsData, inboundsData] = await Promise.all([
				api.listRules(),
				api.listOutbounds(),
				api.listEndpoints(),
				api.listRuleSets(),
				api.listInbounds()
			]);
			rules = rulesData;
			outbounds = outboundsData;
			endpoints = endpointsData;
			ruleSets = ruleSetsData;
			inbounds = inboundsData;
		} catch (e) {
			notifications.error(`Failed to load: ${e}`);
		} finally {
			loading = false;
		}
	}

	function handleRuleSelect(index: number | null) {
		selectedRuleIndex = index;
	}

	async function handleRuleSave(index: number, updatedRule: RouteRule) {
		operating = true;
		try {
			await api.updateRule(index, updatedRule);
			rules[index] = updatedRule;
			rules = [...rules]; // Trigger reactivity
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
			rules = [...rules]; // Trigger reactivity
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

	function handleKeydown(event: KeyboardEvent) {
		// Escape: close panel
		if (event.key === 'Escape' && selectedRuleIndex !== null) {
			selectedRuleIndex = null;
			return;
		}

		// Delete: delete selected rule (with confirmation)
		if ((event.key === 'Delete' || event.key === 'Backspace') && selectedRuleIndex !== null) {
			// Only if not focused on an input
			if (document.activeElement?.tagName !== 'INPUT' && document.activeElement?.tagName !== 'TEXTAREA') {
				handleRuleDelete(selectedRuleIndex);
			}
		}
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
		{#if !loading && rules.length > 0}
			<button class="btn-primary" onclick={handleCreateRule} disabled={operating}>
				{#if operating}
					<svg class="w-4 h-4 animate-spin inline mr-1" fill="none" viewBox="0 0 24 24">
						<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
						<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
					</svg>
				{/if}
				+ {$t('routes.addRule')}
			</button>
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
{:else if rules.length === 0}
	<div class="empty-state">
		<svg class="w-12 h-12 text-[var(--ctp-overlay1)] mx-auto" fill="none" stroke="currentColor" viewBox="0 0 24 24">
			<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l4.553 2.276A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7" />
		</svg>
		<h3 class="mt-4 text-lg font-medium text-[var(--ctp-text)]">{$t('visualBuilder.noRules')}</h3>
		<p class="mt-2 text-[var(--ctp-subtext1)]">{$t('visualBuilder.noRulesHint')}</p>
		<div class="mt-4 flex gap-3 justify-center">
			<button class="btn-primary" onclick={handleCreateRule} disabled={operating}>
				{#if operating}
					<svg class="w-4 h-4 animate-spin inline mr-1" fill="none" viewBox="0 0 24 24">
						<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
						<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
					</svg>
				{/if}
				+ {$t('routes.addRule')}
			</button>
			<a href="/config/routes" class="btn-secondary">
				{$t('visualBuilder.goToRules')}
			</a>
		</div>
	</div>
{:else}
	<div class="mt-6">
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
					onRuleSelect={handleRuleSelect}
					onRuleOutboundChange={handleRuleOutboundChange}
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

	.builder-main :global(.visual-builder) {
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
