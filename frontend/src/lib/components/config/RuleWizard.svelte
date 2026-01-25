<script lang="ts">
	import type { RouteRule, RuleSet, Outbound, Inbound } from '$lib/types';

	interface Props {
		ruleSets: RuleSet[];
		outbounds: Outbound[];
		inbounds: Inbound[];
		usedRuleSets?: Set<string>;
		onSave: (rule: RouteRule) => void;
		onCancel: () => void;
		onCreateRuleSet?: () => void;
	}

	let { ruleSets, outbounds, inbounds, usedRuleSets = new Set(), onSave, onCancel, onCreateRuleSet }: Props = $props();

	// Filter rule sets to only show unused ones
	let availableRuleSets = $derived(ruleSets.filter(rs => !usedRuleSets.has(rs.tag)));

	// Wizard steps
	type Step = 'action' | 'target' | 'conditions' | 'review';
	let currentStep = $state<Step>('action');

	// Rule data
	let action = $state<'route' | 'reject'>('route');
	let outbound = $state(outbounds.find(o => o.type !== 'direct' && o.type !== 'block')?.tag ?? outbounds[0]?.tag ?? '');

	// Target type (what kind of condition)
	type TargetType = 'domains' | 'ips' | 'ruleset' | 'other';
	let targetType = $state<TargetType>('domains');

	// Condition data
	let domainSuffix = $state('');
	let ipCidr = $state('');
	let selectedRuleSets = $state<string[]>([]);
	let ipIsPrivate = $state(false);
	let selectedInbounds = $state<string[]>([]);

	const steps: { id: Step; label: string; number: number }[] = [
		{ id: 'action', label: 'Action', number: 1 },
		{ id: 'target', label: 'What to Match', number: 2 },
		{ id: 'conditions', label: 'Specify', number: 3 },
		{ id: 'review', label: 'Review', number: 4 }
	];

	function nextStep() {
		if (currentStep === 'action') currentStep = 'target';
		else if (currentStep === 'target') currentStep = 'conditions';
		else if (currentStep === 'conditions') currentStep = 'review';
	}

	function prevStep() {
		if (currentStep === 'review') currentStep = 'conditions';
		else if (currentStep === 'conditions') currentStep = 'target';
		else if (currentStep === 'target') currentStep = 'action';
	}

	function canProceed(): boolean {
		if (currentStep === 'action') {
			return action === 'reject' || (action === 'route' && !!outbound);
		}
		if (currentStep === 'target') {
			return !!targetType;
		}
		if (currentStep === 'conditions') {
			if (targetType === 'domains') return domainSuffix.trim().length > 0;
			if (targetType === 'ips') return ipCidr.trim().length > 0 || ipIsPrivate;
			if (targetType === 'ruleset') return selectedRuleSets.length > 0;
			if (targetType === 'other') return selectedInbounds.length > 0;
		}
		return true;
	}

	function parseLines(text: string): string[] {
		return text.split('\n').map(s => s.trim()).filter(s => s);
	}

	function buildRule(): RouteRule {
		const rule: RouteRule = {};

		if (action !== 'route') rule.action = action;
		if (action === 'route') rule.outbound = outbound;

		if (targetType === 'domains' && domainSuffix.trim()) {
			rule.domain_suffix = parseLines(domainSuffix);
		}
		if (targetType === 'ips') {
			if (ipIsPrivate) rule.ip_is_private = true;
			if (ipCidr.trim()) rule.ip_cidr = parseLines(ipCidr);
		}
		if (targetType === 'ruleset' && selectedRuleSets.length > 0) {
			rule.rule_set = selectedRuleSets;
		}
		if (targetType === 'other' && selectedInbounds.length > 0) {
			rule.inbound = selectedInbounds;
		}

		return rule;
	}

	function handleSubmit() {
		onSave(buildRule());
	}

	function toggleRuleSet(tag: string) {
		if (selectedRuleSets.includes(tag)) {
			selectedRuleSets = selectedRuleSets.filter(t => t !== tag);
		} else {
			selectedRuleSets = [...selectedRuleSets, tag];
		}
	}

	function toggleInbound(tag: string) {
		if (selectedInbounds.includes(tag)) {
			selectedInbounds = selectedInbounds.filter(t => t !== tag);
		} else {
			selectedInbounds = [...selectedInbounds, tag];
		}
	}

	// Preview text
	let previewText = $derived.by(() => {
		const parts: string[] = [];

		if (targetType === 'domains' && domainSuffix.trim()) {
			const domains = parseLines(domainSuffix);
			parts.push(`Domains: ${domains.slice(0, 2).join(', ')}${domains.length > 2 ? ` +${domains.length - 2}` : ''}`);
		}
		if (targetType === 'ips') {
			if (ipIsPrivate) parts.push('Private IPs');
			if (ipCidr.trim()) {
				const ips = parseLines(ipCidr);
				parts.push(`IPs: ${ips.slice(0, 2).join(', ')}${ips.length > 2 ? ` +${ips.length - 2}` : ''}`);
			}
		}
		if (targetType === 'ruleset' && selectedRuleSets.length > 0) {
			parts.push(`Rule sets: ${selectedRuleSets.join(', ')}`);
		}
		if (targetType === 'other' && selectedInbounds.length > 0) {
			parts.push(`From inbound: ${selectedInbounds.join(', ')}`);
		}

		const target = action === 'route' ? outbound : 'REJECT';
		return parts.length > 0 ? `${parts.join(' · ')} → ${target}` : '';
	});
