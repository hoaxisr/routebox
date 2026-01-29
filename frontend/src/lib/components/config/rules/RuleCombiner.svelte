<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { RuleConditions, RuleSet, Inbound } from '$lib/types';
	import { ConditionsForm } from './index';

	interface Props {
		conditions: RuleConditions[];
		mode: 'and' | 'or';
		ruleSets: RuleSet[];
		inbounds?: Inbound[];
		hiddenRuleSets?: Set<string>;
		onCreateRuleSet?: () => void;
	}

	let {
		conditions = $bindable(),
		mode = $bindable(),
		ruleSets,
		inbounds = [],
		hiddenRuleSets,
		onCreateRuleSet
	}: Props = $props();

	// Track which cards are expanded
	let expandedIndex = $state<number | null>(null);

	function addCondition() {
		conditions = [...conditions, {}];
		expandedIndex = conditions.length - 1;
	}

	function removeCondition(index: number) {
		conditions = conditions.filter((_, i) => i !== index);
		if (expandedIndex === index) {
			expandedIndex = null;
		} else if (expandedIndex !== null && expandedIndex > index) {
			expandedIndex--;
		}
	}

	function toggleExpand(index: number) {
		expandedIndex = expandedIndex === index ? null : index;
	}

	// Generate summary text for a condition
	function getConditionSummary(cond: RuleConditions): string {
		const parts: string[] = [];

		// Domain conditions
		if (cond.domain?.length) parts.push(`domain: ${cond.domain.slice(0, 2).join(', ')}${cond.domain.length > 2 ? '...' : ''}`);
		if (cond.domain_suffix?.length) parts.push(`suffix: ${cond.domain_suffix.slice(0, 2).join(', ')}${cond.domain_suffix.length > 2 ? '...' : ''}`);
		if (cond.domain_keyword?.length) parts.push(`keyword: ${cond.domain_keyword.slice(0, 2).join(', ')}${cond.domain_keyword.length > 2 ? '...' : ''}`);

		// IP conditions
		if (cond.ip_cidr?.length) parts.push(`ip: ${cond.ip_cidr.slice(0, 2).join(', ')}${cond.ip_cidr.length > 2 ? '...' : ''}`);
		if (cond.source_ip_cidr?.length) parts.push(`src ip: ${cond.source_ip_cidr.slice(0, 2).join(', ')}${cond.source_ip_cidr.length > 2 ? '...' : ''}`);
		if (cond.ip_is_private) parts.push('private IP');
		if (cond.source_ip_is_private) parts.push('private src IP');

		// Port conditions
		if (cond.port?.length) parts.push(`port: ${cond.port.slice(0, 3).join(', ')}${cond.port.length > 3 ? '...' : ''}`);
		if (cond.source_port?.length) parts.push(`src port: ${cond.source_port.slice(0, 3).join(', ')}${cond.source_port.length > 3 ? '...' : ''}`);

		// Protocol/network
		if (cond.protocol?.length) parts.push(`proto: ${cond.protocol.join(', ')}`);
		if (cond.network) parts.push(`net: ${cond.network}`);

		// Rule sets
		if (cond.rule_set?.length) parts.push(`ruleset: ${cond.rule_set.slice(0, 2).join(', ')}${cond.rule_set.length > 2 ? '...' : ''}`);

		// Process
		if (cond.process_name?.length) parts.push(`proc: ${cond.process_name.slice(0, 2).join(', ')}${cond.process_name.length > 2 ? '...' : ''}`);

		// Inbound
		if (cond.inbound?.length) parts.push(`inbound: ${cond.inbound.slice(0, 2).join(', ')}${cond.inbound.length > 2 ? '...' : ''}`);

		// Invert
		if (cond.invert) parts.push('(inverted)');

		return parts.length > 0 ? parts.join(' • ') : $t('routes.combiner.noConditions');
	}

	function hasConditions(cond: RuleConditions): boolean {
		return !!(
			cond.ip_is_private || cond.source_ip_is_private ||
			cond.inbound?.length ||
			cond.domain?.length || cond.domain_suffix?.length || cond.domain_keyword?.length || cond.domain_regex?.length ||
			cond.ip_cidr?.length || cond.source_ip_cidr?.length ||
			cond.port?.length || cond.port_range?.length || cond.source_port?.length || cond.source_port_range?.length ||
			cond.protocol?.length || cond.rule_set?.length ||
			cond.process_name?.length || cond.process_path?.length || cond.process_path_regex?.length ||
			cond.network || cond.ip_version !== undefined || cond.clash_mode ||
			cond.client?.length || cond.auth_user?.length || cond.user?.length || cond.user_id?.length
		);
	}
</script>

