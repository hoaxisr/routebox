<script lang="ts">
	import type { RouteRule, RuleSet, Outbound, Inbound, DnsServer, TlsFragment, TlsRecordFragment } from '$lib/types';
	import { notifications, featureFlags } from '$lib/stores';
	import { api } from '$lib/api/client';
	import { t } from 'svelte-i18n';
	import RuleSetForm from './RuleSetForm.svelte';
	import TlsFragmentForm from './TlsFragmentForm.svelte';
	import HelpTooltip from '$lib/components/shared/HelpTooltip.svelte';

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

	// Filter out rule sets that already have simple routes (unless editing)
	let availableRuleSets = $derived(
		hiddenRuleSets
			? ruleSets.filter(rs => !hiddenRuleSets.has(rs.tag))
			: ruleSets
	);

	// Form state - initialize from existing rule or defaults
	let ipIsPrivate = $state(rule?.ip_is_private ?? false);
	let sourceIpIsPrivate = $state(rule?.source_ip_is_private ?? false);
	let invert = $state(rule?.invert ?? false);
	// Inbound filter
	let selectedInbounds = $state<string[]>(rule?.inbound ?? []);
	// Domain conditions
	let domain = $state(rule?.domain?.join('\n') ?? '');
	let domainSuffix = $state(rule?.domain_suffix?.join('\n') ?? '');
	let domainKeyword = $state(rule?.domain_keyword?.join('\n') ?? '');
	let domainRegex = $state(rule?.domain_regex?.join('\n') ?? '');
	// IP conditions
	let ipCidr = $state(rule?.ip_cidr?.join('\n') ?? '');
	let sourceIpCidr = $state(rule?.source_ip_cidr?.join('\n') ?? '');
	// Port conditions
	let ports = $state(rule?.port?.join(', ') ?? '');
	let portRange = $state(rule?.port_range?.join('\n') ?? '');
	let sourcePorts = $state(rule?.source_port?.join(', ') ?? '');
	let sourcePortRange = $state(rule?.source_port_range?.join('\n') ?? '');
	// Protocol (may be string or array from API)
	let protocol = $state(Array.isArray(rule?.protocol) ? rule.protocol.join(', ') : (rule?.protocol ?? ''));
	// Rule sets
	let selectedRuleSets = $state<string[]>(rule?.rule_set ?? []);
	// Process
	let processName = $state(rule?.process_name?.join('\n') ?? '');
	let processPath = $state(rule?.process_path?.join('\n') ?? '');
	let processPathRegex = $state(rule?.process_path_regex?.join('\n') ?? '');
	// Network
	let network = $state<'tcp' | 'udp' | 'icmp' | ''>(rule?.network ?? '');
	// New matching conditions
	let ipVersion = $state<number | undefined>(rule?.ip_version);
	let clashMode = $state(rule?.clash_mode ?? '');
	let ruleSetIpCidrMatchSource = $state(rule?.rule_set_ip_cidr_match_source ?? false);
	let client = $state(rule?.client?.join(', ') ?? '');
	let authUser = $state(rule?.auth_user?.join('\n') ?? '');
	let user = $state(rule?.user?.join('\n') ?? '');
	let userId = $state(rule?.user_id?.join(', ') ?? '');

	// Action
	let outbound = $state(rule?.outbound ?? '');
	let action = $state<RouteRule['action']>(rule?.action ?? 'route');

	// Sniff-specific options
	let sniffTimeout = $state(rule?.timeout ?? '300ms');

	// Reject-specific options
	let rejectMethod = $state<'default' | 'drop'>(rule?.method ?? 'default');
	let rejectNoDrop = $state(rule?.no_drop ?? false);

	// Resolve-specific options
	let resolveServer = $state(rule?.server ?? '');
	let resolveStrategy = $state(rule?.strategy ?? '');

	// Route-options fields (for route and route-options actions)
	let overrideAddress = $state(rule?.override_address ?? '');
	let overridePort = $state<number | undefined>(rule?.override_port);
	let networkStrategy = $state(rule?.network_strategy ?? '');
	let fallbackDelay = $state(rule?.fallback_delay ?? '');
	let udpConnect = $state(rule?.udp_connect ?? false);
	let udpTimeout = $state(rule?.udp_timeout ?? '');
	let udpDisableDomainUnmapping = $state(rule?.udp_disable_domain_unmapping ?? false);

	// TLS Fragment (≥1.12)
	let tlsFragment = $state<TlsFragment>(rule?.tls_fragment ?? {});
	let tlsRecordFragment = $state<TlsRecordFragment>(rule?.tls_record_fragment ?? {});

	// Show route options collapsible
	let showRouteOptions = $state(false);

	// Tabs
	type Tab = 'source' | 'destination' | 'advanced';
	let activeTab = $state<Tab>('destination');

	// Actions that support condition tabs (not just route/reject)
	let showConditionTabs = $derived(
		action === 'route' || action === 'reject' || action === 'route-options' || action === 'resolve'
	);

	let errors = $state<Record<string, string>>({});

	function toggleInbound(tag: string) {
		if (selectedInbounds.includes(tag)) {
			selectedInbounds = selectedInbounds.filter((t) => t !== tag);
		} else {
			selectedInbounds = [...selectedInbounds, tag];
		}
	}

	function toggleRuleSet(tag: string) {
		if (selectedRuleSets.includes(tag)) {
			selectedRuleSets = selectedRuleSets.filter((t) => t !== tag);
		} else {
			selectedRuleSets = [...selectedRuleSets, tag];
		}
	}

	function parseLines(text: string): string[] {
		return text.split('\n').map(s => s.trim()).filter(s => s);
	}

	function parsePorts(text: string): number[] {
		return text.split(',').map(s => parseInt(s.trim())).filter(n => !isNaN(n) && n > 0 && n <= 65535);
	}

	function parseIntArray(text: string): number[] {
		return text.split(',').map(s => parseInt(s.trim())).filter(n => !isNaN(n));
	}

	function validate(): boolean {
		errors = {};

		// Outbound required only for 'route' action
		if (action === 'route' && !outbound) {
			errors['outbound'] = $t('routes.outboundRequired');
		}

		// Check that at least one condition is set (for actions with conditions)
		if (showConditionTabs) {
			const hasCondition = ipIsPrivate || sourceIpIsPrivate ||
				selectedInbounds.length > 0 ||
				domain.trim() || domainSuffix.trim() || domainKeyword.trim() || domainRegex.trim() ||
				ipCidr.trim() || sourceIpCidr.trim() ||
				ports.trim() || portRange.trim() || sourcePorts.trim() || sourcePortRange.trim() ||
				protocol.trim() || selectedRuleSets.length > 0 ||
				processName.trim() || processPath.trim() || processPathRegex.trim() ||
				network || ipVersion !== undefined || clashMode.trim() || client.trim() ||
				authUser.trim() || user.trim() || userId.trim();

			if (!hasCondition) {
				errors['conditions'] = $t('routes.conditionRequired');
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

		// Outbound only for route action
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

		// Route-options fields (for route and route-options)
		if (action === 'route' || action === 'route-options') {
			if (overrideAddress.trim()) newRule.override_address = overrideAddress.trim();
			if (overridePort && overridePort > 0) newRule.override_port = overridePort;
			if (networkStrategy) newRule.network_strategy = networkStrategy;
			if (fallbackDelay.trim()) newRule.fallback_delay = fallbackDelay.trim();
			if (udpConnect) newRule.udp_connect = true;
			if (udpTimeout.trim()) newRule.udp_timeout = udpTimeout.trim();
			if (udpDisableDomainUnmapping) newRule.udp_disable_domain_unmapping = true;
			// TLS Fragment
			if (tlsFragment?.enabled) newRule.tls_fragment = tlsFragment;
			if (tlsRecordFragment?.enabled) newRule.tls_record_fragment = tlsRecordFragment;
		}

		// Conditions (for actions with condition tabs)
		if (showConditionTabs) {
			if (invert) newRule.invert = true;
			if (selectedInbounds.length > 0) newRule.inbound = selectedInbounds;
			if (ipIsPrivate) newRule.ip_is_private = true;
			if (sourceIpIsPrivate) newRule.source_ip_is_private = true;
			if (domain.trim()) newRule.domain = parseLines(domain);
			if (domainSuffix.trim()) newRule.domain_suffix = parseLines(domainSuffix);
			if (domainKeyword.trim()) newRule.domain_keyword = parseLines(domainKeyword);
			if (domainRegex.trim()) newRule.domain_regex = parseLines(domainRegex);
			if (ipCidr.trim()) newRule.ip_cidr = parseLines(ipCidr);
			if (sourceIpCidr.trim()) newRule.source_ip_cidr = parseLines(sourceIpCidr);
			if (ports.trim()) newRule.port = parsePorts(ports);
			if (portRange.trim()) newRule.port_range = parseLines(portRange);
			if (sourcePorts.trim()) newRule.source_port = parsePorts(sourcePorts);
			if (sourcePortRange.trim()) newRule.source_port_range = parseLines(sourcePortRange);
			if (protocol.trim()) newRule.protocol = protocol.split(',').map(s => s.trim()).filter(Boolean);
			if (selectedRuleSets.length > 0) {
				newRule.rule_set = selectedRuleSets;
				if (ruleSetIpCidrMatchSource) newRule.rule_set_ip_cidr_match_source = true;
			}
			if (processName.trim()) newRule.process_name = parseLines(processName);
			if (processPath.trim()) newRule.process_path = parseLines(processPath);
			if (processPathRegex.trim()) newRule.process_path_regex = parseLines(processPathRegex);
			if (network) newRule.network = network;
			if (ipVersion !== undefined) newRule.ip_version = ipVersion;
			if (clashMode.trim()) newRule.clash_mode = clashMode.trim();
			if (client.trim()) newRule.client = client.split(',').map(s => s.trim()).filter(Boolean);
			if (authUser.trim()) newRule.auth_user = parseLines(authUser);
			if (user.trim()) newRule.user = parseLines(user);
			if (userId.trim()) newRule.user_id = parseIntArray(userId);
		}

		// For hijack-dns, add protocol: dns
		if (action === 'hijack-dns') {
			newRule.protocol = ['dns'];
		}

		onSave(newRule);
	}

	let tabs = $derived([
		{ id: 'destination' as Tab, label: $t('routes.destination'), desc: $t('routes.destinationDesc') },
		{ id: 'source' as Tab, label: $t('routes.source'), desc: $t('routes.sourceDesc') },
		{ id: 'advanced' as Tab, label: $t('common.advanced'), desc: $t('routes.advancedDesc') }
	]);

	// Load DNS servers when resolve action is selected
	$effect(() => {
		if (action === 'resolve') loadDnsServers();
	});
</script>

<form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="space-y-5">
	<!-- Action selection at top -->
	<div>
		<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('routes.ruleAction')}</label>
		<div class="flex flex-wrap gap-2">
			<button type="button" onclick={() => action = 'route'}
				class="toggle-btn {action === 'route' ? 'selected' : ''}">
				{$t('routes.actionRoute')}
			</button>
			<button type="button" onclick={() => action = 'reject'}
				class="toggle-btn-danger {action === 'reject' ? 'selected' : ''}">
				{$t('routes.actionReject')}
			</button>
			<button type="button" onclick={() => action = 'route-options'}
				class="toggle-btn {action === 'route-options' ? 'selected' : ''}">
				{$t('routes.actionRouteOptions')}
			</button>
			<button type="button" onclick={() => action = 'resolve'}
				class="toggle-btn {action === 'resolve' ? 'selected' : ''}">
				{$t('routes.actionResolve')}
			</button>
			<button type="button" onclick={() => action = 'sniff'}
				class="toggle-btn {action === 'sniff' ? 'selected' : ''}">
				{$t('routes.actionSniff')}
			</button>
			<button type="button" onclick={() => action = 'hijack-dns'}
				class="toggle-btn {action === 'hijack-dns' ? 'selected' : ''}">
				{$t('routes.actionHijackDns')}
			</button>
		</div>
		<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">
			{#if action === 'route'}{$t('routes.actionRouteDesc')}
			{:else if action === 'reject'}{$t('routes.actionRejectDesc')}
			{:else if action === 'route-options'}{$t('routes.actionRouteOptionsDesc')}
			{:else if action === 'resolve'}{$t('routes.actionResolveDesc')}
			{:else if action === 'sniff'}{$t('routes.actionSniffDesc')}
			{:else if action === 'hijack-dns'}{$t('routes.actionHijackDnsDesc')}
			{/if}
		</p>
	</div>

	<!-- Outbound (only for route action) -->
	{#if action === 'route'}
		<div>
			<label for="outbound" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.routeToOutbound')} *</label>
			<select
				id="outbound"
				bind:value={outbound}
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['outbound'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
			>
				<option value="">{$t('routes.selectOutbound')}</option>
				{#each outbounds as ob}
					<option value={ob.tag}>{ob.tag} ({ob.type})</option>
				{/each}
			</select>
			{#if errors['outbound']}
				<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['outbound']}</p>
			{/if}
		</div>
	{/if}

	<!-- Reject options -->
	{#if action === 'reject'}
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-3">
			<div>
				<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.rejectMethod')}</label>
				<div class="flex gap-2">
					<button type="button" onclick={() => rejectMethod = 'default'}
						class="flex-1 toggle-btn {rejectMethod === 'default' ? 'selected' : ''}">
						{$t('common.default')}
					</button>
					<button type="button" onclick={() => rejectMethod = 'drop'}
						class="flex-1 toggle-btn {rejectMethod === 'drop' ? 'selected' : ''}">
						{$t('routes.rejectDrop')}
					</button>
				</div>
				<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.rejectMethodHint')}</p>
			</div>
			<label class="flex items-center gap-2 cursor-pointer">
				<input type="checkbox" bind:checked={rejectNoDrop}
					class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]" />
				<div>
					<span class="text-sm text-[var(--ctp-text)]">{$t('routes.rejectNoDrop')}</span>
					<p class="text-xs text-[var(--ctp-overlay0)]">{$t('routes.rejectNoDropHint')}</p>
				</div>
			</label>
		</div>
	{/if}

	<!-- Resolve options -->
	{#if action === 'resolve'}
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-3">
			<div>
				<label for="resolve-server" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.resolveServer')}</label>
				<select id="resolve-server" bind:value={resolveServer}
					class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]">
					<option value="">{$t('common.default')}</option>
					{#each dnsServers as server}
						<option value={server.tag}>{server.tag} ({server.type})</option>
					{/each}
				</select>
			</div>
			<div>
				<label for="resolve-strategy" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.resolveStrategy')}</label>
				<select id="resolve-strategy" bind:value={resolveStrategy}
					class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]">
					<option value="">{$t('common.default')}</option>
					<option value="prefer_ipv4">{$t('dns.strategies.preferIpv4')}</option>
					<option value="prefer_ipv6">{$t('dns.strategies.preferIpv6')}</option>
					<option value="ipv4_only">{$t('dns.strategies.ipv4Only')}</option>
					<option value="ipv6_only">{$t('dns.strategies.ipv6Only')}</option>
				</select>
			</div>
		</div>
	{/if}

	<!-- Route-options panel (for route-options action) -->
	{#if action === 'route-options'}
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-3">
			<p class="text-sm text-[var(--ctp-overlay1)]">{$t('routes.routeOptionsDesc')}</p>
			<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
				<div>
					<label for="override-address" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.overrideAddress')}</label>
					<input id="override-address" type="text" bind:value={overrideAddress} placeholder="1.1.1.1"
						class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
				</div>
				<div>
					<label for="override-port" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.overridePort')}</label>
					<input id="override-port" type="number" min="1" max="65535" bind:value={overridePort} placeholder="443"
						class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
				</div>
			</div>
			{#if $featureFlags['network_strategy']}
				<div>
					<label for="ro-network-strategy" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.defaultNetworkStrategy')}</label>
					<select id="ro-network-strategy" bind:value={networkStrategy}
						class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]">
						<option value="">{$t('common.default')}</option>
						<option value="prefer_ipv4">{$t('dns.strategies.preferIpv4')}</option>
						<option value="prefer_ipv6">{$t('dns.strategies.preferIpv6')}</option>
						<option value="ipv4_only">{$t('dns.strategies.ipv4Only')}</option>
						<option value="ipv6_only">{$t('dns.strategies.ipv6Only')}</option>
					</select>
				</div>
			{/if}
			<details class="group">
				<summary class="cursor-pointer text-sm text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)]">
					{$t('routes.udpOptions')}
				</summary>
				<div class="mt-3 space-y-3 pl-4 border-l-2 border-[var(--ctp-surface2)]">
					<label class="flex items-center gap-2 cursor-pointer">
						<input type="checkbox" bind:checked={udpConnect}
							class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]" />
						<span class="text-sm text-[var(--ctp-text)]">{$t('routes.udpConnect')}</span>
					</label>
					<div>
						<label for="udp-timeout" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.udpTimeout')}</label>
						<input id="udp-timeout" type="text" bind:value={udpTimeout} placeholder="5m"
							class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
					</div>
					<label class="flex items-center gap-2 cursor-pointer">
						<input type="checkbox" bind:checked={udpDisableDomainUnmapping}
							class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]" />
						<span class="text-sm text-[var(--ctp-text)]">{$t('routes.udpDisableDomainUnmapping')}</span>
					</label>
				</div>
			</details>
			{#if $featureFlags['tls_fragment']}
				<TlsFragmentForm bind:tlsFragment bind:tlsRecordFragment />
			{/if}
		</div>
	{/if}

	<!-- Sniff options -->
	{#if action === 'sniff'}
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4">
			<p class="text-sm text-[var(--ctp-overlay1)] mb-3">{$t('routes.sniffDescription')}</p>
			<div>
				<label for="sniff-timeout" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.timeout')}</label>
				<input id="sniff-timeout" type="text" bind:value={sniffTimeout} placeholder="300ms"
					class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
				<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.timeoutHint')}</p>
			</div>
		</div>
	{/if}

	<!-- Hijack-DNS info -->
	{#if action === 'hijack-dns'}
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4">
			<p class="text-sm text-[var(--ctp-overlay1)]">{$t('routes.hijackDnsDescription')}</p>
		</div>
	{/if}

	<!-- Route action: collapsible route-options -->
	{#if action === 'route'}
		<details class="group" bind:open={showRouteOptions}>
			<summary class="cursor-pointer text-sm font-medium text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)] flex items-center gap-1">
				<svg class="w-4 h-4 transition-transform {showRouteOptions ? 'rotate-90' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
				</svg>
				{$t('routes.routeOptionsTitle')}
			</summary>
			<div class="mt-3 bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-3">
				<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
					<div>
						<label for="ro-override-address" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.overrideAddress')}</label>
						<input id="ro-override-address" type="text" bind:value={overrideAddress} placeholder="1.1.1.1"
							class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
					</div>
					<div>
						<label for="ro-override-port" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.overridePort')}</label>
						<input id="ro-override-port" type="number" min="1" max="65535" bind:value={overridePort} placeholder="443"
							class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
					</div>
				</div>
				{#if $featureFlags['network_strategy']}
					<div>
						<label for="ro2-network-strategy" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.defaultNetworkStrategy')}</label>
						<select id="ro2-network-strategy" bind:value={networkStrategy}
							class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]">
							<option value="">{$t('common.default')}</option>
							<option value="prefer_ipv4">{$t('dns.strategies.preferIpv4')}</option>
							<option value="prefer_ipv6">{$t('dns.strategies.preferIpv6')}</option>
							<option value="ipv4_only">{$t('dns.strategies.ipv4Only')}</option>
							<option value="ipv6_only">{$t('dns.strategies.ipv6Only')}</option>
						</select>
					</div>
				{/if}
				<details class="group">
					<summary class="cursor-pointer text-sm text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)]">
						{$t('routes.udpOptions')}
					</summary>
					<div class="mt-3 space-y-3 pl-4 border-l-2 border-[var(--ctp-surface2)]">
						<label class="flex items-center gap-2 cursor-pointer">
							<input type="checkbox" bind:checked={udpConnect}
								class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]" />
							<span class="text-sm text-[var(--ctp-text)]">{$t('routes.udpConnect')}</span>
						</label>
						<div>
							<label for="ro2-udp-timeout" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.udpTimeout')}</label>
							<input id="ro2-udp-timeout" type="text" bind:value={udpTimeout} placeholder="5m"
								class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
						</div>
						<label class="flex items-center gap-2 cursor-pointer">
							<input type="checkbox" bind:checked={udpDisableDomainUnmapping}
								class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]" />
							<span class="text-sm text-[var(--ctp-text)]">{$t('routes.udpDisableDomainUnmapping')}</span>
						</label>
					</div>
				</details>
				{#if $featureFlags['tls_fragment']}
					<TlsFragmentForm bind:tlsFragment bind:tlsRecordFragment />
				{/if}
			</div>
		</details>
	{/if}

	<!-- Tabs -->
	{#if showConditionTabs}
		<div class="border-b border-[var(--ctp-surface2)]">
			<div class="flex gap-1">
				{#each tabs as tab}
					<button type="button" onclick={() => activeTab = tab.id}
						class="px-4 py-2 text-sm font-medium border-b-2 transition-colors -mb-px {activeTab === tab.id
							? 'border-[var(--ctp-primary)] text-[var(--ctp-primary)]'
							: 'border-transparent text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)]'}">
						{tab.label}
					</button>
				{/each}
			</div>
		</div>

		{#if errors['conditions']}
			<p class="text-sm text-[var(--ctp-red)]">{errors['conditions']}</p>
		{/if}
	{/if}

	<!-- Tab Content -->
	{#if showConditionTabs}
	<div class="min-h-[200px]">
		{#if activeTab === 'destination'}
			<div class="space-y-4">
				<!-- Toggle options row -->
				<div class="flex flex-wrap gap-4">
					<label class="flex items-center gap-2 p-2 bg-[var(--ctp-surface0)] rounded-lg cursor-pointer hover:bg-[var(--ctp-surface1)] transition-colors">
						<input type="checkbox" bind:checked={ipIsPrivate}
							class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]" />
						<div>
							<span class="text-sm text-[var(--ctp-text)]">{$t('routes.privateIp')}</span>
							<p class="text-xs text-[var(--ctp-overlay0)]">{$t('routes.privateIpHint')}</p>
						</div>
					</label>
					<label class="flex items-center gap-2 p-2 bg-[var(--ctp-surface0)] rounded-lg cursor-pointer hover:bg-[var(--ctp-surface1)] transition-colors">
						<input type="checkbox" bind:checked={invert}
							class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]" />
						<div>
							<span class="text-sm text-[var(--ctp-text)]">{$t('routes.invert')}</span>
							<p class="text-xs text-[var(--ctp-overlay0)]">{$t('routes.invertHint')}</p>
						</div>
					</label>
				</div>

				<!-- Domain Suffix (most common) -->
				<div>
					<label for="domain-suffix" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('routes.domainSuffix')}
						<span class="font-normal text-[var(--ctp-overlay0)]">({$t('routes.mostCommon')})</span>
					</label>
					<textarea id="domain-suffix" bind:value={domainSuffix} rows={3}
						placeholder="google.com&#10;youtube.com&#10;facebook.com"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
					></textarea>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.domainSuffixHint')}</p>
				</div>

				<!-- IP CIDR -->
				<div>
					<label for="ip-cidr" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('routes.ipCidr')}
						<span class="font-normal text-[var(--ctp-overlay0)]">({$t('routes.onePerLine')})</span>
					</label>
					<textarea id="ip-cidr" bind:value={ipCidr} rows={2}
						placeholder="192.168.1.0/24&#10;10.0.0.0/8&#10;8.8.8.8"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
					></textarea>
				</div>

				<!-- Ports + Network + IP Version -->
				<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
					<div>
						<label for="ports" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.port')}</label>
						<input id="ports" type="text" bind:value={ports} placeholder="80, 443, 8080"
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
					</div>
					<div>
						<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.network')}</label>
						<div class="flex gap-2">
							<button type="button" onclick={() => network = ''}
								class="flex-1 toggle-btn {network === '' ? 'selected' : ''}">{$t('routes.networkAny')}</button>
							<button type="button" onclick={() => network = 'tcp'}
								class="flex-1 toggle-btn {network === 'tcp' ? 'selected' : ''}">TCP</button>
							<button type="button" onclick={() => network = 'udp'}
								class="flex-1 toggle-btn {network === 'udp' ? 'selected' : ''}">UDP</button>
						</div>
					</div>
				</div>

				<!-- IP Version -->
				<div>
					<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.ipVersion')}</label>
					<div class="flex gap-2">
						<button type="button" onclick={() => ipVersion = undefined}
							class="flex-1 toggle-btn {ipVersion === undefined ? 'selected' : ''}">{$t('routes.networkAny')}</button>
						<button type="button" onclick={() => ipVersion = 4}
							class="flex-1 toggle-btn {ipVersion === 4 ? 'selected' : ''}">IPv4</button>
						<button type="button" onclick={() => ipVersion = 6}
							class="flex-1 toggle-btn {ipVersion === 6 ? 'selected' : ''}">IPv6</button>
					</div>
				</div>

				<!-- Clash Mode -->
				<div>
					<label for="clash-mode" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.clashMode')}</label>
					<input id="clash-mode" type="text" bind:value={clashMode} placeholder="direct, global, rule"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.clashModeHint')}</p>
				</div>
			</div>
		{:else if activeTab === 'source'}
			<div class="space-y-4">
				<!-- Source IP is Private -->
				<label class="flex items-center gap-2 p-2 bg-[var(--ctp-surface0)] rounded-lg cursor-pointer hover:bg-[var(--ctp-surface1)] transition-colors">
					<input type="checkbox" bind:checked={sourceIpIsPrivate}
						class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]" />
					<div>
						<span class="text-sm text-[var(--ctp-text)]">{$t('routes.sourceIpIsPrivate')}</span>
						<p class="text-xs text-[var(--ctp-overlay0)]">{$t('routes.sourceIpIsPrivateHint')}</p>
					</div>
				</label>

				<!-- Inbound Filter -->
				{#if inbounds.length > 0}
					<div>
						<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('routes.matchFromInbound')}</label>
						<div class="bg-[var(--ctp-surface0)] rounded-lg border border-[var(--ctp-surface2)] divide-y divide-[var(--ctp-surface2)]">
							{#each inbounds as ib}
								<button type="button" onclick={() => toggleInbound(ib.tag)}
									class="w-full px-4 py-3 flex items-center justify-between hover:bg-[var(--ctp-surface1)] transition-colors text-left {selectedInbounds.includes(ib.tag) ? 'bg-[var(--ctp-surface1)]' : ''}">
									<div class="flex items-center gap-2">
										<span class="font-medium text-[var(--ctp-text)]">{ib.tag}</span>
										<span class="selection-chip">{ib.type}</span>
									</div>
									{#if selectedInbounds.includes(ib.tag)}
										<svg class="w-5 h-5 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
										</svg>
									{/if}
								</button>
							{/each}
						</div>
						<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.matchFromInboundHint')}</p>
					</div>
				{/if}

				<!-- Source IP -->
				<div>
					<label for="source-ip-cidr" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('routes.sourceIpCidr')}
						<span class="font-normal text-[var(--ctp-overlay0)]">({$t('routes.clientAddress')})</span>
					</label>
					<textarea id="source-ip-cidr" bind:value={sourceIpCidr} rows={2}
						placeholder="192.168.1.100&#10;10.0.0.0/24"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
					></textarea>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.sourceIpCidrHint')}</p>
				</div>

				<!-- Source Ports -->
				<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
					<div>
						<label for="source-ports" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.sourcePort')}</label>
						<input id="source-ports" type="text" bind:value={sourcePorts} placeholder="1024, 8080"
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
					</div>
					<div>
						<label for="source-port-range" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.sourcePortRange')}</label>
						<input id="source-port-range" type="text" bind:value={sourcePortRange} placeholder="1000:2000"
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
					</div>
				</div>

				<!-- Auth User -->
				<div>
					<label for="auth-user" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('routes.authUser')}
						<span class="font-normal text-[var(--ctp-overlay0)]">({$t('routes.onePerLine')})</span>
					</label>
					<textarea id="auth-user" bind:value={authUser} rows={2} placeholder="admin&#10;user1"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
					></textarea>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.authUserHint')}</p>
				</div>
			</div>
		{:else if activeTab === 'advanced'}
			<div class="space-y-4">
				<!-- Rule Sets -->
				<div>
					<label class="flex items-center gap-1 text-sm font-medium text-[var(--ctp-subtext1)] mb-2">
						{$t('routes.ruleSets')}
						<HelpTooltip text={$t('help.ruleSet')} />
					</label>
					{#if availableRuleSets.length === 0}
						<div class="p-4 bg-[var(--ctp-surface0)] rounded-lg text-center text-[var(--ctp-overlay0)]">
							<p>{$t('routes.noRuleSetsAvailable')}</p>
							<p class="text-xs mt-1">{hiddenRuleSets && hiddenRuleSets.size > 0 ? $t('routes.allRuleSetsHaveRoutes') : $t('routes.addRuleSetsFirst')}</p>
						</div>
					{:else}
						<div class="bg-[var(--ctp-surface0)] rounded-lg border border-[var(--ctp-surface2)] divide-y divide-[var(--ctp-surface2)]">
							{#each availableRuleSets as rs}
								<button type="button" onclick={() => toggleRuleSet(rs.tag)}
									class="w-full px-4 py-3 flex items-center justify-between hover:bg-[var(--ctp-surface1)] transition-colors text-left {selectedRuleSets.includes(rs.tag) ? 'bg-[var(--ctp-surface1)]' : ''}">
									<div>
										<span class="font-medium text-[var(--ctp-text)]">{rs.tag}</span>
										<span class="ml-2 selection-chip">{rs.type}</span>
									</div>
									{#if selectedRuleSets.includes(rs.tag)}
										<svg class="w-5 h-5 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
										</svg>
									{/if}
								</button>
							{/each}
						</div>
						{#if selectedRuleSets.length > 0}
							<label class="flex items-center gap-2 mt-2 cursor-pointer">
								<input type="checkbox" bind:checked={ruleSetIpCidrMatchSource}
									class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]" />
								<span class="text-sm text-[var(--ctp-text)]">{$t('routes.ruleSetIpCidrMatchSource')}</span>
							</label>
						{/if}
					{/if}
					<button type="button" onclick={() => showInlineRuleSetForm = true}
						class="mt-2 px-3 py-1.5 text-sm text-[var(--ctp-primary)] hover:bg-[var(--ctp-surface0)] rounded-lg transition-colors flex items-center gap-1">
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
						</svg>
						{$t('ruleSets.newRuleSet')}
					</button>
				</div>

				<!-- Protocol -->
				<div>
					<label for="protocol" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.protocol')}</label>
					<input id="protocol" type="text" bind:value={protocol} placeholder="http, tls, quic"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.protocolHint')}</p>
				</div>

				<!-- Process -->
				<div>
					<label for="process-name" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('routes.processName')}
						<span class="font-normal text-[var(--ctp-overlay0)]">({$t('routes.onePerLine')})</span>
					</label>
					<textarea id="process-name" bind:value={processName} rows={2}
						placeholder="chrome&#10;firefox&#10;telegram"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
					></textarea>
				</div>

				<!-- Client (feature flag) -->
				{#if $featureFlags['client_sniff']}
					<div>
						<label for="client" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.client')}</label>
						<input id="client" type="text" bind:value={client} placeholder="chromium, firefox"
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
						<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.clientHint')}</p>
					</div>
				{/if}

				<!-- Less common fields (collapsible) -->
				<details class="group">
					<summary class="cursor-pointer text-sm text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)]">
						{$t('routes.moreOptions')}
					</summary>
					<div class="mt-4 space-y-4 pl-4 border-l-2 border-[var(--ctp-surface2)]">
						<!-- Domain Exact Match -->
						<div>
							<label for="domain" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.domainExact')}</label>
							<textarea id="domain" bind:value={domain} rows={2} placeholder="example.com"
								class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
							></textarea>
						</div>

						<!-- Domain Keyword -->
						<div>
							<label for="domain-keyword" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.domainKeyword')}</label>
							<textarea id="domain-keyword" bind:value={domainKeyword} rows={2} placeholder="facebook&#10;twitter"
								class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
							></textarea>
						</div>

						<!-- Domain Regex -->
						<div>
							<label for="domain-regex" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.domainRegex')}</label>
							<textarea id="domain-regex" bind:value={domainRegex} rows={2} placeholder="^.*\.example\.com$"
								class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
							></textarea>
						</div>

						<!-- Port Range -->
						<div>
							<label for="port-range" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.portRange')}</label>
							<input id="port-range" type="text" bind:value={portRange} placeholder="1000:2000"
								class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
						</div>

						<!-- Process Path -->
						<div>
							<label for="process-path" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.processPath')}</label>
							<textarea id="process-path" bind:value={processPath} rows={2} placeholder="/usr/bin/telegram*"
								class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
							></textarea>
						</div>

						<!-- Process Path Regex (feature flag) -->
						{#if $featureFlags['process_path_regex']}
							<div>
								<label for="process-path-regex" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.processPathRegex')}</label>
								<textarea id="process-path-regex" bind:value={processPathRegex} rows={2} placeholder="/usr/bin/telegram.*"
									class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
								></textarea>
							</div>
						{/if}

						<!-- User / User ID (Linux only) -->
						<div>
							<label for="user" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.linuxUser')}</label>
							<textarea id="user" bind:value={user} rows={2} placeholder="nobody&#10;www-data"
								class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
							></textarea>
						</div>
						<div>
							<label for="user-id" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.linuxUserId')}</label>
							<input id="user-id" type="text" bind:value={userId} placeholder="65534, 33"
								class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
						</div>
					</div>
				</details>
			</div>
		{/if}
	</div>
	{/if}

	<!-- Actions -->
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
							selectedRuleSets = [...selectedRuleSets, newRuleSet.tag];
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