</script>

<div class="space-y-6">
	<!-- Step indicator -->
	<div class="flex items-center justify-between px-2">
		{#each steps as step, i}
			<div class="flex items-center">
				<div class="flex flex-col items-center">
					<div
						class="w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium transition-colors {
							currentStep === step.id
								? 'bg-[var(--ctp-primary)] text-white'
								: steps.findIndex(s => s.id === currentStep) > i
									? 'bg-[var(--ctp-green)] text-white'
									: 'bg-[var(--ctp-surface1)] text-[var(--ctp-overlay1)]'
						}"
					>
						{#if steps.findIndex(s => s.id === currentStep) > i}
							<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
							</svg>
						{:else}
							{step.number}
						{/if}
					</div>
					<span class="text-xs mt-1 text-[var(--ctp-overlay1)]">{step.label}</span>
				</div>
				{#if i < steps.length - 1}
					<div class="w-12 h-0.5 mx-2 {steps.findIndex(s => s.id === currentStep) > i ? 'bg-[var(--ctp-green)]' : 'bg-[var(--ctp-surface2)]'}"></div>
				{/if}
			</div>
		{/each}
	</div>

	<!-- Step content -->
	<div class="min-h-[280px]">
		{#if currentStep === 'action'}
			<div class="space-y-4">
				<h3 class="text-lg font-medium text-[var(--ctp-text)]">What should happen to matching traffic?</h3>

				<div class="grid gap-3">
					<button
						type="button"
						onclick={() => action = 'route'}
						class="p-4 rounded-lg border-2 text-left transition-colors {action === 'route' ? 'border-[var(--ctp-primary)] bg-[var(--ctp-surface0)]' : 'border-[var(--ctp-surface2)] hover:border-[var(--ctp-overlay0)]'}"
					>
						<div class="flex items-center gap-3">
							<div class="w-10 h-10 rounded-lg bg-[var(--ctp-primary)] bg-opacity-20 flex items-center justify-center">
								<svg class="w-5 h-5 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6" />
								</svg>
							</div>
							<div>
								<div class="font-medium text-[var(--ctp-text)]">Route to outbound</div>
								<div class="text-sm text-[var(--ctp-overlay1)]">Send traffic through VPN or direct</div>
							</div>
						</div>
					</button>

					<button
						type="button"
						onclick={() => action = 'reject'}
						class="p-4 rounded-lg border-2 text-left transition-colors {action === 'reject' ? 'border-[var(--ctp-red)] bg-[var(--ctp-surface0)]' : 'border-[var(--ctp-surface2)] hover:border-[var(--ctp-overlay0)]'}"
					>
						<div class="flex items-center gap-3">
							<div class="w-10 h-10 rounded-lg bg-[var(--ctp-red)] bg-opacity-20 flex items-center justify-center">
								<svg class="w-5 h-5 text-[var(--ctp-red)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636" />
								</svg>
							</div>
							<div>
								<div class="font-medium text-[var(--ctp-text)]">Block / Reject</div>
								<div class="text-sm text-[var(--ctp-overlay1)]">Drop matching connections</div>
							</div>
						</div>
					</button>
				</div>

				{#if action === 'route'}
					<div class="mt-4">
						<label for="wizard-outbound" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">
							Route to:
						</label>
						<select
							id="wizard-outbound"
							bind:value={outbound}
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						>
							{#each outbounds as ob}
								<option value={ob.tag}>{ob.tag} ({ob.type})</option>
							{/each}
						</select>
					</div>
				{/if}
			</div>

		{:else if currentStep === 'target'}
			<div class="space-y-4">
				<h3 class="text-lg font-medium text-[var(--ctp-text)]">What do you want to match?</h3>

				<div class="grid gap-3">
					<button
						type="button"
						onclick={() => targetType = 'domains'}
						class="p-4 rounded-lg border-2 text-left transition-colors {targetType === 'domains' ? 'border-[var(--ctp-primary)] bg-[var(--ctp-surface0)]' : 'border-[var(--ctp-surface2)] hover:border-[var(--ctp-overlay0)]'}"
					>
						<div class="flex items-center gap-3">
							<span class="text-2xl">🌐</span>
							<div>
								<div class="font-medium text-[var(--ctp-text)]">Domains</div>
								<div class="text-sm text-[var(--ctp-overlay1)]">Match by website domain (google.com, youtube.com)</div>
							</div>
						</div>
					</button>

					<button
						type="button"
						onclick={() => targetType = 'ips'}
						class="p-4 rounded-lg border-2 text-left transition-colors {targetType === 'ips' ? 'border-[var(--ctp-primary)] bg-[var(--ctp-surface0)]' : 'border-[var(--ctp-surface2)] hover:border-[var(--ctp-overlay0)]'}"
					>
						<div class="flex items-center gap-3">
							<span class="text-2xl">📍</span>
							<div>
								<div class="font-medium text-[var(--ctp-text)]">IP Addresses</div>
								<div class="text-sm text-[var(--ctp-overlay1)]">Match by destination IP or CIDR range</div>
							</div>
						</div>
					</button>

					<button
						type="button"
						onclick={() => targetType = 'ruleset'}
						class="p-4 rounded-lg border-2 text-left transition-colors {targetType === 'ruleset' ? 'border-[var(--ctp-primary)] bg-[var(--ctp-surface0)]' : 'border-[var(--ctp-surface2)] hover:border-[var(--ctp-overlay0)]'}"
					>
						<div class="flex items-center gap-3">
							<span class="text-2xl">📋</span>
							<div>
								<div class="font-medium text-[var(--ctp-text)]">Rule Sets</div>
								<div class="text-sm text-[var(--ctp-overlay1)]">Use pre-defined domain/IP lists</div>
							</div>
						</div>
					</button>

					{#if inbounds.length > 0}
						<button
							type="button"
							onclick={() => targetType = 'other'}
							class="p-4 rounded-lg border-2 text-left transition-colors {targetType === 'other' ? 'border-[var(--ctp-primary)] bg-[var(--ctp-surface0)]' : 'border-[var(--ctp-surface2)] hover:border-[var(--ctp-overlay0)]'}"
						>
							<div class="flex items-center gap-3">
								<span class="text-2xl">🔌</span>
								<div>
									<div class="font-medium text-[var(--ctp-text)]">By Inbound</div>
									<div class="text-sm text-[var(--ctp-overlay1)]">Match traffic from specific interfaces</div>
								</div>
							</div>
						</button>
					{/if}
				</div>
			</div>

		{:else if currentStep === 'conditions'}
			<div class="space-y-4">
				<h3 class="text-lg font-medium text-[var(--ctp-text)]">
					{#if targetType === 'domains'}Enter domain names{:else if targetType === 'ips'}Enter IP addresses{:else if targetType === 'ruleset'}Select rule sets{:else}Select inbounds{/if}
				</h3>

				{#if targetType === 'domains'}
					<div>
						<label for="wizard-domains" class="block text-sm text-[var(--ctp-overlay1)] mb-2">
							Enter domains (one per line). Matches the domain and all subdomains.
						</label>
						<textarea
							id="wizard-domains"
							bind:value={domainSuffix}
							rows={6}
							placeholder="google.com&#10;youtube.com&#10;facebook.com"
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
						></textarea>
					</div>
				{:else if targetType === 'ips'}
					<div class="space-y-4">
						<label class="flex items-center gap-3 p-3 bg-[var(--ctp-surface0)] rounded-lg cursor-pointer hover:bg-[var(--ctp-surface1)] transition-colors">
							<input
								type="checkbox"
								bind:checked={ipIsPrivate}
								class="w-5 h-5 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
							/>
							<div>
								<span class="font-medium text-[var(--ctp-text)]">Private/LAN IPs</span>
								<p class="text-sm text-[var(--ctp-overlay1)]">192.168.x.x, 10.x.x.x, 172.16-31.x.x</p>
							</div>
						</label>

						<div>
							<label for="wizard-ips" class="block text-sm text-[var(--ctp-overlay1)] mb-2">
								Or enter specific IPs/CIDRs (one per line):
							</label>
							<textarea
								id="wizard-ips"
								bind:value={ipCidr}
								rows={4}
								placeholder="8.8.8.8&#10;1.1.1.1&#10;10.0.0.0/8"
								class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
							></textarea>
						</div>
					</div>
				{:else if targetType === 'ruleset'}
					{#if availableRuleSets.length > 0}
						<div class="bg-[var(--ctp-surface0)] rounded-lg border border-[var(--ctp-surface2)] divide-y divide-[var(--ctp-surface2)] max-h-64 overflow-y-auto">
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
					{:else}
						<div class="bg-[var(--ctp-surface0)] rounded-lg border border-[var(--ctp-surface2)] p-6 text-center">
							<p class="text-[var(--ctp-overlay1)] mb-2">No available rule sets</p>
							<p class="text-sm text-[var(--ctp-overlay0)]">All rule sets already have routing rules assigned</p>
						</div>
					{/if}
					{#if onCreateRuleSet}
						<button
							type="button"
							onclick={onCreateRuleSet}
							class="mt-3 w-full px-4 py-2.5 border-2 border-dashed border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-overlay1)] hover:border-[var(--ctp-primary)] hover:text-[var(--ctp-primary)] transition-colors flex items-center justify-center gap-2"
						>
							<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
							</svg>
							Create New Rule Set
						</button>
					{/if}
				{:else if targetType === 'other'}
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
				{/if}
			</div>

		{:else if currentStep === 'review'}
			<div class="space-y-4">
				<h3 class="text-lg font-medium text-[var(--ctp-text)]">Review your rule</h3>

				<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-3">
					<div class="flex items-center gap-2">
						<span class="status-badge {action === 'route' ? 'success' : 'error'}">
							{action === 'route' ? 'ROUTE' : 'REJECT'}
						</span>
						{#if action === 'route'}
							<span class="text-[var(--ctp-text)]">→</span>
							<span class="font-medium text-[var(--ctp-primary)]">{outbound}</span>
						{/if}
					</div>

					<div class="border-t border-[var(--ctp-surface2)] pt-3">
						<div class="text-sm text-[var(--ctp-overlay1)] mb-1">Matching:</div>
						<div class="text-[var(--ctp-text)]">
							{#if targetType === 'domains' && domainSuffix.trim()}
								<div class="flex flex-wrap gap-1">
									{#each parseLines(domainSuffix).slice(0, 5) as domain}
										<span class="selection-chip">{domain}</span>
									{/each}
									{#if parseLines(domainSuffix).length > 5}
										<span class="text-[var(--ctp-overlay0)]">+{parseLines(domainSuffix).length - 5} more</span>
									{/if}
								</div>
							{:else if targetType === 'ips'}
								<div class="flex flex-wrap gap-1">
									{#if ipIsPrivate}
										<span class="selection-chip">Private IPs</span>
									{/if}
									{#each parseLines(ipCidr).slice(0, 3) as ip}
										<span class="selection-chip">{ip}</span>
									{/each}
									{#if parseLines(ipCidr).length > 3}
										<span class="text-[var(--ctp-overlay0)]">+{parseLines(ipCidr).length - 3} more</span>
									{/if}
								</div>
							{:else if targetType === 'ruleset'}
								<div class="flex flex-wrap gap-1">
									{#each selectedRuleSets as rs}
										<span class="selection-chip">{rs}</span>
									{/each}
								</div>
							{:else if targetType === 'other'}
								<div class="flex flex-wrap gap-1">
									{#each selectedInbounds as ib}
										<span class="selection-chip">{ib}</span>
									{/each}
								</div>
							{/if}
						</div>
					</div>
				</div>

				<p class="text-sm text-[var(--ctp-overlay1)]">
					Click "Create Rule" to add this rule. You can edit it later or drag to reorder.
				</p>
			</div>
		{/if}
	</div>

	<!-- Preview bar -->
	{#if previewText}
		<div class="bg-[var(--ctp-surface0)] rounded-lg px-4 py-2 text-sm text-[var(--ctp-overlay1)]">
			{previewText}
		</div>
	{/if}

	<!-- Navigation -->
	<div class="flex justify-between pt-4 border-t border-[var(--ctp-surface2)]">
		<button
			type="button"
			onclick={currentStep === 'action' ? onCancel : prevStep}
			class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
		>
			{currentStep === 'action' ? 'Cancel' : 'Back'}
		</button>

		{#if currentStep === 'review'}
			<button
				type="button"
				onclick={handleSubmit}
				class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
			>
				Create Rule
			</button>
		{:else}
			<button
				type="button"
				onclick={nextStep}
				disabled={!canProceed()}
				class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50"
			>
				Next
			</button>
		{/if}
	</div>
</div>
