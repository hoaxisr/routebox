<script lang="ts">
	import { t } from 'svelte-i18n';
	import { featureFlags } from '$lib/stores';
	import type { RuleSet } from '$lib/types';
	import HelpTooltip from '$lib/components/shared/HelpTooltip.svelte';

	interface Props {
		availableRuleSets: RuleSet[];
		selectedRuleSets: string[];
		ruleSetIpCidrMatchSource: boolean;
		hiddenRuleSets?: Set<string>;
		protocol: string;
		processName: string;
		client: string;
		// Less common fields
		domain: string;
		domainKeyword: string;
		domainRegex: string;
		portRange: string;
		processPath: string;
		processPathRegex: string;
		user: string;
		userId: string;
		// Callback
		onCreateRuleSet?: () => void;
	}

	let {
		availableRuleSets,
		selectedRuleSets = $bindable(),
		ruleSetIpCidrMatchSource = $bindable(),
		hiddenRuleSets,
		protocol = $bindable(),
		processName = $bindable(),
		client = $bindable(),
		domain = $bindable(),
		domainKeyword = $bindable(),
		domainRegex = $bindable(),
		portRange = $bindable(),
		processPath = $bindable(),
		processPathRegex = $bindable(),
		user = $bindable(),
		userId = $bindable(),
		onCreateRuleSet
	}: Props = $props();

	function toggleRuleSet(tag: string) {
		if (selectedRuleSets.includes(tag)) {
			selectedRuleSets = selectedRuleSets.filter((t) => t !== tag);
		} else {
			selectedRuleSets = [...selectedRuleSets, tag];
		}
	}
</script>

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
		{#if onCreateRuleSet}
			<button type="button" onclick={onCreateRuleSet}
				class="mt-2 px-3 py-1.5 text-sm text-[var(--ctp-primary)] hover:bg-[var(--ctp-surface0)] rounded-lg transition-colors flex items-center gap-1">
				<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
				</svg>
				{$t('ruleSets.newRuleSet')}
			</button>
		{/if}
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