<div class="space-y-4">
	<!-- Mode toggle -->
	<div class="flex items-center gap-3">
		<span class="text-sm text-[var(--ctp-subtext1)]">{$t('routes.combiner.combineWith')}:</span>
		<div class="flex rounded-lg overflow-hidden border border-[var(--ctp-surface2)]">
			<button type="button" onclick={() => mode = 'and'}
				class="px-4 py-1.5 text-sm font-medium transition-colors {mode === 'and'
					? 'bg-[var(--ctp-primary)] text-white'
					: 'bg-[var(--ctp-surface0)] text-[var(--ctp-text)] hover:bg-[var(--ctp-surface1)]'}">
				{$t('routes.combiner.and')}
			</button>
			<button type="button" onclick={() => mode = 'or'}
				class="px-4 py-1.5 text-sm font-medium transition-colors {mode === 'or'
					? 'bg-[var(--ctp-primary)] text-white'
					: 'bg-[var(--ctp-surface0)] text-[var(--ctp-text)] hover:bg-[var(--ctp-surface1)]'}">
				{$t('routes.combiner.or')}
			</button>
		</div>
		<span class="text-xs text-[var(--ctp-overlay0)]">
			{mode === 'and' ? $t('routes.combiner.andHint') : $t('routes.combiner.orHint')}
		</span>
	</div>

	<!-- Condition cards -->
	<div class="space-y-0">
		{#each conditions as cond, index}
			<!-- Connector line with operator (except for first card) -->
			{#if index > 0}
				<div class="flex items-center justify-center py-2">
					<div class="flex-1 h-px bg-[var(--ctp-surface2)]"></div>
					<span class="px-3 py-1 text-xs font-medium rounded-full {mode === 'and'
						? 'bg-[var(--ctp-blue)] bg-opacity-20 text-[var(--ctp-blue)]'
						: 'bg-[var(--ctp-peach)] bg-opacity-20 text-[var(--ctp-peach)]'}">
						{mode === 'and' ? 'AND' : 'OR'}
					</span>
					<div class="flex-1 h-px bg-[var(--ctp-surface2)]"></div>
				</div>
			{/if}

			<!-- Condition card -->
			<div class="border border-[var(--ctp-surface2)] rounded-lg overflow-hidden {expandedIndex === index
				? 'ring-2 ring-[var(--ctp-primary)] ring-opacity-50'
				: ''} {!hasConditions(cond) ? 'border-[var(--ctp-yellow)] border-opacity-50' : ''}">
				<!-- Card header -->
				<div class="flex items-center gap-3 px-4 py-3 bg-[var(--ctp-surface0)] cursor-pointer"
					onclick={() => toggleExpand(index)}
					role="button"
					tabindex="0"
					onkeydown={(e) => e.key === 'Enter' && toggleExpand(index)}>
					<!-- Expand/collapse icon -->
					<svg class="w-4 h-4 text-[var(--ctp-overlay1)] transition-transform {expandedIndex === index ? 'rotate-90' : ''}"
						fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
					</svg>

					<!-- Condition number -->
					<span class="w-6 h-6 rounded-full bg-[var(--ctp-surface2)] flex items-center justify-center text-xs font-medium text-[var(--ctp-text)]">
						{index + 1}
					</span>

					<!-- Summary -->
					<div class="flex-1 min-w-0">
						<p class="text-sm text-[var(--ctp-text)] truncate {!hasConditions(cond) ? 'text-[var(--ctp-yellow)]' : ''}">
							{getConditionSummary(cond)}
						</p>
					</div>

					<!-- Remove button -->
					{#if conditions.length > 1}
						<button type="button"
							onclick={(e) => { e.stopPropagation(); removeCondition(index); }}
							class="p-1 rounded hover:bg-[var(--ctp-surface2)] text-[var(--ctp-overlay1)] hover:text-[var(--ctp-red)] transition-colors"
							title={$t('routes.combiner.removeCondition')}>
							<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
							</svg>
						</button>
					{/if}
				</div>

				<!-- Expanded content -->
				{#if expandedIndex === index}
					<div class="p-4 border-t border-[var(--ctp-surface2)] bg-[var(--ctp-base)]">
						<ConditionsForm
							bind:conditions={conditions[index]}
							{ruleSets}
							{inbounds}
							{hiddenRuleSets}
							{onCreateRuleSet}
						/>
					</div>
				{/if}
			</div>
		{/each}
	</div>

	<!-- Add condition button -->
	<button type="button" onclick={addCondition}
		class="w-full py-3 border-2 border-dashed border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-overlay1)] hover:border-[var(--ctp-primary)] hover:text-[var(--ctp-primary)] transition-colors flex items-center justify-center gap-2">
		<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
			<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
		</svg>
		{$t('routes.combiner.addCondition')}
	</button>
</div>
