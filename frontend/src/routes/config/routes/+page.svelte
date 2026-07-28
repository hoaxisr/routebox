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
	import {
		assignedRuleSetTags,
		simpleRuleSetTag,
		applyMappingOutbound,
		reorderArray
	} from '$lib/utils/routeRules';

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
	// Open by default: with rule-sets and full rules merged into one list, this
	// section IS the page — collapsed it would show nothing but a header.
	let rulesExpanded = $state(true);

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

	// Rule-sets that already have a plain mapping somewhere in the rules array.
	// The mapping ITSELF is a row of the one ordered list below — it is not
	// hoisted into a section of its own, because both kinds of rule live in the
	// same route.rules array and only their global order decides what matches
	// first. Two separately-numbered sections made that order unrepresentable:
	// a full rule could never be placed above a rule-set one.
	let assignedTags = $derived(assignedRuleSetTags(rules));
	// Rule-sets with no rule at all: nothing to order, so they sit apart and
	// their picker CREATES the mapping (appended last, then draggable).
	let unassignedRuleSets = $derived(ruleSets.filter((rs) => !assignedTags.has(rs.tag)));

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
		// A row drawn as a rule-set is confirmed BY TAG: "#7" is a number that
		// just moved if anything was dragged, and it reads as if the rule-set
		// itself were being deleted (only its route rule is).
		const tag = simpleRuleSetTag(rules[index]);
		const prompt = tag
			? $t('routes.deleteRuleSetMapping', { values: { tag } })
			: $t('routes.deleteRuleConfirm', { values: { number: index + 1 } });
		if (!confirm(prompt)) return;
		try {
			await api.deleteRule(index);
			rules = rules.filter((_, i) => i !== index);
			hasChanges = true;
			unsavedChanges.markChanged('Routes', tag ? `Removed route for ${tag}` : `Deleted rule #${index + 1}`);
			notifications.success($t('routes.ruleDeleted'));
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	async function handleReorder(from: number, to: number) {
		try {
			await api.reorderRules(from, to);
			// Same contract as the backend: `to` is where the rule ends up.
			rules = reorderArray(rules, from, to);
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

	// Inline outbound switch on a rule-set row of the ordered list. The rule keeps
	// its position: only its destination changes.
	async function handleMappingOutboundChange(index: number, newOutbound: string) {
		const updatedRule = applyMappingOutbound(rules[index], newOutbound);
		const tag = simpleRuleSetTag(rules[index]);
		try {
			await api.updateRule(index, updatedRule);
			rules = rules.map((r, i) => i === index ? updatedRule : r);
			hasChanges = true;
			unsavedChanges.markChanged('Routes', `Changed outbound for ${tag ?? `rule #${index + 1}`}`);
		} catch (e) {
			// The select is one-way bound, so a failed write would leave the
			// user's choice on screen while the rule is unchanged. Re-assigning
			// forces it back to what the config actually says.
			rules = [...rules];
			notifications.error(`${e}`);
		}
	}

	// Assigning an unassigned rule-set appends the mapping at the END of the
	// rules array — last priority, then draggable anywhere like any other rule.
	async function handleAssignRuleSet(ruleSetTag: string, newOutbound: string) {
		const rule = applyMappingOutbound({ rule_set: [ruleSetTag] }, newOutbound);
		try {
			await api.createRule(rule);
			rules = [...rules, rule];
			hasChanges = true;
			unsavedChanges.markChanged('Routes', `Added route for ${ruleSetTag}`);
		} catch (e) {
			// Reset the picker: nothing was created (see handleMappingOutboundChange).
			ruleSets = [...ruleSets];
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

		<!-- Routing rules: ONE ordered list. Rule-set mappings and full rules share
		     the same route.rules array, so a single global order is the only honest
		     way to show — and set — which rule matches first (#37 follow-up). -->
		<div class="bg-[var(--ctp-surface0)] rounded-xl overflow-hidden">
			<div class="px-4 py-3 bg-[var(--ctp-surface1)] border-b border-[var(--ctp-surface2)] flex items-center justify-between gap-2 flex-wrap">
				<button
					onclick={() => rulesExpanded = !rulesExpanded}
					class="flex items-center gap-2 hover:text-[var(--ctp-text)] transition-colors"
				>
					<svg class="w-4 h-4 text-[var(--ctp-overlay1)] transition-transform {rulesExpanded ? 'rotate-90' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
					</svg>
					<span class="font-medium text-[var(--ctp-subtext1)]">{$t('routes.rulesTitle')}</span>
					<span class="text-sm text-[var(--ctp-overlay0)]">({rules.length})</span>
				</button>
				<div class="flex items-center gap-2">
					<a
						href="/config/rule-sets"
						class="px-3 py-1.5 text-sm text-[var(--ctp-primary)] hover:bg-[var(--ctp-surface2)] rounded-lg transition-colors flex items-center gap-1"
					>
						{$t('ruleSets.manageRuleSets')}
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
						</svg>
					</a>
					<button
						onclick={openAddRule}
						class="px-3 py-1.5 text-sm bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
					>
						+ {$t('common.add')}
					</button>
				</div>
			</div>

			{#if rulesExpanded}
				<div class="p-4">
					<DraggableRuleList
						{rules}
						{ruleSets}
						outbounds={allOutbounds}
						onReorder={handleReorder}
						onEdit={openEditRule}
						onDelete={handleDeleteRule}
						onOutboundChange={handleMappingOutboundChange}
						finalOutbound={settings.final}
						onAdd={openAddRule}
					/>
				</div>

				{#if unassignedRuleSets.length > 0}
					<div class="px-4 pb-4">
						<div class="text-xs uppercase tracking-wide text-[var(--ctp-overlay0)] mb-2">
							{$t('routes.unassignedRuleSets')}
						</div>
						<div class="rounded-lg border border-dashed border-[var(--ctp-surface2)] divide-y divide-[var(--ctp-surface2)]">
							{#each unassignedRuleSets as ruleSet (ruleSet.tag)}
								<div class="px-3 py-2 flex items-center gap-2 flex-wrap">
									<span class="px-2 py-0.5 text-xs rounded bg-[var(--ctp-surface2)] text-[var(--ctp-overlay1)] flex-shrink-0">
										{ruleSet.type}
									</span>
									<span class="text-sm font-medium text-[var(--ctp-text)] truncate">{ruleSet.tag}</span>
									<span class="text-[var(--ctp-overlay0)]">&rarr;</span>
									<select
										value=""
										onchange={(e) => {
											const val = (e.target as HTMLSelectElement).value;
											if (val) handleAssignRuleSet(ruleSet.tag, val);
										}}
										class="px-2 py-1 text-xs rounded bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] text-[var(--ctp-text)] focus:outline-none focus:ring-1 focus:ring-[var(--ctp-primary)] cursor-pointer"
									>
										<option value="" disabled selected>{$t('routes.noRoute')}</option>
										{#each allOutbounds as ob}
											<option value={ob.tag}>{ob.tag}</option>
										{/each}
										<option value="__reject__">&#9940; REJECT</option>
									</select>
								</div>
							{/each}
						</div>
						<p class="mt-2 text-xs text-[var(--ctp-overlay0)]">{$t('routes.unassignedRuleSetsHint')}</p>
					</div>
				{/if}

				{#if rules.length > 0}
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
					usedRuleSets={assignedTags}
					onSave={handleWizardSave}
					onCancel={() => showWizard = false}
					onCreateRuleSet={() => { showWizard = false; }}
				/>
			</div>
		</div>
	</div>
{/if}
