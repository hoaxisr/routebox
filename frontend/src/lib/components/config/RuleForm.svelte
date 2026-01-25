<script lang="ts">
	import type { RouteRule, RuleSet, Outbound, Inbound } from '$lib/types';
	import { notifications } from '$lib/stores';
	import { t } from 'svelte-i18n';

	interface Props {
		rule?: RouteRule;
		ruleSets: RuleSet[];
		outbounds: Outbound[];
		inbounds?: Inbound[];
		hiddenRuleSets?: Set<string>;
		onSave: (rule: RouteRule) => void;
		onCancel: () => void;
	}

	let { rule, ruleSets, outbounds, inbounds = [], hiddenRuleSets, onSave, onCancel }: Props = $props();

	// Filter out rule sets that already have simple routes (unless editing)
	let availableRuleSets = $derived(
		hiddenRuleSets
			? ruleSets.filter(rs => !hiddenRuleSets.has(rs.tag))
			: ruleSets
	);

	// Form state - initialize from existing rule or defaults
	let ipIsPrivate = $state(rule?.ip_is_private ?? false);
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
	// Network
	let network = $state<'tcp' | 'udp' | ''>(rule?.network ?? '');
	// Action
	let outbound = $state(rule?.outbound ?? '');
	let action = $state<'route' | 'reject' | 'sniff' | 'hijack-dns'>(rule?.action ?? 'route');

	// Sniff-specific options
	let sniffTimeout = $state(rule?.timeout ?? '300ms');

	// Tabs (only shown for route/reject)
	type Tab = 'source' | 'destination' | 'advanced';
	let activeTab = $state<Tab>('destination');

	// Whether to show condition tabs (sniff/hijack-dns don't need them typically)
	let showConditionTabs = $derived(action === 'route' || action === 'reject');

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

	function validate(): boolean {
		errors = {};

		// Outbound required only for 'route' action
		if (action === 'route' && !outbound) {
			errors['outbound'] = $t('routes.outboundRequired');
		}

		// Check that at least one condition is set (only for route/reject actions)
		// sniff and hijack-dns typically work without conditions (applies to all traffic)
		if (action === 'route' || action === 'reject') {
			const hasCondition = ipIsPrivate ||
				selectedInbounds.length > 0 ||
				domain.trim() ||
				domainSuffix.trim() ||
				domainKeyword.trim() ||
				domainRegex.trim() ||
				ipCidr.trim() ||
				sourceIpCidr.trim() ||
				ports.trim() ||
				portRange.trim() ||
				sourcePorts.trim() ||
				sourcePortRange.trim() ||
				protocol.trim() ||
				selectedRuleSets.length > 0 ||
				processName.trim() ||
				processPath.trim() ||
				network;

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

		// Action - always set if not default 'route'
		if (action !== 'route') newRule.action = action;

		// Outbound only for route action
		if (action === 'route' && outbound) newRule.outbound = outbound;

		// Sniff-specific: timeout
		if (action === 'sniff' && sniffTimeout && sniffTimeout !== '300ms') {
			newRule.timeout = sniffTimeout;
		}

		// For route/reject, add all conditions
		if (action === 'route' || action === 'reject') {
			if (invert) newRule.invert = true;
			if (selectedInbounds.length > 0) newRule.inbound = selectedInbounds;
			if (ipIsPrivate) newRule.ip_is_private = true;
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
			if (selectedRuleSets.length > 0) newRule.rule_set = selectedRuleSets;
			if (processName.trim()) newRule.process_name = parseLines(processName);
			if (processPath.trim()) newRule.process_path = parseLines(processPath);
			if (network) newRule.network = network;
		}

		// For hijack-dns, add protocol: dns if not set (typical usage)
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
</script>

<form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="space-y-5">
	<!-- Action selection at top -->
	<div>
		<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('routes.ruleAction')}</label>
		<div class="flex flex-wrap gap-2">
			<button
				type="button"
				onclick={() => action = 'route'}
				class="toggle-btn {action === 'route' ? 'selected' : ''}"
			>
				{$t('routes.actionRoute')}
			</button>
			<button
				type="button"
				onclick={() => action = 'reject'}
				class="toggle-btn-danger {action === 'reject' ? 'selected' : ''}"
			>
				{$t('routes.actionReject')}
			</button>
			<button
				type="button"
				onclick={() => action = 'sniff'}
				class="toggle-btn {action === 'sniff' ? 'selected' : ''}"
			>
				{$t('routes.actionSniff')}
			</button>
			<button
				type="button"
				onclick={() => action = 'hijack-dns'}
				class="toggle-btn {action === 'hijack-dns' ? 'selected' : ''}"
			>
				{$t('routes.actionHijackDns')}
			</button>
		</div>
		<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">
			{#if action === 'route'}{$t('routes.actionRouteDesc')}
			{:else if action === 'reject'}{$t('routes.actionRejectDesc')}
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

	<!-- Sniff options -->
	{#if action === 'sniff'}
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4">
			<p class="text-sm text-[var(--ctp-overlay1)] mb-3">
				{$t('routes.sniffDescription')}
			</p>
			<div>
				<label for="sniff-timeout" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.timeout')}</label>
				<input
					id="sniff-timeout"
					type="text"
					bind:value={sniffTimeout}
					placeholder="300ms"
					class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				/>
				<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.timeoutHint')}</p>
			</div>
		</div>
	{/if}

	<!-- Hijack-DNS info -->
	{#if action === 'hijack-dns'}
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4">
			<p class="text-sm text-[var(--ctp-overlay1)]">
				{$t('routes.hijackDnsDescription')}
			</p>
		</div>
	{/if}

	<!-- Tabs (only for route/reject) -->
	{#if showConditionTabs}
		<div class="border-b border-[var(--ctp-surface2)]">
			<div class="flex gap-1">
				{#each tabs as tab}
					<button
						type="button"
						onclick={() => activeTab = tab.id}
						class="px-4 py-2 text-sm font-medium border-b-2 transition-colors -mb-px {activeTab === tab.id
							? 'border-[var(--ctp-primary)] text-[var(--ctp-primary)]'
							: 'border-transparent text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)]'}"
					>
						{tab.label}
					</button>
				{/each}
			</div>
		</div>

		{#if errors['conditions']}
			<p class="text-sm text-[var(--ctp-red)]">{errors['conditions']}</p>
		{/if}
	{/if}

	<!-- Tab Content (only for route/reject) -->
	{#if showConditionTabs}
	<div class="min-h-[200px]">
		{#if activeTab === 'destination'}
			<div class="space-y-4">
				<!-- Toggle options row -->
				<div class="flex flex-wrap gap-4">
					<label class="flex items-center gap-2 p-2 bg-[var(--ctp-surface0)] rounded-lg cursor-pointer hover:bg-[var(--ctp-surface1)] transition-colors">
						<input
							type="checkbox"
							bind:checked={ipIsPrivate}
							class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
						/>
						<div>
							<span class="text-sm text-[var(--ctp-text)]">{$t('routes.privateIp')}</span>
							<p class="text-xs text-[var(--ctp-overlay0)]">{$t('routes.privateIpHint')}</p>
						</div>
					</label>
					<label class="flex items-center gap-2 p-2 bg-[var(--ctp-surface0)] rounded-lg cursor-pointer hover:bg-[var(--ctp-surface1)] transition-colors">
						<input
							type="checkbox"
							bind:checked={invert}
							class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
						/>
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
					<textarea
						id="domain-suffix"
						bind:value={domainSuffix}
						rows={3}
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
					<textarea
						id="ip-cidr"
						bind:value={ipCidr}
						rows={2}
						placeholder="192.168.1.0/24&#10;10.0.0.0/8&#10;8.8.8.8"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
					></textarea>
				</div>

				<!-- Ports -->
				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="ports" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.port')}</label>
						<input
							id="ports"
							type="text"
							bind:value={ports}
							placeholder="80, 443, 8080"
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>
					<div>
						<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.network')}</label>
						<div class="flex gap-2">
							<button
								type="button"
								onclick={() => network = ''}
								class="flex-1 toggle-btn {network === '' ? 'selected' : ''}"
							>
								{$t('routes.networkAny')}
							</button>
							<button
								type="button"
								onclick={() => network = 'tcp'}
								class="flex-1 toggle-btn {network === 'tcp' ? 'selected' : ''}"
							>
								TCP
							</button>
							<button
								type="button"
								onclick={() => network = 'udp'}
								class="flex-1 toggle-btn {network === 'udp' ? 'selected' : ''}"
							>
								UDP
							</button>
						</div>
					</div>
				</div>
			</div>
		{:else if activeTab === 'source'}
			<div class="space-y-4">
				<!-- Inbound Filter -->
				{#if inbounds.length > 0}
					<div>
						<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">
							{$t('routes.matchFromInbound')}
						</label>
						<div class="bg-[var(--ctp-surface0)] rounded-lg border border-[var(--ctp-surface2)] divide-y divide-[var(--ctp-surface2)]">
							{#each inbounds as ib}
								<button
									type="button"
									onclick={() => toggleInbound(ib.tag)}
									class="w-full px-4 py-3 flex items-center justify-between hover:bg-[var(--ctp-surface1)] transition-colors text-left {selectedInbounds.includes(ib.tag) ? 'bg-[var(--ctp-surface1)]' : ''}"
								>
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
				{:else}
					<div class="p-4 bg-[var(--ctp-surface0)] rounded-lg text-center text-[var(--ctp-overlay0)]">
						<p>{$t('routes.noInboundsConfigured')}</p>
						<p class="text-xs mt-1">{$t('routes.noInboundsHint')}</p>
					</div>
				{/if}

				<!-- Source IP -->
				<div>
					<label for="source-ip-cidr" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('routes.sourceIpCidr')}
						<span class="font-normal text-[var(--ctp-overlay0)]">({$t('routes.clientAddress')})</span>
					</label>
					<textarea
						id="source-ip-cidr"
						bind:value={sourceIpCidr}
						rows={2}
						placeholder="192.168.1.100&#10;10.0.0.0/24"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
					></textarea>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.sourceIpCidrHint')}</p>
				</div>

				<!-- Source Ports -->
				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="source-ports" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.sourcePort')}</label>
						<input
							id="source-ports"
							type="text"
							bind:value={sourcePorts}
							placeholder="1024, 8080"
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>
					<div>
						<label for="source-port-range" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.sourcePortRange')}</label>
						<input
							id="source-port-range"
							type="text"
							bind:value={sourcePortRange}
							placeholder="1000:2000"
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>
				</div>
			</div>
		{:else if activeTab === 'advanced'}
			<div class="space-y-4">
				<!-- Rule Sets -->
				<div>
					<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('routes.ruleSets')}</label>
					{#if availableRuleSets.length === 0}
						<div class="p-4 bg-[var(--ctp-surface0)] rounded-lg text-center text-[var(--ctp-overlay0)]">
							<p>{$t('routes.noRuleSetsAvailable')}</p>
							<p class="text-xs mt-1">{hiddenRuleSets && hiddenRuleSets.size > 0 ? $t('routes.allRuleSetsHaveRoutes') : $t('routes.addRuleSetsFirst')}</p>
						</div>
					{:else}
						<div class="bg-[var(--ctp-surface0)] rounded-lg border border-[var(--ctp-surface2)] divide-y divide-[var(--ctp-surface2)]">
							{#each availableRuleSets as rs}
								<button
									type="button"
									onclick={() => toggleRuleSet(rs.tag)}
									class="w-full px-4 py-3 flex items-center justify-between hover:bg-[var(--ctp-surface1)] transition-colors text-left {selectedRuleSets.includes(rs.tag) ? 'bg-[var(--ctp-surface1)]' : ''}"
								>
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
					{/if}
				</div>

				<!-- Protocol -->
				<div>
					<label for="protocol" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.protocol')}</label>
					<input
						id="protocol"
						type="text"
						bind:value={protocol}
						placeholder="http, tls, quic"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.protocolHint')}</p>
				</div>

				<!-- Process -->
				<div>
					<label for="process-name" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('routes.processName')}
						<span class="font-normal text-[var(--ctp-overlay0)]">({$t('routes.onePerLine')})</span>
					</label>
					<textarea
						id="process-name"
						bind:value={processName}
						rows={2}
						placeholder="chrome&#10;firefox&#10;telegram"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
					></textarea>
				</div>

				<!-- Less common fields (collapsible) -->
				<details class="group">
					<summary class="cursor-pointer text-sm text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)]">
						{$t('routes.moreOptions')}
					</summary>
					<div class="mt-4 space-y-4 pl-4 border-l-2 border-[var(--ctp-surface2)]">
						<!-- Domain Exact Match -->
						<div>
							<label for="domain" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
								{$t('routes.domainExact')}
							</label>
							<textarea
								id="domain"
								bind:value={domain}
								rows={2}
								placeholder="example.com"
								class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
							></textarea>
						</div>

						<!-- Domain Keyword -->
						<div>
							<label for="domain-keyword" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
								{$t('routes.domainKeyword')}
							</label>
							<textarea
								id="domain-keyword"
								bind:value={domainKeyword}
								rows={2}
								placeholder="facebook&#10;twitter"
								class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
							></textarea>
						</div>

						<!-- Domain Regex -->
						<div>
							<label for="domain-regex" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
								{$t('routes.domainRegex')}
							</label>
							<textarea
								id="domain-regex"
								bind:value={domainRegex}
								rows={2}
								placeholder="^.*\.example\.com$"
								class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
							></textarea>
						</div>

						<!-- Port Range -->
						<div>
							<label for="port-range" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
								{$t('routes.portRange')}
							</label>
							<input
								id="port-range"
								type="text"
								bind:value={portRange}
								placeholder="1000:2000"
								class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
							/>
						</div>

						<!-- Process Path -->
						<div>
							<label for="process-path" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
								{$t('routes.processPath')}
							</label>
							<textarea
								id="process-path"
								bind:value={processPath}
								rows={2}
								placeholder="/usr/bin/telegram*"
								class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
							></textarea>
						</div>
					</div>
				</details>
			</div>
		{/if}
	</div>
	{/if}

	<!-- Actions -->
	<div class="flex justify-end gap-3 pt-4 border-t border-[var(--ctp-surface2)]">
		<button
			type="button"
			onclick={onCancel}
			class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
		>
			{$t('common.cancel')}
		</button>
		<button
			type="submit"
			class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
		>
			{rule ? $t('common.saveChanges') : $t('routes.addRule')}
		</button>
	</div>
</form>
