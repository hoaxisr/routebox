<script lang="ts">
	import type { DnsRule, DnsServer, RuleSet, Outbound } from '$lib/types';
	import { notifications, configReadOnly } from '$lib/stores';
	import { api } from '$lib/api/client';
	import { t } from 'svelte-i18n';
	import RuleSetForm from './RuleSetForm.svelte';

	interface Props {
		rule?: DnsRule;
		dnsServers: DnsServer[];
		ruleSets: RuleSet[];
		outbounds?: Outbound[];
		onSave: (rule: DnsRule) => void;
		onCancel: () => void;
		onRuleSetCreated?: (ruleSet: RuleSet) => void;
	}

	let { rule, dnsServers, ruleSets, outbounds = [], onSave, onCancel, onRuleSetCreated }: Props = $props();

	let showInlineRuleSetForm = $state(false);

	// Form state - Match conditions
	let domain = $state(rule?.domain?.join('\n') ?? '');
	let domainSuffix = $state(rule?.domain_suffix?.join('\n') ?? '');
	let domainKeyword = $state(rule?.domain_keyword?.join('\n') ?? '');
	let domainRegex = $state(rule?.domain_regex?.join('\n') ?? '');
	let ipCidr = $state(rule?.ip_cidr?.join('\n') ?? '');
	let queryType = $state(rule?.query_type?.join(', ') ?? '');
	let selectedRuleSets = $state<string[]>(rule?.rule_set ?? []);

	// Action settings
	// evaluate/respond exist only in the generated fallback tail (#68), which this
	// form never opens — anything else edits as a plain route rule.
	let action = $state<'route' | 'reject' | 'predefined'>(
		rule?.action === 'reject' || rule?.action === 'predefined' ? rule.action : 'route'
	);
	let server = $state(rule?.server ?? '');

	// Reject action options
	let rejectMethod = $state<'default' | 'drop'>(rule?.method ?? 'default');
	let noDrop = $state(rule?.no_drop ?? false);

	// Predefined action options
	let rcode = $state<DnsRule['rcode']>(rule?.rcode ?? 'NXDOMAIN');

	// Common options
	let disableCache = $state(rule?.disable_cache ?? false);

	// Tabs
	type Tab = 'domains' | 'other' | 'ruleset';
	let activeTab = $state<Tab>('domains');

	let errors = $state<Record<string, string>>({});

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

	function parseComma(text: string): string[] {
		return text.split(',').map(s => s.trim().toUpperCase()).filter(s => s);
	}

	function validate(): boolean {
		errors = {};

		// Server is required ONLY for route action
		if (action === 'route' && !server) {
			errors['server'] = $t('dns.ruleServer') + ' ' + $t('common.required').toLowerCase();
		}

		// Check at least one condition
		const hasCondition = domain.trim() ||
			domainSuffix.trim() ||
			domainKeyword.trim() ||
			domainRegex.trim() ||
			ipCidr.trim() ||
			queryType.trim() ||
			selectedRuleSets.length > 0;

		if (!hasCondition) {
			errors['conditions'] = $t('routes.ruleConditions') + ' ' + $t('common.required').toLowerCase();
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

		const newRule: DnsRule = {};

		// Match conditions
		if (domain.trim()) newRule.domain = parseLines(domain);
		if (domainSuffix.trim()) newRule.domain_suffix = parseLines(domainSuffix);
		if (domainKeyword.trim()) newRule.domain_keyword = parseLines(domainKeyword);
		if (domainRegex.trim()) newRule.domain_regex = parseLines(domainRegex);
		if (ipCidr.trim()) newRule.ip_cidr = parseLines(ipCidr);
		if (queryType.trim()) newRule.query_type = parseComma(queryType);
		if (selectedRuleSets.length > 0) newRule.rule_set = selectedRuleSets;

		// Action and action-specific fields
		if (action !== 'route') {
			newRule.action = action;
		}

		if (action === 'route') {
			newRule.server = server;
		} else if (action === 'reject') {
			if (rejectMethod !== 'default') newRule.method = rejectMethod;
			if (noDrop) newRule.no_drop = true;
		} else if (action === 'predefined') {
			newRule.rcode = rcode;
		}

		// Common options
		if (disableCache) newRule.disable_cache = true;

		onSave(newRule);
	}

	const tabs: { id: Tab; label: string }[] = $derived([
		{ id: 'domains', label: $t('routes.domain') },
		{ id: 'other', label: $t('common.advanced') },
		{ id: 'ruleset', label: $t('routes.ruleSets') }
	]);

	const queryTypePresets = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS', 'PTR'];

	const rcodeOptions: { value: DnsRule['rcode']; label: string; descKey: string }[] = [
		{ value: 'NXDOMAIN', label: 'NXDOMAIN', descKey: 'dns.rcodeNxdomain' },
		{ value: 'REFUSED', label: 'REFUSED', descKey: 'dns.rcodeRefused' },
		{ value: 'SERVFAIL', label: 'SERVFAIL', descKey: 'dns.rcodeServfail' },
		{ value: 'NOERROR', label: 'NOERROR', descKey: 'dns.rcodeNoerror' },
		{ value: 'FORMERR', label: 'FORMERR', descKey: 'dns.rcodeFormerr' },
		{ value: 'NOTIMP', label: 'NOTIMP', descKey: 'dns.rcodeNotimp' },
	];
</script>

<form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="space-y-6">
	<!-- Action Selection (moved to top for better UX) -->
	<div>
		<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('routes.ruleAction')}</label>
		<div class="flex gap-2">
			<button
				type="button"
				onclick={() => action = 'route'}
				class="toggle-btn {action === 'route' ? 'selected' : ''}"
			>
				{$t('dns.actionRoute')}
			</button>
			<button
				type="button"
				onclick={() => action = 'reject'}
				class="toggle-btn-danger {action === 'reject' ? 'selected' : ''}"
			>
				{$t('dns.actionReject')}
			</button>
			<button
				type="button"
				onclick={() => action = 'predefined'}
				class="toggle-btn-danger {action === 'predefined' ? 'selected' : ''}"
			>
				{$t('dns.actionPredefined')}
			</button>
		</div>
	</div>

	<!-- Action-specific settings -->
	{#if action === 'route'}
		<!-- DNS Server (only for route) -->
		<div>
			<label for="server" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('dns.ruleServer')} *</label>
			<select
				id="server"
				bind:value={server}
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['server'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
			>
				<option value="">{$t('dns.selectServer')}</option>
				{#each dnsServers as srv}
					<option value={srv.tag}>{srv.tag} ({srv.type})</option>
				{/each}
			</select>
			{#if errors['server']}
				<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['server']}</p>
			{/if}
		</div>
	{:else if action === 'reject'}
		<!-- Reject options -->
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
			<div>
				<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('dns.rejectMethod')}</label>
				<div class="flex gap-2">
					<button
						type="button"
						onclick={() => rejectMethod = 'default'}
						class="toggle-btn {rejectMethod === 'default' ? 'selected' : ''}"
					>
						REFUSED
					</button>
					<button
						type="button"
						onclick={() => rejectMethod = 'drop'}
						class="toggle-btn {rejectMethod === 'drop' ? 'selected' : ''}"
					>
						{$t('dns.rejectDrop')}
					</button>
				</div>
				<p class="mt-2 text-xs text-[var(--ctp-overlay0)]">
					{rejectMethod === 'default' ? $t('dns.rejectRefusedHint') : $t('dns.rejectDropHint')}
				</p>
			</div>

			{#if rejectMethod === 'default'}
				<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
					<input
						type="checkbox"
						bind:checked={noDrop}
						class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
					/>
					{$t('dns.noDropLabel')}
					<span class="text-xs text-[var(--ctp-overlay0)]">({$t('dns.noDropHint')})</span>
				</label>
			{/if}
		</div>
	{:else if action === 'predefined'}
		<!-- Predefined options -->
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4">
			<label for="rcode" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('dns.responseCode')}</label>
			<div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
				{#each rcodeOptions as opt}
					<button
						type="button"
						onclick={() => rcode = opt.value}
						class="text-left px-3 py-2 rounded-lg border transition-colors {rcode === opt.value
							? 'border-[var(--ctp-primary)] bg-[color-mix(in_srgb,var(--ctp-primary)_10%,transparent)]'
							: 'border-[var(--ctp-surface2)] hover:border-[var(--ctp-overlay0)]'}"
					>
						<span class="font-mono text-sm {rcode === opt.value ? 'text-[var(--ctp-primary)]' : 'text-[var(--ctp-text)]'}">{opt.label}</span>
						<p class="text-xs text-[var(--ctp-overlay0)]">{$t(opt.descKey)}</p>
					</button>
				{/each}
			</div>
		</div>
	{/if}

	<!-- Tabs for match conditions -->
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
					{#if tab.id === 'ruleset' && selectedRuleSets.length > 0}
						<span class="ml-1 text-xs">({selectedRuleSets.length})</span>
					{/if}
				</button>
			{/each}
		</div>
	</div>

	{#if errors['conditions']}
		<p class="text-sm text-[var(--ctp-red)]">{errors['conditions']}</p>
	{/if}

	<!-- Tab Content -->
	<div class="min-h-[200px]">
		{#if activeTab === 'domains'}
			<div class="space-y-4">
				<!-- Domain (exact) -->
				<div>
					<label for="domain" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('routes.domain')}
						<span class="font-normal text-[var(--ctp-overlay0)]">({$t('dns.onePerLine')})</span>
					</label>
					<textarea
						id="domain"
						bind:value={domain}
						rows={2}
						placeholder="example.com&#10;test.org"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
					></textarea>
				</div>

				<!-- Domain Suffix -->
				<div>
					<label for="domain-suffix" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('routes.domainSuffix')}
						<span class="font-normal text-[var(--ctp-overlay0)]">({$t('dns.onePerLine')})</span>
					</label>
					<textarea
						id="domain-suffix"
						bind:value={domainSuffix}
						rows={2}
						placeholder="google.com&#10;youtube.com"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
					></textarea>
				</div>

				<!-- Domain Keyword -->
				<div>
					<label for="domain-keyword" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('routes.domainKeyword')}
						<span class="font-normal text-[var(--ctp-overlay0)]">({$t('dns.onePerLine')})</span>
					</label>
					<textarea
						id="domain-keyword"
						bind:value={domainKeyword}
						rows={2}
						placeholder="facebook&#10;instagram"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
					></textarea>
				</div>

				<!-- Domain Regex -->
				<div>
					<label for="domain-regex" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('routes.domainRegex')}
						<span class="font-normal text-[var(--ctp-overlay0)]">({$t('dns.onePerLine')})</span>
					</label>
					<textarea
						id="domain-regex"
						bind:value={domainRegex}
						rows={2}
						placeholder="^ads?\\..*$&#10;.*\\.ru$"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
					></textarea>
				</div>
			</div>
		{:else if activeTab === 'other'}
			<div class="space-y-4">
				<!-- IP CIDR -->
				<div>
					<label for="ip-cidr" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('routes.ipCidr')}
						<span class="font-normal text-[var(--ctp-overlay0)]">({$t('dns.onePerLine')})</span>
					</label>
					<textarea
						id="ip-cidr"
						bind:value={ipCidr}
						rows={2}
						placeholder="192.168.0.0/16&#10;10.0.0.0/8"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
					></textarea>
				</div>

				<!-- Query Type -->
				<div>
					<label for="query-type" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('dns.queryType')}
						<span class="font-normal text-[var(--ctp-overlay0)]">({$t('dns.commaSeparated')})</span>
					</label>
					<input
						id="query-type"
						type="text"
						bind:value={queryType}
						placeholder="A, AAAA, CNAME"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
					<div class="mt-2 flex flex-wrap gap-1">
						{#each queryTypePresets as qt}
							<button
								type="button"
								onclick={() => {
									const types = parseComma(queryType);
									if (types.includes(qt)) {
										queryType = types.filter(t => t !== qt).join(', ');
									} else {
										queryType = [...types, qt].join(', ');
									}
								}}
								class="query-type-chip {parseComma(queryType).includes(qt) ? 'selected' : ''}"
							>
								{qt}
							</button>
						{/each}
					</div>
				</div>
			</div>
		{:else if activeTab === 'ruleset'}
			<div class="space-y-4">
				{#if ruleSets.length === 0}
					<div class="p-8 text-center text-[var(--ctp-overlay0)]">
						<p>{$t('routes.noRuleSets')}</p>
						<p class="text-sm mt-1">{$t('routes.noRuleSetsHint')}</p>
					</div>
				{:else}
					<p class="text-sm text-[var(--ctp-overlay1)]">{$t('dns.selectRuleSetsHint')}</p>
					<div class="bg-[var(--ctp-surface0)] rounded-lg border border-[var(--ctp-surface2)] divide-y divide-[var(--ctp-surface2)]">
						{#each ruleSets as rs}
							<button
								type="button"
								onclick={() => toggleRuleSet(rs.tag)}
								class="w-full px-4 py-3 flex items-center justify-between hover:bg-[var(--ctp-surface1)] transition-colors text-left {selectedRuleSets.includes(rs.tag) ? 'bg-[var(--ctp-surface1)]' : ''}"
							>
								<div>
									<span class="font-medium text-[var(--ctp-text)]">{rs.tag}</span>
									<span class="ml-2 selection-chip">
										{rs.type}
									</span>
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
				<button
					type="button"
					onclick={() => showInlineRuleSetForm = true}
					class="mt-2 px-3 py-1.5 text-sm text-[var(--ctp-primary)] hover:bg-[var(--ctp-surface0)] rounded-lg transition-colors flex items-center gap-1"
				>
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
					</svg>
					{$t('ruleSets.newRuleSet')}
				</button>
			</div>
		{/if}
	</div>

	<!-- Options -->
	<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
		<input
			type="checkbox"
			bind:checked={disableCache}
			class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
		/>
		{$t('dns.disableCache')}
	</label>

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
			class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50 disabled:cursor-not-allowed"
			disabled={$configReadOnly}
			title={$configReadOnly ? $t('readOnly.saveBlocked') : ''}
		>
			{rule ? $t('common.saveChanges') : $t('dns.addRule')}
		</button>
	</div>
</form>

<!-- Inline Rule Set Creation Modal -->
{#if showInlineRuleSetForm}
	<div class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-[60] p-4">
		<div class="bg-[var(--ctp-base)] rounded-xl w-full max-w-lg max-h-[90vh] overflow-y-auto">
			<div class="px-4 py-3 border-b border-[var(--ctp-surface2)] flex items-center justify-between">
				<h2 class="text-lg font-medium text-[var(--ctp-text)]">{$t('ruleSets.newRuleSet')}</h2>
				<button
					onclick={() => showInlineRuleSetForm = false}
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
