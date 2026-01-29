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
		RouteOptions
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

	// Initialize conditions from existing rule
	function initConditions(): RuleConditions {
		if (!rule) return {};
		return {
			ip_is_private: rule.ip_is_private,
			source_ip_is_private: rule.source_ip_is_private,
			invert: rule.invert,
			inbound: rule.inbound,
			domain: rule.domain,
			domain_suffix: rule.domain_suffix,
			domain_keyword: rule.domain_keyword,
			domain_regex: rule.domain_regex,
			ip_cidr: rule.ip_cidr,
			source_ip_cidr: rule.source_ip_cidr,
			port: rule.port,
			port_range: rule.port_range,
			source_port: rule.source_port,
			source_port_range: rule.source_port_range,
			protocol: rule.protocol,
			rule_set: rule.rule_set,
			rule_set_ip_cidr_match_source: rule.rule_set_ip_cidr_match_source,
			process_name: rule.process_name,
			process_path: rule.process_path,
			process_path_regex: rule.process_path_regex,
			network: rule.network,
			ip_version: rule.ip_version,
			clash_mode: rule.clash_mode,
			client: rule.client,
			auth_user: rule.auth_user,
			user: rule.user,
			user_id: rule.user_id
		};
	}

	// Form state
	let action = $state<RouteRule['action']>(rule?.action ?? 'route');
	let outbound = $state(rule?.outbound ?? '');
	let conditions = $state<RuleConditions>(initConditions());

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

		if (showConditions && !hasAnyCondition(conditions)) {
			errors['conditions'] = $t('routes.conditionRequired');
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
			Object.assign(newRule, conditions);
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
		{#if errors['conditions']}
			<p class="text-sm text-[var(--ctp-red)]">{errors['conditions']}</p>
		{/if}
		<ConditionsForm
			bind:conditions
			{ruleSets}
			{inbounds}
			{hiddenRuleSets}
			onCreateRuleSet={handleCreateRuleSet}
		/>
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
