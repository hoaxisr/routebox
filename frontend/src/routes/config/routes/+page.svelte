<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, unsavedChanges } from '$lib/stores';
	import type { RouteRule, RuleSet, Outbound, Inbound, RouteSettings, Endpoint } from '$lib/types';
	import RuleSetForm from '$lib/components/config/RuleSetForm.svelte';
	import RuleForm from '$lib/components/config/RuleForm.svelte';
	import RuleTemplates from '$lib/components/config/RuleTemplates.svelte';
	import RuleWizard from '$lib/components/config/RuleWizard.svelte';
	import RouteInspector from '$lib/components/config/RouteInspector.svelte';
	import DraggableRuleList from '$lib/components/config/DraggableRuleList.svelte';

	type Tab = 'rules' | 'inspector';
	let activeTab = $state<Tab>('rules');

	let rules = $state<RouteRule[]>([]);
	let ruleSets = $state<RuleSet[]>([]);
	let outbounds = $state<Outbound[]>([]);
	let endpoints = $state<Endpoint[]>([]);
	let inbounds = $state<Inbound[]>([]);
	let settings = $state<RouteSettings>({ final: 'direct' });

	// Combined list: outbounds + endpoints (endpoints can be used directly as routing targets)
	let allOutbounds = $derived([
		...outbounds,
		...endpoints.map(e => ({ tag: e.tag, type: e.type } as Outbound))
	]);
	let loading = $state(true);
	let hasChanges = $state(false);
	let applying = $state(false);

	// Collapsed sections state
	let ruleSetsExpanded = $state(false);
	let rulesExpanded = $state(false);

	// Modal states
	let showRuleSetForm = $state(false);
	let showRuleForm = $state(false);
	let showTemplates = $state(false);
	let showWizard = $state(false);
	let showAddMenu = $state(false);
	let showCreateMenu = $state(false);
	let editingRuleIndex = $state<number | null>(null);
	let editingRuleSet = $state<RuleSet | null>(null);
	let viewingRuleSet = $state<RuleSet | null>(null);

	async function fetchData() {
		try {
			const [rulesData, ruleSetsData, outboundsData, endpointsData, inboundsData, settingsData] = await Promise.all([
				api.listRules(),
				api.listRuleSets(),
				api.listOutbounds(),
				api.listEndpoints(),
				api.listInbounds(),
				api.getRouteSettings()
			]);
			rules = rulesData;
			ruleSets = ruleSetsData;
			outbounds = outboundsData;
			endpoints = endpointsData;
			inbounds = inboundsData;
			settings = settingsData;
		} catch (e) {
			notifications.error(`Failed to load: ${e}`);
		} finally {
			loading = false;
		}
	}

	async function applyChanges() {
		applying = true;
		try {
			await api.applyConfig();
			notifications.success('Changes applied successfully');
			hasChanges = false;
			unsavedChanges.clearChanges();
		} catch (e) {
			notifications.error(`Failed to apply: ${e}`);
		} finally {
			applying = false;
		}
	}

	// Derived: find outbound for each rule-set (simple rule_set -> outbound rules)
	let ruleSetOutboundMap = $derived.by(() => {
		const map = new Map<string, { outbound: string; action: string; ruleIndex: number }>();
		rules.forEach((rule, index) => {
			// Check if this is a simple rule_set -> outbound rule
			const ruleSetRefs = rule.rule_set;
			if (ruleSetRefs && ruleSetRefs.length === 1) {
				// Check if this rule ONLY has rule_set condition (no other conditions)
				const hasOtherConditions = rule.domain || rule.domain_suffix || rule.domain_keyword ||
					rule.domain_regex || rule.ip_cidr || rule.protocol || rule.port ||
					rule.source_ip_cidr || rule.source_port || rule.network || rule.inbound;
				if (!hasOtherConditions) {
					const action = rule.action || 'route';
					const outbound = action === 'reject' ? 'REJECT' : (rule.outbound || settings.final);
					map.set(ruleSetRefs[0], { outbound, action, ruleIndex: index });
				}
			}
		});
		return map;
	});

	// Derived: rules that are NOT simple rule_set mappings (shown in Rules section)
	let filteredRules = $derived.by(() => {
		return rules.map((rule, index) => ({ rule, originalIndex: index })).filter(({ rule }) => {
			const ruleSetRefs = rule.rule_set;
			if (ruleSetRefs && ruleSetRefs.length === 1) {
				const hasOtherConditions = rule.domain || rule.domain_suffix || rule.domain_keyword ||
					rule.domain_regex || rule.ip_cidr || rule.protocol || rule.port ||
					rule.source_ip_cidr || rule.source_port || rule.network || rule.inbound;
				if (!hasOtherConditions) {
					return false; // This is a simple rule_set mapping, hide it
				}
			}
			return true;
		});
	});

	// Derived: rule-sets that already have rules (for hiding in advanced form)
	let usedRuleSets = $derived(new Set(ruleSetOutboundMap.keys()));

	// Rule Set handlers
	async function handleCreateRuleSet(ruleSet: RuleSet, routeConfig?: { outbound?: string; action?: string }) {
		try {
			await api.createRuleSet(ruleSet);
			ruleSets = [...ruleSets, ruleSet];

			// If route config provided, also create a rule
			if (routeConfig && (routeConfig.outbound || routeConfig.action === 'reject')) {
				const rule: RouteRule = {
					rule_set: [ruleSet.tag]
				};
				if (routeConfig.action === 'reject') {
					rule.action = 'reject';
				} else if (routeConfig.outbound) {
					rule.outbound = routeConfig.outbound;
				}
				await api.createRule(rule);
				rules = [...rules, rule];
				unsavedChanges.markChanged('Routes', `Created rule set "${ruleSet.tag}" with route to ${routeConfig.action === 'reject' ? 'REJECT' : routeConfig.outbound}`);
			} else {
				unsavedChanges.markChanged('Routes', `Created rule set "${ruleSet.tag}"`);
			}

			showRuleSetForm = false;
			editingRuleSet = null;
			hasChanges = true;
			notifications.success(`Rule set '${ruleSet.tag}' created`);
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	async function handleUpdateRuleSet(ruleSet: RuleSet, routeConfig?: { outbound?: string; action?: string }) {
		if (!editingRuleSet) return;
		const oldTag = editingRuleSet.tag;
		try {
			// Check if rule set itself changed (not just the route config)
			const ruleSetChanged =
				editingRuleSet.tag !== ruleSet.tag ||
				editingRuleSet.type !== ruleSet.type ||
				editingRuleSet.format !== ruleSet.format ||
				editingRuleSet.url !== ruleSet.url ||
				editingRuleSet.path !== ruleSet.path ||
				editingRuleSet.download_detour !== ruleSet.download_detour ||
				editingRuleSet.update_interval !== ruleSet.update_interval;

			const existingMapping = ruleSetOutboundMap.get(oldTag);

			// Only update rule set if it actually changed
			if (ruleSetChanged) {
				// If rule set is referenced by a rule, we need to update the rule first to remove reference
				if (existingMapping) {
					// Temporarily update the rule to not reference this rule set
					await api.deleteRule(existingMapping.ruleIndex);
				}

				// Delete old and create new rule set
				await api.deleteRuleSet(oldTag);
				await api.createRuleSet(ruleSet);
				ruleSets = ruleSets.map(rs => rs.tag === oldTag ? ruleSet : rs);
			}

			// Handle route config changes
			if (routeConfig && (routeConfig.outbound || routeConfig.action === 'reject')) {
				const rule: RouteRule = { rule_set: [ruleSet.tag] };
				if (routeConfig.action === 'reject') {
					rule.action = 'reject';
				} else if (routeConfig.outbound) {
					rule.outbound = routeConfig.outbound;
				}

				if (existingMapping && !ruleSetChanged) {
					// Just update the existing rule (rule set didn't change)
					await api.updateRule(existingMapping.ruleIndex, rule);
					rules = rules.map((r, i) => i === existingMapping.ruleIndex ? rule : r);
				} else {
					// Create new rule (either new mapping or we deleted the old rule above)
					await api.createRule(rule);
					if (ruleSetChanged && existingMapping) {
						// We deleted the old rule, so adjust the rules array
						rules = rules.filter((_, i) => i !== existingMapping.ruleIndex);
					}
					rules = [...rules, rule];
				}
			} else if (existingMapping && !ruleSetChanged) {
				// Route config removed - delete the rule
				await api.deleteRule(existingMapping.ruleIndex);
				rules = rules.filter((_, i) => i !== existingMapping.ruleIndex);
			}

			showRuleSetForm = false;
			editingRuleSet = null;
			hasChanges = true;
			unsavedChanges.markChanged('Routes', `Updated rule set "${ruleSet.tag}"`);
			notifications.success(`Rule set '${ruleSet.tag}' updated`);
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	async function handleDeleteRuleSet(tag: string) {
		if (!confirm(`Delete rule set "${tag}"?`)) return;
		try {
			await api.deleteRuleSet(tag);
			ruleSets = ruleSets.filter(rs => rs.tag !== tag);
			hasChanges = true;
			unsavedChanges.markChanged('Routes', `Deleted rule set "${tag}"`);
			notifications.success(`Rule set '${tag}' deleted`);
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	// Route Rule handlers
	async function handleCreateRule(rule: RouteRule) {
		try {
			await api.createRule(rule);
			rules = [...rules, rule];
			showRuleForm = false;
			hasChanges = true;
			unsavedChanges.markChanged('Routes', 'Created routing rule');
			notifications.success('Rule created');
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	async function handleUpdateRule(rule: RouteRule) {
		if (editingRuleIndex === null) return;
		try {
			await api.updateRule(editingRuleIndex, rule);
			rules = rules.map((r, i) => i === editingRuleIndex ? rule : r);
			showRuleForm = false;
			editingRuleIndex = null;
			hasChanges = true;
			unsavedChanges.markChanged('Routes', `Updated rule #${editingRuleIndex + 1}`);
			notifications.success('Rule updated');
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	async function handleDeleteRule(index: number) {
		if (!confirm(`Delete rule #${index + 1}?`)) return;
		try {
			await api.deleteRule(index);
			rules = rules.filter((_, i) => i !== index);
			hasChanges = true;
			unsavedChanges.markChanged('Routes', `Deleted rule #${index + 1}`);
			notifications.success('Rule deleted');
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	async function handleReorder(from: number, to: number) {
		try {
			await api.reorderRules(from, to);
			// Update local state
			const newRules = [...rules];
			const [moved] = newRules.splice(from, 1);
			newRules.splice(to, 0, moved);
			rules = newRules;
			hasChanges = true;
			unsavedChanges.markChanged('Routes', 'Reordered rules');
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	function openEditRule(index: number) {
		editingRuleIndex = index;
		showRuleForm = true;
	}

	function openAddRule() {
		editingRuleIndex = null;
		showRuleForm = true;
	}

	function openTemplates() {
		showTemplates = true;
	}

	async function handleTemplateSelect(ruleSet: RuleSet, rule: RouteRule) {
		showTemplates = false;
		try {
			// Create rule set first
			await api.createRuleSet(ruleSet);
			ruleSets = [...ruleSets, ruleSet];

			// Then create the rule
			await api.createRule(rule);
			rules = [...rules, rule];

			hasChanges = true;
			unsavedChanges.markChanged('Routes', `Added ${ruleSet.tag} from template`);
			notifications.success(`Added ${ruleSet.tag}`);
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	async function handleWizardSave(rule: RouteRule) {
		showWizard = false;
		try {
			await api.createRule(rule);
			rules = [...rules, rule];
			hasChanges = true;
			unsavedChanges.markChanged('Routes', 'Created rule via wizard');
			notifications.success('Rule created');
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	// Settings handlers
	async function handleSettingsChange() {
		try {
			await api.updateRouteSettings(settings);
			hasChanges = true;
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	onMount(fetchData);
</script>

<div class="space-y-6">
	<!-- Header with tabs -->
	<div class="flex items-center justify-between">
		<div class="flex items-center gap-6">
			<h1 class="text-2xl font-bold text-[var(--ctp-text)]">{$t('routes.routing')}</h1>
			<div class="flex border-b border-[var(--ctp-surface2)]">
				<button
					onclick={() => activeTab = 'rules'}
					class="px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors {activeTab === 'rules'
						? 'border-[var(--ctp-primary)] text-[var(--ctp-primary)]'
						: 'border-transparent text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)]'}"
				>
					{$t('routes.rules')}
				</button>
				<button
					onclick={() => activeTab = 'inspector'}
					class="px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors flex items-center gap-1.5 {activeTab === 'inspector'
						? 'border-[var(--ctp-primary)] text-[var(--ctp-primary)]'
						: 'border-transparent text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)]'}"
				>
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
					</svg>
					{$t('routes.inspector')}
				</button>
			</div>
		</div>
		<div class="flex items-center gap-2">
			<!-- Create Rule dropdown (Wizard + Templates) -->
			{#if activeTab === 'rules'}
				<div class="relative">
					<button
						onclick={() => showCreateMenu = !showCreateMenu}
						class="px-4 py-2 bg-[var(--ctp-green)] text-white rounded-lg hover:opacity-90 transition-opacity flex items-center gap-2"
					>
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
						</svg>
						{$t('routes.createRule')}
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
						</svg>
					</button>
					{#if showCreateMenu}
						<!-- svelte-ignore a11y_click_events_have_key_events -->
						<!-- svelte-ignore a11y_no_static_element_interactions -->
						<div
							class="fixed inset-0 z-40"
							onclick={() => showCreateMenu = false}
						></div>
						<div class="absolute right-0 mt-1 w-48 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg shadow-lg z-50 overflow-hidden">
							<button
								onclick={() => { showCreateMenu = false; showWizard = true; }}
								class="w-full px-4 py-2.5 text-left text-sm text-[var(--ctp-text)] hover:bg-[var(--ctp-surface1)] flex items-center gap-2"
							>
								<svg class="w-4 h-4 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
								</svg>
								{$t('routes.guidedWizard')}
							</button>
							<button
								onclick={() => { showCreateMenu = false; showTemplates = true; }}
								class="w-full px-4 py-2.5 text-left text-sm text-[var(--ctp-text)] hover:bg-[var(--ctp-surface1)] flex items-center gap-2"
							>
								<svg class="w-4 h-4 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 5a1 1 0 011-1h14a1 1 0 011 1v2a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM4 13a1 1 0 011-1h6a1 1 0 011 1v6a1 1 0 01-1 1H5a1 1 0 01-1-1v-6zM16 13a1 1 0 011-1h2a1 1 0 011 1v6a1 1 0 01-1 1h-2a1 1 0 01-1-1v-6z" />
								</svg>
								{$t('routes.quickTemplates')}
							</button>
						</div>
					{/if}
				</div>
			{/if}
			{#if hasChanges && activeTab === 'rules'}
				<button
					onclick={applyChanges}
					disabled={applying}
					class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50 flex items-center gap-2"
				>
					{#if applying}
						<svg class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
							<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
							<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
						</svg>
					{/if}
					{$t('changes.applyChanges')}
				</button>
			{/if}
		</div>
	</div>

	{#if loading}
		<div class="text-[var(--ctp-overlay0)]">{$t('common.loading')}</div>
	{:else if activeTab === 'inspector'}
		<!-- Route Inspector -->
		<RouteInspector {rules} finalOutbound={settings.final} />
	{:else}
		<!-- Route Settings -->
		<div class="bg-[var(--ctp-surface0)] rounded-xl p-4 space-y-4">
			<h2 class="font-medium text-[var(--ctp-subtext1)]">{$t('routes.routeSettings')}</h2>
			<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
				<div>
					<label for="final" class="block text-sm text-[var(--ctp-overlay1)] mb-1">{$t('routes.finalOutbound')}</label>
					<select
						id="final"
						bind:value={settings.final}
						onchange={handleSettingsChange}
						class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					>
						{#each allOutbounds as ob}
							<option value={ob.tag}>{ob.tag}</option>
						{/each}
					</select>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.trafficNoMatchHint')}</p>
				</div>
				<div class="flex items-start">
					<label class="flex items-center gap-2 cursor-pointer p-2">
						<input
							type="checkbox"
							bind:checked={settings.auto_detect_interface}
							onchange={handleSettingsChange}
							class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
						/>
						<div>
							<span class="text-[var(--ctp-text)]">{$t('routes.autoDetectInterface')}</span>
							<p class="text-xs text-[var(--ctp-overlay0)]">{$t('routes.autoDetectInterfaceHint')}</p>
						</div>
					</label>
				</div>
			</div>
		</div>

		<!-- Rule Sets Section -->
		<div class="bg-[var(--ctp-surface0)] rounded-xl overflow-hidden">
			<div class="px-4 py-3 bg-[var(--ctp-surface1)] border-b border-[var(--ctp-surface2)] flex items-center justify-between">
				<button
					onclick={() => ruleSetsExpanded = !ruleSetsExpanded}
					class="flex items-center gap-2 hover:text-[var(--ctp-text)] transition-colors"
				>
					<svg class="w-4 h-4 text-[var(--ctp-overlay1)] transition-transform {ruleSetsExpanded ? 'rotate-90' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
					</svg>
					<span class="font-medium text-[var(--ctp-subtext1)]">{$t('routes.ruleSets')}</span>
					<span class="text-sm text-[var(--ctp-overlay0)]">({ruleSets.length})</span>
				</button>
				<button
					onclick={() => { showRuleSetForm = true; editingRuleSet = null; }}
					class="px-3 py-1.5 text-sm bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
				>
					+ {$t('common.add')}
				</button>
			</div>

			{#if ruleSetsExpanded}
				{#if ruleSets.length === 0}
					<div class="p-6 text-center text-[var(--ctp-overlay0)]">
						<p>{$t('routes.noRuleSets')}</p>
						<p class="text-sm mt-1">{$t('routes.noRuleSetsHint')}</p>
					</div>
				{:else}
					<div class="divide-y divide-[var(--ctp-surface2)]">
						{#each ruleSets as ruleSet}
							{@const mapping = ruleSetOutboundMap.get(ruleSet.tag)}
							<div class="px-4 py-3 flex items-center justify-between group">
								<div class="flex items-center gap-3 min-w-0 flex-1">
									<span class="px-2 py-0.5 text-xs rounded bg-[var(--ctp-surface2)] text-[var(--ctp-overlay1)] flex-shrink-0">
										{ruleSet.type}
									</span>
									<span class="font-medium text-[var(--ctp-text)]">{ruleSet.tag}</span>
									{#if mapping}
										<span class="text-[var(--ctp-overlay0)]">→</span>
										<span class="px-2 py-0.5 text-xs rounded flex-shrink-0 {mapping.action === 'reject' ? 'bg-[var(--ctp-red)] text-white' : 'bg-[var(--ctp-primary)] text-white'}">
											{mapping.outbound}
										</span>
									{:else}
										<span class="text-xs text-[var(--ctp-overlay0)] italic">{$t('routes.noRoute')}</span>
									{/if}
								</div>
								<div class="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
									<button
										onclick={() => viewingRuleSet = ruleSet}
										class="p-1.5 rounded hover:bg-[var(--ctp-surface2)] text-[var(--ctp-overlay1)]"
										title={$t('routes.viewDetails')}
									>
										<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
										</svg>
									</button>
									<button
										onclick={() => { editingRuleSet = ruleSet; showRuleSetForm = true; }}
										class="p-1.5 rounded hover:bg-[var(--ctp-surface2)] text-[var(--ctp-overlay1)]"
										title={$t('common.edit')}
									>
										<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
										</svg>
									</button>
									<button
										onclick={() => handleDeleteRuleSet(ruleSet.tag)}
										class="p-1.5 rounded hover:bg-[var(--ctp-red)] hover:bg-opacity-20 text-[var(--ctp-red)]"
										title={$t('common.delete')}
									>
										<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
										</svg>
									</button>
								</div>
							</div>
						{/each}
					</div>
				{/if}
			{/if}
		</div>

		<!-- Route Rules Section (Complex rules only, simple rule_set mappings shown above) -->
		<div class="bg-[var(--ctp-surface0)] rounded-xl overflow-hidden">
			<div class="px-4 py-3 bg-[var(--ctp-surface1)] border-b border-[var(--ctp-surface2)] flex items-center justify-between">
				<button
					onclick={() => rulesExpanded = !rulesExpanded}
					class="flex items-center gap-2 hover:text-[var(--ctp-text)] transition-colors"
				>
					<svg class="w-4 h-4 text-[var(--ctp-overlay1)] transition-transform {rulesExpanded ? 'rotate-90' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
					</svg>
					<span class="font-medium text-[var(--ctp-subtext1)]">{$t('routes.advancedRules')}</span>
					<span class="text-sm text-[var(--ctp-overlay0)]">({filteredRules.length})</span>
				</button>
				<button
					onclick={openAddRule}
					class="px-3 py-1.5 text-sm bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
				>
					+ {$t('common.add')}
				</button>
			</div>

			{#if rulesExpanded}
				<div class="p-4">
					<DraggableRuleList
						rules={filteredRules.map(r => r.rule)}
						onReorder={(from, to) => {
							// Map filtered indices back to original indices
							const fromOriginal = filteredRules[from].originalIndex;
							const toOriginal = filteredRules[to].originalIndex;
							handleReorder(fromOriginal, toOriginal);
						}}
						onEdit={(index) => openEditRule(filteredRules[index].originalIndex)}
						onDelete={(index) => handleDeleteRule(filteredRules[index].originalIndex)}
					/>
				</div>

				{#if filteredRules.length > 0}
					<div class="px-4 py-3 border-t border-[var(--ctp-surface2)] bg-[var(--ctp-surface1)]">
						<div class="flex items-center gap-2 text-sm text-[var(--ctp-overlay1)]">
							<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
							</svg>
							<span>{$t('routes.rulesOrderHint')}</span>
						</div>
					</div>
				{/if}
			{/if}
		</div>

		<!-- Final destination indicator -->
		{#if rules.length > 0}
			<div class="flex items-center justify-center gap-3 py-2">
				<div class="h-px flex-1 bg-[var(--ctp-surface2)]"></div>
				<div class="info-banner">
					<span class="text-sm text-[var(--ctp-overlay1)]">{$t('routes.noMatch')}:</span>
					<span class="text-sm font-medium text-[var(--ctp-primary)]">{settings.final}</span>
				</div>
				<div class="h-px flex-1 bg-[var(--ctp-surface2)]"></div>
			</div>
		{/if}
	{/if}
</div>

<!-- Rule Set Form Modal -->
{#if showRuleSetForm}
	<div class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
		<div class="bg-[var(--ctp-base)] rounded-xl w-full max-w-lg max-h-[90vh] overflow-y-auto">
			<div class="px-4 py-3 border-b border-[var(--ctp-surface2)] flex items-center justify-between">
				<h2 class="text-lg font-medium text-[var(--ctp-text)]">
					{editingRuleSet ? `${$t('routes.editRuleSet')}: ${editingRuleSet.tag}` : $t('routes.addRuleSet')}
				</h2>
				<button
					onclick={() => { showRuleSetForm = false; editingRuleSet = null; }}
					class="p-1 rounded-md hover:bg-[var(--ctp-surface1)] text-[var(--ctp-overlay1)]"
					aria-label="Close"
				>
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>
			<div class="p-4">
				<RuleSetForm
					existingTags={ruleSets.filter(rs => rs.tag !== editingRuleSet?.tag).map(rs => rs.tag)}
					outbounds={allOutbounds}
					ruleSet={editingRuleSet}
					currentOutbound={editingRuleSet ? ruleSetOutboundMap.get(editingRuleSet.tag)?.outbound : undefined}
					currentAction={editingRuleSet ? ruleSetOutboundMap.get(editingRuleSet.tag)?.action : undefined}
					onSave={editingRuleSet ? handleUpdateRuleSet : handleCreateRuleSet}
					onCancel={() => { showRuleSetForm = false; editingRuleSet = null; }}
				/>
			</div>
		</div>
	</div>
{/if}

<!-- Rule Set View Modal -->
{#if viewingRuleSet}
	<div class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
		<div class="bg-[var(--ctp-base)] rounded-xl w-full max-w-md">
			<div class="px-4 py-3 border-b border-[var(--ctp-surface2)] flex items-center justify-between">
				<h2 class="text-lg font-medium text-[var(--ctp-text)]">{$t('routes.ruleSetDetails')}</h2>
				<button
					onclick={() => viewingRuleSet = null}
					class="p-1 rounded-md hover:bg-[var(--ctp-surface1)] text-[var(--ctp-overlay1)]"
					aria-label="Close"
				>
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>
			<div class="p-4 space-y-3">
				<div>
					<span class="text-sm text-[var(--ctp-overlay1)]">{$t('common.tag')}</span>
					<p class="font-medium text-[var(--ctp-text)]">{viewingRuleSet.tag}</p>
				</div>
				<div>
					<span class="text-sm text-[var(--ctp-overlay1)]">{$t('common.type')}</span>
					<p class="text-[var(--ctp-text)]">{viewingRuleSet.type}</p>
				</div>
				<div>
					<span class="text-sm text-[var(--ctp-overlay1)]">{$t('routes.ruleSetFormat')}</span>
					<p class="text-[var(--ctp-text)]">{viewingRuleSet.format}</p>
				</div>
				{#if viewingRuleSet.url}
					<div>
						<span class="text-sm text-[var(--ctp-overlay1)]">{$t('routes.ruleSetUrl')}</span>
						<a href={viewingRuleSet.url} target="_blank" rel="noopener" class="block text-[var(--ctp-primary)] hover:underline break-all text-sm">
							{viewingRuleSet.url}
						</a>
					</div>
				{/if}
				{#if viewingRuleSet.path}
					<div>
						<span class="text-sm text-[var(--ctp-overlay1)]">{$t('routes.ruleSetPath')}</span>
						<p class="text-[var(--ctp-text)] font-mono text-sm">{viewingRuleSet.path}</p>
					</div>
				{/if}
				{#if ruleSetOutboundMap.get(viewingRuleSet.tag)}
					{@const mapping = ruleSetOutboundMap.get(viewingRuleSet.tag)!}
					<div>
						<span class="text-sm text-[var(--ctp-overlay1)]">{$t('routes.routeTo')}</span>
						<p class="font-medium {mapping.action === 'reject' ? 'text-[var(--ctp-red)]' : 'text-[var(--ctp-green)]'}">
							{mapping.outbound}
						</p>
					</div>
				{/if}
			</div>
			<div class="px-4 py-3 border-t border-[var(--ctp-surface2)] flex justify-end gap-2">
				<button
					onclick={() => { editingRuleSet = viewingRuleSet; viewingRuleSet = null; showRuleSetForm = true; }}
					class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90"
				>
					{$t('common.edit')}
				</button>
				<button
					onclick={() => viewingRuleSet = null}
					class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)]"
				>
					{$t('common.close')}
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Rule Form Modal -->
{#if showRuleForm}
	<div class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
		<div class="bg-[var(--ctp-base)] rounded-xl w-full max-w-lg max-h-[90vh] overflow-y-auto">
			<div class="px-4 py-3 border-b border-[var(--ctp-surface2)] flex items-center justify-between">
				<h2 class="text-lg font-medium text-[var(--ctp-text)]">
					{editingRuleIndex !== null ? $t('routes.editRuleNumber', { values: { number: editingRuleIndex + 1 } }) : $t('routes.addRule')}
				</h2>
				<button
					onclick={() => { showRuleForm = false; editingRuleIndex = null; }}
					class="p-1 rounded-md hover:bg-[var(--ctp-surface1)] text-[var(--ctp-overlay1)]"
					aria-label="Close"
				>
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>
			<div class="p-4">
				<RuleForm
					rule={editingRuleIndex !== null ? rules[editingRuleIndex] : undefined}
					{ruleSets}
					outbounds={allOutbounds}
					{inbounds}
					hiddenRuleSets={editingRuleIndex === null ? usedRuleSets : undefined}
					onSave={editingRuleIndex !== null ? handleUpdateRule : handleCreateRule}
					onCancel={() => { showRuleForm = false; editingRuleIndex = null; }}
				/>
			</div>
		</div>
	</div>
{/if}

<!-- Rule Templates Modal -->
{#if showTemplates}
	<div class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
		<div class="bg-[var(--ctp-base)] rounded-xl w-full max-w-lg max-h-[90vh] overflow-y-auto">
			<div class="px-4 py-3 border-b border-[var(--ctp-surface2)] flex items-center justify-between">
				<h2 class="text-lg font-medium text-[var(--ctp-text)]">{$t('routes.quickTemplates')}</h2>
				<button
					onclick={() => showTemplates = false}
					class="p-1 rounded-md hover:bg-[var(--ctp-surface1)] text-[var(--ctp-overlay1)]"
					aria-label="Close"
				>
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>
			<div class="p-4">
				<RuleTemplates
					outbounds={allOutbounds}
					existingRuleSetTags={ruleSets.map(rs => rs.tag)}
					onSelect={handleTemplateSelect}
					onClose={() => showTemplates = false}
				/>
			</div>
		</div>
	</div>
{/if}

<!-- Rule Wizard Modal -->
{#if showWizard}
	<div class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
		<div class="bg-[var(--ctp-base)] rounded-xl w-full max-w-lg max-h-[90vh] overflow-y-auto">
			<div class="px-4 py-3 border-b border-[var(--ctp-surface2)] flex items-center justify-between">
				<h2 class="text-lg font-medium text-[var(--ctp-text)]">{$t('routes.createRule')}</h2>
				<button
					onclick={() => showWizard = false}
					class="p-1 rounded-md hover:bg-[var(--ctp-surface1)] text-[var(--ctp-overlay1)]"
					aria-label="Close"
				>
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>
			<div class="p-4">
				<RuleWizard
					{ruleSets}
					outbounds={allOutbounds}
					{inbounds}
					{usedRuleSets}
					onSave={handleWizardSave}
					onCancel={() => showWizard = false}
					onCreateRuleSet={() => { showWizard = false; showRuleSetForm = true; editingRuleSet = null; }}
				/>
			</div>
		</div>
	</div>
{/if}
