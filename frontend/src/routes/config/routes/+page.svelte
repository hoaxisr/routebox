<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, unsavedChanges, featureFlags, configReadOnly } from '$lib/stores';
	import type { RouteRule, RuleSet, Outbound, Inbound, RouteSettings, Endpoint } from '$lib/types';
	import RuleForm from '$lib/components/config/RuleForm.svelte';
	import RuleTemplates from '$lib/components/config/RuleTemplates.svelte';
	import RuleWizard from '$lib/components/config/RuleWizard.svelte';
	import RouteInspector from '$lib/components/config/RouteInspector.svelte';
	import DraggableRuleList from '$lib/components/config/DraggableRuleList.svelte';
	import HelpTooltip from '$lib/components/shared/HelpTooltip.svelte';

	type Tab = 'rules' | 'inspector';
	let activeTab = $state<Tab>('rules');

	let rules = $state<RouteRule[]>([]);
	let ruleSets = $state<RuleSet[]>([]);
	let outbounds = $state<Outbound[]>([]);
	let endpoints = $state<Endpoint[]>([]);
	let inbounds = $state<Inbound[]>([]);
	let settings = $state<RouteSettings>({ final: 'direct' });

	// Combined list: outbounds + endpoints (endpoints can be used directly as routing targets).
	// The managed 'awg-server' endpoint is a server inbound, not an egress — exclude it as a
	// routing target (it stays valid as a source below).
	let allOutbounds = $derived([
		...outbounds,
		...endpoints.filter(e => e.tag !== 'awg-server').map(e => ({ tag: e.tag, type: e.type } as Outbound))
	]);
	// Source options: inbounds + server-capable endpoints (listen_port set).
	// amnezia-box tags AWG/WG server traffic with the endpoint tag as inbound,
	// so endpoint tags are valid values for the rule `inbound` field.
	let sourceInbounds = $derived([
		...inbounds,
		...endpoints
			.filter(e => (e.type === 'awg' || e.type === 'wireguard') && e.listen_port)
			.map(e => ({ tag: e.tag, type: `${e.type} ${$t('routes.serverEndpoint')}` } as Inbound))
	]);
	let loading = $state(true);
	let hasChanges = $state(false);
	let applying = $state(false);

	// Collapsed sections state
	let ruleSetsExpanded = $state(false);
	let rulesExpanded = $state(false);

	// Modal states
	let showRuleForm = $state(false);
	let showTemplates = $state(false);
	let showWizard = $state(false);
	let showCreateMenu = $state(false);
	let editingRuleIndex = $state<number | null>(null);

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
			notifications.success($t('changes.configApplied'));
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

	// Derived: rule-sets that have a route rule, sorted by rule priority (route-rule order)
	let assignedRuleSets = $derived(
		ruleSets
			.filter((rs) => ruleSetOutboundMap.has(rs.tag))
			.map((rs) => ({ ruleSet: rs, ruleIndex: ruleSetOutboundMap.get(rs.tag)!.ruleIndex }))
			.sort((a, b) => a.ruleIndex - b.ruleIndex)
	);
	// Derived: rule-sets without a route rule ("no route") — no priority, not draggable
	let unassignedRuleSets = $derived(ruleSets.filter((rs) => !ruleSetOutboundMap.has(rs.tag)));

	// Drag & drop state for the assigned rule-set list
	let rsDraggedIndex = $state<number | null>(null);
	let rsDropTargetIndex = $state<number | null>(null);

	function handleRsDragStart(e: DragEvent, index: number) {
		rsDraggedIndex = index;
		if (e.dataTransfer) {
			e.dataTransfer.effectAllowed = 'move';
			e.dataTransfer.setData('text/plain', index.toString());
		}
	}

	function handleRsDragOver(e: DragEvent, index: number) {
		e.preventDefault();
		if (e.dataTransfer) {
			e.dataTransfer.dropEffect = 'move';
		}
		if (rsDraggedIndex !== null && rsDraggedIndex !== index) {
			rsDropTargetIndex = index;
		}
	}

	function handleRsDragLeave() {
		rsDropTargetIndex = null;
	}

	async function handleRsDrop(e: DragEvent, toIndex: number) {
		e.preventDefault();
		if (rsDraggedIndex !== null && rsDraggedIndex !== toIndex) {
			// Translate section positions to full rules-array indices and reuse the shared handler
			await handleReorder(assignedRuleSets[rsDraggedIndex].ruleIndex, assignedRuleSets[toIndex].ruleIndex);
		}
		rsDraggedIndex = null;
		rsDropTargetIndex = null;
	}

	function handleRsDragEnd() {
		rsDraggedIndex = null;
		rsDropTargetIndex = null;
	}

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

	// Route Rule handlers
	async function handleCreateRule(rule: RouteRule) {
		try {
			await api.createRule(rule);
			rules = [...rules, rule];
			showRuleForm = false;
			hasChanges = true;
			unsavedChanges.markChanged('Routes', 'Created routing rule');
			notifications.success($t('routes.ruleCreated'));
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
			hasChanges = true;
			unsavedChanges.markChanged('Routes', `Updated rule #${editingRuleIndex + 1}`);
			editingRuleIndex = null;
			notifications.success($t('routes.ruleUpdated'));
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
			notifications.success($t('routes.ruleDeleted'));
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
			notifications.success($t('routes.ruleSetAdded', { values: { tag: ruleSet.tag } }));
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
			notifications.success($t('routes.ruleCreated'));
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	function handleRuleSetCreated(ruleSet: RuleSet) {
		ruleSets = [...ruleSets, ruleSet];
		hasChanges = true;
		unsavedChanges.markChanged('Routes', `Created rule set "${ruleSet.tag}"`);
	}

	// Inline outbound change for rule set mappings
	async function handleRuleSetOutboundChange(ruleSetTag: string, newOutbound: string) {
		const mapping = ruleSetOutboundMap.get(ruleSetTag);
		if (mapping) {
			// Update existing rule
			const updatedRule = { ...rules[mapping.ruleIndex] };
			if (newOutbound === '__reject__') {
				updatedRule.action = 'reject';
				delete updatedRule.outbound;
			} else {
				updatedRule.action = 'route';
				updatedRule.outbound = newOutbound;
			}
			try {
				await api.updateRule(mapping.ruleIndex, updatedRule);
				rules = rules.map((r, i) => i === mapping.ruleIndex ? updatedRule : r);
				hasChanges = true;
				unsavedChanges.markChanged('Routes', `Changed outbound for ${ruleSetTag}`);
			} catch (e) {
				notifications.error(`${e}`);
			}
		} else {
			// Create new simple rule
			const rule: RouteRule = newOutbound === '__reject__'
				? { rule_set: [ruleSetTag], action: 'reject' }
				: { rule_set: [ruleSetTag], outbound: newOutbound };
			try {
				await api.createRule(rule);
				rules = [...rules, rule];
				hasChanges = true;
				unsavedChanges.markChanged('Routes', `Added route for ${ruleSetTag}`);
			} catch (e) {
				notifications.error(`${e}`);
			}
		}
	}

	async function handleDeleteRuleSetMapping(ruleSetTag: string) {
		const mapping = ruleSetOutboundMap.get(ruleSetTag);
		if (!mapping) return;
		if (!confirm($t('routes.deleteRuleSetMapping', { values: { tag: ruleSetTag } }) || `Remove route for ${ruleSetTag}?`)) return;
		try {
			await api.deleteRule(mapping.ruleIndex);
			rules = rules.filter((_, i) => i !== mapping.ruleIndex);
			hasChanges = true;
			unsavedChanges.markChanged('Routes', `Removed route for ${ruleSetTag}`);
			notifications.success($t('common.deleted'));
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
					{$t('routes.inspector.title')}
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
					disabled={applying || $configReadOnly}
					title={$configReadOnly ? $t('readOnly.saveBlocked') : ''}
					class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
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
					<label for="final" class="flex items-center gap-1 text-sm text-[var(--ctp-overlay1)] mb-1">
						{$t('routes.finalOutbound')}
						<HelpTooltip text={$t('help.finalOutbound')} />
					</label>
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
				<div>
					<label for="default_interface" class="block text-sm text-[var(--ctp-overlay1)] mb-1">{$t('routes.defaultInterface')}</label>
					<input
						id="default_interface"
						type="text"
						bind:value={settings.default_interface}
						onchange={handleSettingsChange}
						placeholder="eth0"
						class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.defaultInterfaceHint')}</p>
				</div>
					{#if $featureFlags['default_domain_resolver']}
					<div>
						<label for="default_domain_resolver" class="block text-sm text-[var(--ctp-overlay1)] mb-1">{$t('routes.defaultDomainResolver')}</label>
						<input
							id="default_domain_resolver"
							type="text"
							bind:value={settings.default_domain_resolver}
							onchange={handleSettingsChange}
							placeholder="dns-server-tag"
							class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
						<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.defaultDomainResolverHint')}</p>
					</div>
				{/if}
				{#if !$featureFlags['default_domain_resolver']}
				<div>
					<label for="default_domain_strategy" class="block text-sm text-[var(--ctp-overlay1)] mb-1">{$t('routes.defaultDomainStrategy')}</label>
					<select
						id="default_domain_strategy"
						bind:value={settings.default_domain_strategy}
						onchange={handleSettingsChange}
						class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					>
						<option value="">{$t('common.none')}</option>
						<option value="prefer_ipv4">{$t('dns.strategies.preferIpv4')}</option>
						<option value="prefer_ipv6">{$t('dns.strategies.preferIpv6')}</option>
						<option value="ipv4_only">{$t('dns.strategies.ipv4Only')}</option>
						<option value="ipv6_only">{$t('dns.strategies.ipv6Only')}</option>
					</select>
				</div>
			{/if}
			</div>
		</div>

		<!-- Rule Sets Section (read-only overview) -->
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
				<a
					href="/config/rule-sets"
					class="px-3 py-1.5 text-sm text-[var(--ctp-primary)] hover:bg-[var(--ctp-surface2)] rounded-lg transition-colors flex items-center gap-1"
				>
					{$t('ruleSets.manageRuleSets')}
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
					</svg>
				</a>
			</div>

			{#if ruleSetsExpanded}
				{#if ruleSets.length === 0}
					<div class="p-6 text-center text-[var(--ctp-overlay0)]">
						<p>{$t('routes.noRuleSets')}</p>
						<p class="text-sm mt-1">{$t('routes.noRuleSetsHint')}</p>
					</div>
				{:else}
					{#snippet ruleSetRowContent(ruleSet: RuleSet, hasMapping: boolean)}
						{@const mapping = ruleSetOutboundMap.get(ruleSet.tag)}
						<span class="px-2 py-0.5 text-xs rounded bg-[var(--ctp-surface2)] text-[var(--ctp-overlay1)] flex-shrink-0">
							{ruleSet.type}
						</span>
						<span class="font-medium text-[var(--ctp-text)]">{ruleSet.tag}</span>
						<span class="text-[var(--ctp-overlay0)]">→</span>
						<select
							value={mapping ? (mapping.action === 'reject' ? '__reject__' : mapping.outbound) : ''}
							onchange={(e) => {
								const val = (e.target as HTMLSelectElement).value;
								if (val) handleRuleSetOutboundChange(ruleSet.tag, val);
							}}
							class="px-2 py-1 text-xs rounded bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] text-[var(--ctp-text)] focus:outline-none focus:ring-1 focus:ring-[var(--ctp-primary)] cursor-pointer"
						>
							{#if !hasMapping}
								<option value="" disabled selected>{$t('routes.noRoute')}</option>
							{/if}
							{#each allOutbounds as ob}
								<option value={ob.tag}>{ob.tag}</option>
							{/each}
							<option value="__reject__">⛔ REJECT</option>
						</select>
					{/snippet}

					{#snippet ruleSetDeleteButton(ruleSet: RuleSet)}
						<button
							onclick={() => handleDeleteRuleSetMapping(ruleSet.tag)}
							class="p-1.5 rounded-md hover:bg-[var(--ctp-red)] hover:bg-opacity-10 text-[var(--ctp-overlay1)] hover:text-[var(--ctp-red)] transition-colors"
							title={$t('common.delete')}
						>
							<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
							</svg>
						</button>
					{/snippet}

					<div class="divide-y divide-[var(--ctp-surface2)]">
						<!-- Assigned rule-sets: draggable, ordered by route-rule priority -->
						{#each assignedRuleSets as item, i (item.ruleSet.tag)}
							<div
								draggable="true"
								ondragstart={(e) => handleRsDragStart(e, i)}
								ondragover={(e) => handleRsDragOver(e, i)}
								ondragleave={handleRsDragLeave}
								ondrop={(e) => handleRsDrop(e, i)}
								ondragend={handleRsDragEnd}
								class="group relative px-4 py-3 flex items-center justify-between hover:bg-[var(--ctp-surface1)] transition-colors cursor-move
									{rsDraggedIndex === i ? 'opacity-50' : ''}"
							>
								<div class="flex items-center gap-3 min-w-0 flex-1">
									<!-- Drag handle -->
									<div class="flex-shrink-0 text-[var(--ctp-overlay0)] group-hover:text-[var(--ctp-text)] transition-colors">
										<svg class="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
											<path d="M7 2a2 2 0 1 0 .001 4.001A2 2 0 0 0 7 2zm0 6a2 2 0 1 0 .001 4.001A2 2 0 0 0 7 8zm0 6a2 2 0 1 0 .001 4.001A2 2 0 0 0 7 14zm6-8a2 2 0 1 0-.001-4.001A2 2 0 0 0 13 6zm0 2a2 2 0 1 0 .001 4.001A2 2 0 0 0 13 8zm0 6a2 2 0 1 0 .001 4.001A2 2 0 0 0 13 14z" />
										</svg>
									</div>
									<!-- Priority number -->
									<div class="w-6 h-6 flex-shrink-0 rounded-full bg-[var(--ctp-surface2)] flex items-center justify-center text-xs font-medium text-[var(--ctp-subtext0)]">
										{i + 1}
									</div>
									{@render ruleSetRowContent(item.ruleSet, true)}
								</div>
								<div class="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
									{@render ruleSetDeleteButton(item.ruleSet)}
								</div>
								<!-- Drop indicator -->
								{#if rsDropTargetIndex === i}
									<div class="absolute -top-px left-0 right-0 h-0.5 bg-[var(--ctp-primary)] z-10"></div>
								{/if}
							</div>
						{/each}

						<!-- Unassigned rule-sets: no route rule, no priority, not draggable -->
						{#each unassignedRuleSets as ruleSet (ruleSet.tag)}
							<div class="group px-4 py-3 flex items-center justify-between hover:bg-[var(--ctp-surface1)] transition-colors">
								<div class="flex items-center gap-3 min-w-0 flex-1">
									{@render ruleSetRowContent(ruleSet, false)}
								</div>
							</div>
						{/each}
					</div>

					{#if assignedRuleSets.length > 1}
						<div class="px-4 py-3 border-t border-[var(--ctp-surface2)] bg-[var(--ctp-surface1)]">
							<div class="flex items-center gap-2 text-sm text-[var(--ctp-overlay1)]">
								<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
								</svg>
								<span>{$t('routes.ruleSetsOrderHint')}</span>
							</div>
						</div>
					{/if}
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
						onAdd={openAddRule}
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
					inbounds={sourceInbounds}
					onSave={editingRuleIndex !== null ? handleUpdateRule : handleCreateRule}
					onCancel={() => { showRuleForm = false; editingRuleIndex = null; }}
					onRuleSetCreated={handleRuleSetCreated}
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
					inbounds={sourceInbounds}
					usedRuleSets={new Set(ruleSetOutboundMap.keys())}
					onSave={handleWizardSave}
					onCancel={() => showWizard = false}
					onCreateRuleSet={() => { showWizard = false; }}
				/>
			</div>
		</div>
	</div>
{/if}
