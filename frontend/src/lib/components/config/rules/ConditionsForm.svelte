<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { RuleConditions, RuleSet, Inbound } from '$lib/types';
	import { DestinationTab, SourceTab, AdvancedTab } from './conditions';

	interface Props {
		conditions: RuleConditions;
		ruleSets: RuleSet[];
		inbounds?: Inbound[];
		hiddenRuleSets?: Set<string>;
		onCreateRuleSet?: () => void;
	}

	let {
		conditions = $bindable(),
		ruleSets,
		inbounds = [],
		hiddenRuleSets,
		onCreateRuleSet
	}: Props = $props();

	// Tabs
	type Tab = 'destination' | 'source' | 'advanced';
	let activeTab = $state<Tab>('destination');

	let tabs = $derived([
		{ id: 'destination' as Tab, label: $t('routes.destination') },
		{ id: 'source' as Tab, label: $t('routes.source') },
		{ id: 'advanced' as Tab, label: $t('common.advanced') }
	]);

	// Filter rule sets
	let availableRuleSets = $derived(
		hiddenRuleSets
			? ruleSets.filter(rs => !hiddenRuleSets.has(rs.tag))
			: ruleSets
	);

	// String state for form fields (synced with conditions)
	// Destination tab
	let ipIsPrivate = $state(conditions.ip_is_private ?? false);
	let invert = $state(conditions.invert ?? false);
	let domainSuffix = $state(conditions.domain_suffix?.join('\n') ?? '');
	let ipCidr = $state(conditions.ip_cidr?.join('\n') ?? '');
	let ports = $state(conditions.port?.join(', ') ?? '');
	let network = $state<'tcp' | 'udp' | 'icmp' | ''>(conditions.network ?? '');
	let ipVersion = $state<number | undefined>(conditions.ip_version);
	let clashMode = $state(conditions.clash_mode ?? '');

	// Source tab
	let sourceIpIsPrivate = $state(conditions.source_ip_is_private ?? false);
	let selectedInbounds = $state<string[]>(conditions.inbound ?? []);
	let sourceIpCidr = $state(conditions.source_ip_cidr?.join('\n') ?? '');
	let sourcePorts = $state(conditions.source_port?.join(', ') ?? '');
	let sourcePortRange = $state(conditions.source_port_range?.join('\n') ?? '');
	let authUser = $state(conditions.auth_user?.join('\n') ?? '');

	// Advanced tab
	let selectedRuleSets = $state<string[]>(conditions.rule_set ?? []);
	let ruleSetIpCidrMatchSource = $state(conditions.rule_set_ip_cidr_match_source ?? false);
	let protocol = $state(Array.isArray(conditions.protocol) ? conditions.protocol.join(', ') : '');
	let processName = $state(conditions.process_name?.join('\n') ?? '');
	let client = $state(conditions.client?.join(', ') ?? '');
	let domain = $state(conditions.domain?.join('\n') ?? '');
	let domainKeyword = $state(conditions.domain_keyword?.join('\n') ?? '');
	let domainRegex = $state(conditions.domain_regex?.join('\n') ?? '');
	let portRange = $state(conditions.port_range?.join('\n') ?? '');
	let processPath = $state(conditions.process_path?.join('\n') ?? '');
	let processPathRegex = $state(conditions.process_path_regex?.join('\n') ?? '');
	let user = $state(conditions.user?.join('\n') ?? '');
	let userId = $state(conditions.user_id?.join(', ') ?? '');

	// Helper functions
	function parseLines(text: string): string[] {
		return text.split('\n').map(s => s.trim()).filter(s => s);
	}

	function parsePorts(text: string): number[] {
		return text.split(',').map(s => parseInt(s.trim())).filter(n => !isNaN(n) && n > 0 && n <= 65535);
	}

	function parseIntArray(text: string): number[] {
		return text.split(',').map(s => parseInt(s.trim())).filter(n => !isNaN(n));
	}

	function parseCommaSeparated(text: string): string[] {
		return text.split(',').map(s => s.trim()).filter(Boolean);
	}

	// Sync form state to conditions object
	$effect(() => {
		const newConditions: RuleConditions = {};

		// Destination
		if (ipIsPrivate) newConditions.ip_is_private = true;
		if (invert) newConditions.invert = true;
		if (domainSuffix.trim()) newConditions.domain_suffix = parseLines(domainSuffix);
		if (ipCidr.trim()) newConditions.ip_cidr = parseLines(ipCidr);
		if (ports.trim()) newConditions.port = parsePorts(ports);
		if (network) newConditions.network = network;
		if (ipVersion !== undefined) newConditions.ip_version = ipVersion;
		if (clashMode.trim()) newConditions.clash_mode = clashMode.trim();

		// Source
		if (sourceIpIsPrivate) newConditions.source_ip_is_private = true;
		if (selectedInbounds.length > 0) newConditions.inbound = selectedInbounds;
		if (sourceIpCidr.trim()) newConditions.source_ip_cidr = parseLines(sourceIpCidr);
		if (sourcePorts.trim()) newConditions.source_port = parsePorts(sourcePorts);
		if (sourcePortRange.trim()) newConditions.source_port_range = parseLines(sourcePortRange);
		if (authUser.trim()) newConditions.auth_user = parseLines(authUser);

		// Advanced
		if (selectedRuleSets.length > 0) {
			newConditions.rule_set = selectedRuleSets;
			if (ruleSetIpCidrMatchSource) newConditions.rule_set_ip_cidr_match_source = true;
		}
		if (protocol.trim()) newConditions.protocol = parseCommaSeparated(protocol);
		if (processName.trim()) newConditions.process_name = parseLines(processName);
		if (client.trim()) newConditions.client = parseCommaSeparated(client);
		if (domain.trim()) newConditions.domain = parseLines(domain);
		if (domainKeyword.trim()) newConditions.domain_keyword = parseLines(domainKeyword);
		if (domainRegex.trim()) newConditions.domain_regex = parseLines(domainRegex);
		if (portRange.trim()) newConditions.port_range = parseLines(portRange);
		if (processPath.trim()) newConditions.process_path = parseLines(processPath);
		if (processPathRegex.trim()) newConditions.process_path_regex = parseLines(processPathRegex);
		if (user.trim()) newConditions.user = parseLines(user);
		if (userId.trim()) newConditions.user_id = parseIntArray(userId);

		conditions = newConditions;
	});
</script>

<!-- Tabs -->
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

<!-- Tab Content -->
<div class="min-h-[200px] pt-4">
	{#if activeTab === 'destination'}
		<DestinationTab
			bind:ipIsPrivate
			bind:invert
			bind:domainSuffix
			bind:ipCidr
			bind:ports
			bind:network
			bind:ipVersion
			bind:clashMode
		/>
	{:else if activeTab === 'source'}
		<SourceTab
			bind:sourceIpIsPrivate
			{inbounds}
			bind:selectedInbounds
			bind:sourceIpCidr
			bind:sourcePorts
			bind:sourcePortRange
			bind:authUser
		/>
	{:else if activeTab === 'advanced'}
		<AdvancedTab
			{availableRuleSets}
			bind:selectedRuleSets
			bind:ruleSetIpCidrMatchSource
			{hiddenRuleSets}
			bind:protocol
			bind:processName
			bind:client
			bind:domain
			bind:domainKeyword
			bind:domainRegex
			bind:portRange
			bind:processPath
			bind:processPathRegex
			bind:user
			bind:userId
			{onCreateRuleSet}
		/>
	{/if}
</div>
