<script lang="ts">
	import type { RouteRule, RuleSet, Outbound, Inbound, DnsServer, TlsFragment, TlsRecordFragment, RuleConditions } from '$lib/types';
	import { notifications } from '$lib/stores';
	import { api } from '$lib/api/client';
	import { t } from 'svelte-i18n';
	import RuleSetForm from './RuleSetForm.svelte';
	import {
		ConditionsForm,
		ActionSelector,
		OutboundSelector,
		RejectOptions,
		ResolveOptions,
		SniffOptions,
		RouteOptions,
		RuleCombiner
	} from './rules';

	interface Props {
		rule?: RouteRule;
		ruleSets: RuleSet[];
		outbounds: Outbound[];
		inbounds?: Inbound[];
		hiddenRuleSets?: Set<string>;
		onSave: (rule: RouteRule) => void;
		onCancel: () => void;
		onRuleSetCreated?: (ruleSet: RuleSet) => void;
	}

	let { rule, ruleSets, outbounds, inbounds = [], hiddenRuleSets, onSave, onCancel, onRuleSetCreated }: Props = $props();

	let showInlineRuleSetForm = $state(false);

	// DNS servers (loaded for resolve action)
	let dnsServers = $state<DnsServer[]>([]);
	let dnsServersLoaded = $state(false);

	async function loadDnsServers() {
		if (dnsServersLoaded) return;
		try {
			dnsServers = await api.listDnsServers();
			dnsServersLoaded = true;
		} catch (e) {
			console.error('Failed to load DNS servers:', e);
		}
	}

	// Check if editing a logical rule
	function isLogicalRule(): boolean {
		return rule?.type === 'logical' && Array.isArray(rule.rules);
	}

	// Extract conditions from a rule (for simple mode)
	function extractConditions(r: RouteRule): RuleConditions {
		return {
			ip_is_private: r.ip_is_private,
			source_ip_is_private: r.source_ip_is_private,
			invert: r.invert,
			inbound: r.inbound,
			domain: r.domain,
			domain_suffix: r.domain_suffix,
			domain_keyword: r.domain_keyword,
			domain_regex: r.domain_regex,
			ip_cidr: r.ip_cidr,
			source_ip_cidr: r.source_ip_cidr,
			port: r.port,
			port_range: r.port_range,
			source_port: r.source_port,
			source_port_range: r.source_port_range,
			protocol: r.protocol,
			rule_set: r.rule_set,
			rule_set_ip_cidr_match_source: r.rule_set_ip_cidr_match_source,
			process_name: r.process_name,
			process_path: r.process_path,
			process_path_regex: r.process_path_regex,
			network: r.network,
			ip_version: r.ip_version,
			clash_mode: r.clash_mode,
			client: r.client,
			auth_user: r.auth_user,
			user: r.user,
			user_id: r.user_id
		};
	}

	// Initialize conditions from existing rule
	function initConditions(): RuleConditions {
		if (!rule || isLogicalRule()) return {};
		return extractConditions(rule);
	}

	// Initialize combined conditions from logical rule
	function initCombinedConditions(): RuleConditions[] {
		if (!rule || !isLogicalRule()) return [{}];
		return rule.rules?.map(r => extractConditions(r)) ?? [{}];
	}

	// Form state
	let action = $state<RouteRule['action']>(rule?.action ?? 'route');
	let outbound = $state(rule?.outbound ?? '');
	let conditions = $state<RuleConditions>(initConditions());

	// Rule mode: simple (single conditions) vs combined (logical AND/OR)
	let ruleMode = $state<'simple' | 'combined'>(isLogicalRule() ? 'combined' : 'simple');
	let logicalMode = $state<'and' | 'or'>(rule?.mode ?? 'and');
	let combinedConditions = $state<RuleConditions[]>(initCombinedConditions());

	// Sniff options
	let sniffTimeout = $state(rule?.timeout ?? '300ms');

	// Reject options
	let rejectMethod = $state<'default' | 'drop'>(rule?.method ?? 'default');
	let rejectNoDrop = $state(rule?.no_drop ?? false);

	// Resolve options
	let resolveServer = $state(rule?.server ?? '');
	let resolveStrategy = $state(rule?.strategy ?? '');

	// Route options
	let overrideAddress = $state(rule?.override_address ?? '');
	let overridePort = $state<number | undefined>(rule?.override_port);
	let networkStrategy = $state(rule?.network_strategy ?? '');
	let udpConnect = $state(rule?.udp_connect ?? false);
	let udpTimeout = $state(rule?.udp_timeout ?? '');
	let udpDisableDomainUnmapping = $state(rule?.udp_disable_domain_unmapping ?? false);
	let tlsFragment = $state<TlsFragment>(rule?.tls_fragment ?? {});
	let tlsRecordFragment = $state<TlsRecordFragment>(rule?.tls_record_fragment ?? {});

	let errors = $state<Record<string, string>>({});

	// Actions that show conditions form
	let showConditions = $derived(
		action === 'route' || action === 'reject' || action === 'route-options' || action === 'resolve'
	);

	function hasAnyCondition(c: RuleConditions): boolean {
		return !!(
			c.ip_is_private || c.source_ip_is_private ||
			c.inbound?.length ||
			c.domain?.length || c.domain_suffix?.length || c.domain_keyword?.length || c.domain_regex?.length ||
			c.ip_cidr?.length || c.source_ip_cidr?.length ||
			c.port?.length || c.port_range?.length || c.source_port?.length || c.source_port_range?.length ||
			c.protocol?.length || c.rule_set?.length ||
			c.process_name?.length || c.process_path?.length || c.process_path_regex?.length ||
			c.network || c.ip_version !== undefined || c.clash_mode ||
			c.client?.length || c.auth_user?.length || c.user?.length || c.user_id?.length
		);
	}

	function validate(): boolean {
		errors = {};

		if (action === 'route' && !outbound) {
			errors['outbound'] = $t('routes.outboundRequired');
		}

		if (showConditions) {
			if (ruleMode === 'simple') {
				if (!hasAnyCondition(conditions)) {
					errors['conditions'] = $t('routes.conditionRequired');
				}
			} else {
				// Combined mode: need at least 2 conditions, each with content
				if (combinedConditions.length < 2) {
					errors['conditions'] = $t('routes.combiner.needTwoConditions');
				} else {
					const emptyIndex = combinedConditions.findIndex(c => !hasAnyCondition(c));
					if (emptyIndex !== -1) {
						errors['conditions'] = $t('routes.combiner.emptyCondition', { values: { number: emptyIndex + 1 } });
					}
				}
			}
		}

		const errorKeys = Object.keys(errors);
		if (errorKeys.length > 0) {
			notifications.error(errors[errorKeys[0]]);
			return false;
		}

		return true;
	}

	function handleSubmit() {
		if (!validate()) return;

		const newRule: RouteRule = {};

		// Action
		if (action !== 'route') newRule.action = action;

		// Outbound (route action only)
		if (action === 'route' && outbound) newRule.outbound = outbound;

		// Sniff-specific
		if (action === 'sniff' && sniffTimeout && sniffTimeout !== '300ms') {
			newRule.timeout = sniffTimeout;
		}

		// Reject-specific
		if (action === 'reject') {
			if (rejectMethod !== 'default') newRule.method = rejectMethod;
			if (rejectNoDrop) newRule.no_drop = true;
		}

		// Resolve-specific
		if (action === 'resolve') {
			if (resolveServer) newRule.server = resolveServer;
			if (resolveStrategy) newRule.strategy = resolveStrategy as RouteRule['strategy'];
		}

		// Route options (for route and route-options)
		if (action === 'route' || action === 'route-options') {
			if (overrideAddress.trim()) newRule.override_address = overrideAddress.trim();
			if (overridePort && overridePort > 0) newRule.override_port = overridePort;
			if (networkStrategy) newRule.network_strategy = networkStrategy;
			if (udpConnect) newRule.udp_connect = true;
			if (udpTimeout.trim()) newRule.udp_timeout = udpTimeout.trim();
			if (udpDisableDomainUnmapping) newRule.udp_disable_domain_unmapping = true;
			if (tlsFragment?.enabled) newRule.tls_fragment = tlsFragment;
			if (tlsRecordFragment?.enabled) newRule.tls_record_fragment = tlsRecordFragment;
		}

		// Conditions
		if (showConditions) {
			if (ruleMode === 'simple') {
				Object.assign(newRule, conditions);
			} else {
				// Combined mode: output logical rule
				newRule.type = 'logical';
				newRule.mode = logicalMode;
				newRule.rules = combinedConditions.map(c => ({ ...c }));
			}
		}

		// Hijack-dns: add protocol: dns
		if (action === 'hijack-dns') {
			newRule.protocol = ['dns'];
		}

		onSave(newRule);
	}

	// Handle rule set creation from ConditionsForm
	function handleCreateRuleSet() {
		showInlineRuleSetForm = true;
	}

	// Load DNS servers when resolve action is selected
	$effect(() => {
		if (action === 'resolve') loadDnsServers();
	});
</script>

<form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="space-y-5">
	<!-- Action selector -->
	<ActionSelector bind:action />

	<!-- Outbound (route action only) -->
	{#if action === 'route'}
		<OutboundSelector bind:outbound {outbounds} error={errors['outbound']} />
	{/if}

	<!-- Action-specific options -->
	{#if action === 'reject'}
		<RejectOptions bind:method={rejectMethod} bind:noDrop={rejectNoDrop} />
	{/if}

	{#if action === 'resolve'}
		<ResolveOptions bind:server={resolveServer} bind:strategy={resolveStrategy} {dnsServers} />
	{/if}

	{#if action === 'route-options'}
		<RouteOptions
			bind:overrideAddress
			bind:overridePort
			bind:networkStrategy
			bind:udpConnect
			bind:udpTimeout
			bind:udpDisableDomainUnmapping
			bind:tlsFragment
			bind:tlsRecordFragment
			showDescription
		/>
	{/if}

	{#if action === 'sniff'}
		<SniffOptions bind:timeout={sniffTimeout} />
	{/if}

	{#if action === 'hijack-dns'}
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4">
			<p class="text-sm text-[var(--ctp-overlay1)]">{$t('routes.hijackDnsDescription')}</p>
		</div>
	{/if}

	<!-- Route options (collapsible, for route action) -->
	{#if action === 'route'}
		<RouteOptions
			bind:overrideAddress
			bind:overridePort
			bind:networkStrategy
			bind:udpConnect
			bind:udpTimeout
			bind:udpDisableDomainUnmapping
			bind:tlsFragment
			bind:tlsRecordFragment
			collapsible
		/>
	{/if}

	<!-- Conditions -->
	{#if showConditions}
		<!-- Simple/Combined mode toggle -->
		<div class="flex items-center gap-3 pb-2">
			<span class="text-sm text-[var(--ctp-subtext1)]">{$t('routes.combiner.ruleType')}:</span>
			<div class="flex rounded-lg overflow-hidden border border-[var(--ctp-surface2)]">
				<button type="button" onclick={() => ruleMode = 'simple'}
					class="px-3 py-1.5 text-sm font-medium transition-colors {ruleMode === 'simple'
						? 'bg-[var(--ctp-primary)] text-white'
						: 'bg-[var(--ctp-surface0)] text-[var(--ctp-text)] hover:bg-[var(--ctp-surface1)]'}">
					{$t('routes.combiner.simple')}
				</button>
				<button type="button" onclick={() => ruleMode = 'combined'}
					class="px-3 py-1.5 text-sm font-medium transition-colors {ruleMode === 'combined'
						? 'bg-[var(--ctp-primary)] text-white'
						: 'bg-[var(--ctp-surface0)] text-[var(--ctp-text)] hover:bg-[var(--ctp-surface1)]'}">
					{$t('routes.combiner.combined')}
				</button>
			</div>
		</div>

		{#if errors['conditions']}
			<p class="text-sm text-[var(--ctp-red)]">{errors['conditions']}</p>
		{/if}

		{#if ruleMode === 'simple'}
			<ConditionsForm
				bind:conditions
				{ruleSets}
				{inbounds}
				{hiddenRuleSets}
				onCreateRuleSet={handleCreateRuleSet}
			/>
		{:else}
			<RuleCombiner
				bind:conditions={combinedConditions}
				bind:mode={logicalMode}
				{ruleSets}
				{inbounds}
				{hiddenRuleSets}
				onCreateRuleSet={handleCreateRuleSet}
			/>
		{/if}
	{/if}

	<!-- Form actions -->
	<div class="flex justify-end gap-3 pt-4 border-t border-[var(--ctp-surface2)]">
		<button type="button" onclick={onCancel}
			class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors">
			{$t('common.cancel')}
		</button>
		<button type="submit"
			class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity">
			{rule ? $t('common.saveChanges') : $t('routes.addRule')}
		</button>
	</div>
</form>

<!-- Inline Rule Set Creation Modal -->
{#if showInlineRuleSetForm}
	<div class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-[60] p-4">
		<div class="bg-[var(--ctp-base)] rounded-xl w-full max-w-lg max-h-[90vh] overflow-y-auto">
			<div class="px-4 py-3 border-b border-[var(--ctp-surface2)] flex items-center justify-between">
				<h2 class="text-lg font-medium text-[var(--ctp-text)]">{$t('ruleSets.newRuleSet')}</h2>
				<button onclick={() => showInlineRuleSetForm = false}
					class="p-1 rounded-md hover:bg-[var(--ctp-surface1)] text-[var(--ctp-overlay1)]" aria-label="Close">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>
			<div class="p-4">
				<RuleSetForm
					existingTags={ruleSets.map(rs => rs.tag)}
					{outbounds}
					onSave={async (newRuleSet) => {
						try {
							await api.createRuleSet(newRuleSet);
							conditions = { ...conditions, rule_set: [...(conditions.rule_set ?? []), newRuleSet.tag] };
							showInlineRuleSetForm = false;
							onRuleSetCreated?.(newRuleSet);
							notifications.success($t('ruleSets.ruleSetCreated'));
						} catch (e) {
							notifications.error(`${e}`);
						}
					}}
					onCancel={() => showInlineRuleSetForm = false}
				/>
			</div>
		</div>
	</div>
{/if}
