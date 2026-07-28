<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { RouteRule, RuleSet, Outbound } from '$lib/types';
	import { ruleSetRowTag, mappingOutboundValue } from '$lib/utils/routeRules';

	interface Props {
		rules: RouteRule[];
		onReorder: (from: number, to: number) => void;
		onEdit: (index: number) => void;
		onDelete: (index: number) => void;
		onAdd?: () => void;
		/** Known rule-sets: a rule that plainly maps one gets a rule-set row. */
		ruleSets?: RuleSet[];
		/** Targets offered by the inline picker on those rows. */
		outbounds?: Outbound[];
		/** Inline outbound switch; without it rule-set rows stay read-only. */
		onOutboundChange?: (index: number, outbound: string) => void;
		/** route.final — where a mapping with no explicit outbound actually goes. */
		finalOutbound?: string;
	}

	let {
		rules,
		onReorder,
		onEdit,
		onDelete,
		onAdd,
		ruleSets = [],
		outbounds = [],
		onOutboundChange,
		finalOutbound = ''
	}: Props = $props();

	// Rule-set rows are only offered where the tag is actually known and the page
	// wired up a change handler; otherwise the rule renders as an ordinary one.
	let knownRuleSets = $derived(new Map(ruleSets.map((rs) => [rs.tag, rs])));
	let knownTags = $derived(new Set(knownRuleSets.keys()));
	function ruleSetRowFor(rule: RouteRule): RuleSet | null {
		const tag = ruleSetRowTag(rule, knownTags, !!onOutboundChange);
		return tag ? (knownRuleSets.get(tag) ?? null) : null;
	}

	let draggedIndex = $state<number | null>(null);
	let dropTargetIndex = $state<number | null>(null);

	function handleDragStart(e: DragEvent, index: number) {
		draggedIndex = index;
		if (e.dataTransfer) {
			e.dataTransfer.effectAllowed = 'move';
			e.dataTransfer.setData('text/plain', index.toString());
		}
	}

	function handleDragOver(e: DragEvent, index: number) {
		e.preventDefault();
		if (e.dataTransfer) {
			e.dataTransfer.dropEffect = 'move';
		}
		if (draggedIndex !== null && draggedIndex !== index) {
			dropTargetIndex = index;
		}
	}

	function handleDragLeave() {
		dropTargetIndex = null;
	}

	function handleDrop(e: DragEvent, toIndex: number) {
		e.preventDefault();
		if (draggedIndex !== null && draggedIndex !== toIndex) {
			onReorder(draggedIndex, toIndex);
		}
		draggedIndex = null;
		dropTargetIndex = null;
	}

	function handleDragEnd() {
		draggedIndex = null;
		dropTargetIndex = null;
	}

	// Format list with "and X more" suffix
	function formatList(items: string[], max: number = 2): string {
		if (items.length <= max) {
			return items.join(', ');
		}
		return `${items.slice(0, max).join(', ')} +${items.length - max}`;
	}

	// Get condition type for icon
	type ConditionType = 'domain' | 'ip' | 'port' | 'process' | 'ruleset' | 'protocol' | 'network' | 'inbound' | 'private';

	function getConditionTypes(rule: RouteRule): ConditionType[] {
		const types: ConditionType[] = [];
		if (rule.inbound?.length) types.push('inbound');
		if (rule.ip_is_private) types.push('private');
		if (rule.domain?.length || rule.domain_suffix?.length || rule.domain_keyword?.length || rule.domain_regex?.length) types.push('domain');
		if (rule.ip_cidr?.length || rule.source_ip_cidr?.length) types.push('ip');
		if (rule.port?.length || rule.port_range?.length || rule.source_port?.length) types.push('port');
		if (rule.process_name?.length || rule.process_path?.length) types.push('process');
		if (rule.rule_set?.length) types.push('ruleset');
		if (rule.protocol?.length) types.push('protocol');
		if (rule.network) types.push('network');
		return types;
	}

	// Build human-readable description parts
	function getRuleDescriptionParts(rule: RouteRule): { main: string; details: string[] } {
		// Special actions
		if (rule.action === 'sniff') {
			return {
				main: 'Detect protocol',
				details: rule.timeout ? [`timeout: ${rule.timeout}`] : []
			};
		}
		if (rule.action === 'hijack-dns') {
			return {
				main: 'Redirect DNS queries',
				details: []
			};
		}

		const details: string[] = [];
		let main = '';

		// Inbound filter first (important context)
		if (rule.inbound?.length) {
			main = `From ${formatList(rule.inbound)}`;
		}

		// Domain conditions - show actual values
		if (rule.domain_suffix?.length) {
			const domains = rule.domain_suffix.map(d => `*.${d}`);
			details.push(formatList(domains, 3));
		}
		if (rule.domain?.length) {
			details.push(formatList(rule.domain, 3));
		}
		if (rule.domain_keyword?.length) {
			details.push(`contains: ${formatList(rule.domain_keyword, 2)}`);
		}
		if (rule.domain_regex?.length) {
			details.push(`${rule.domain_regex.length} regex pattern${rule.domain_regex.length > 1 ? 's' : ''}`);
		}

		// IP conditions
		if (rule.ip_is_private) {
			details.push('Private IP (LAN)');
		}
		if (rule.ip_cidr?.length) {
			details.push(formatList(rule.ip_cidr, 2));
		}
		if (rule.source_ip_cidr?.length) {
			details.push(`from: ${formatList(rule.source_ip_cidr, 2)}`);
		}

		// Port conditions
		if (rule.port?.length) {
			details.push(`port ${formatList(rule.port.map(String), 4)}`);
		}
		if (rule.port_range?.length) {
			details.push(`ports ${formatList(rule.port_range, 2)}`);
		}

		// Protocol
		if (rule.protocol?.length) {
			details.push(rule.protocol.join('/').toUpperCase());
		}

		// Network
		if (rule.network) {
			details.push(rule.network.toUpperCase());
		}

		// Rule sets - show names
		if (rule.rule_set?.length) {
			details.push(`ruleset: ${formatList(rule.rule_set, 2)}`);
		}

		// Process
		if (rule.process_name?.length) {
			details.push(`app: ${formatList(rule.process_name, 2)}`);
		}

		// Build main description if not set
		if (!main) {
			if (details.length === 0) {
				main = 'All traffic';
			} else if (details.length === 1) {
				main = details[0];
				details.length = 0;
			} else {
				main = details[0];
				details.shift();
			}
		}

		// Inversion flips the meaning of every condition above it, so it belongs in
		// front of the summary — as a trailing detail it would be the first thing
		// truncated, leaving a row that reads as the exact opposite of the rule.
		if (rule.invert) {
			main = `NOT ${main}`;
		}

		return { main, details };
	}

	// Get action/outbound display info
	function getTargetInfo(rule: RouteRule): { label: string; type: 'route' | 'reject' | 'action' } {
		if (rule.action === 'reject') {
			return { label: 'Block', type: 'reject' };
		}
		if (rule.action === 'sniff') {
			return { label: 'Sniff', type: 'action' };
		}
		if (rule.action === 'hijack-dns') {
			return { label: 'DNS', type: 'action' };
		}
		return { label: rule.outbound || 'direct', type: 'route' };
	}
</script>

<div class="space-y-2">
	{#each rules as rule, index}
		{@const desc = getRuleDescriptionParts(rule)}
		{@const target = getTargetInfo(rule)}
		{@const condTypes = getConditionTypes(rule)}
		{@const ruleSetRow = ruleSetRowFor(rule)}
		<div
			draggable="true"
			ondragstart={(e) => handleDragStart(e, index)}
			ondragover={(e) => handleDragOver(e, index)}
			ondragleave={handleDragLeave}
			ondrop={(e) => handleDrop(e, index)}
			ondragend={handleDragEnd}
			class="group relative flex items-center gap-3 p-3 bg-[var(--ctp-surface0)] rounded-lg border transition-all cursor-move
				{draggedIndex === index ? 'opacity-50 border-[var(--ctp-primary)]' : 'border-[var(--ctp-surface2)]'}
				{dropTargetIndex === index ? 'border-[var(--ctp-primary)] border-dashed' : ''}
				hover:border-[var(--ctp-overlay0)]"
		>
			<!-- Drag handle -->
			<div class="flex flex-col gap-0.5 text-[var(--ctp-overlay0)] group-hover:text-[var(--ctp-text)]">
				<svg class="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
					<path d="M7 2a2 2 0 1 0 .001 4.001A2 2 0 0 0 7 2zm0 6a2 2 0 1 0 .001 4.001A2 2 0 0 0 7 8zm0 6a2 2 0 1 0 .001 4.001A2 2 0 0 0 7 14zm6-8a2 2 0 1 0-.001-4.001A2 2 0 0 0 13 6zm0 2a2 2 0 1 0 .001 4.001A2 2 0 0 0 13 8zm0 6a2 2 0 1 0 .001 4.001A2 2 0 0 0 13 14z" />
				</svg>
			</div>

			<!-- Index -->
			<div class="w-8 h-8 flex-shrink-0 rounded-full bg-[var(--ctp-surface2)] flex items-center justify-center text-sm font-medium text-[var(--ctp-subtext0)]">
				{index + 1}
			</div>

			<!-- Condition type icons -->
			<div class="flex items-center gap-1 flex-shrink-0">
				{#if condTypes.includes('inbound')}
					<span class="w-5 h-5 rounded bg-[var(--ctp-surface2)] flex items-center justify-center" title="Inbound filter">
						<svg class="w-3 h-3 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 16l-4-4m0 0l4-4m-4 4h14" />
						</svg>
					</span>
				{/if}
				{#if condTypes.includes('domain')}
					<span class="w-5 h-5 rounded bg-[var(--ctp-surface2)] flex items-center justify-center" title="Domain match">
						<svg class="w-3 h-3 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9" />
						</svg>
					</span>
				{/if}
				{#if condTypes.includes('ip') || condTypes.includes('private')}
					<span class="w-5 h-5 rounded bg-[var(--ctp-surface2)] flex items-center justify-center" title="IP match">
						<svg class="w-3 h-3 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2" />
						</svg>
					</span>
				{/if}
				{#if condTypes.includes('port')}
					<span class="w-5 h-5 rounded bg-[var(--ctp-surface2)] flex items-center justify-center" title="Port match">
						<svg class="w-3 h-3 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z" />
						</svg>
					</span>
				{/if}
				{#if condTypes.includes('ruleset')}
					<span class="w-5 h-5 rounded bg-[var(--ctp-surface2)] flex items-center justify-center" title="Rule set">
						<svg class="w-3 h-3 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
						</svg>
					</span>
				{/if}
				{#if condTypes.includes('process')}
					<span class="w-5 h-5 rounded bg-[var(--ctp-surface2)] flex items-center justify-center" title="Process match">
						<svg class="w-3 h-3 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
						</svg>
					</span>
				{/if}
			</div>

			<!-- Rule info -->
			<div class="flex-1 min-w-0">
				{#if ruleSetRow}
					<div class="flex items-center gap-2 min-w-0">
						<span class="px-2 py-0.5 text-xs rounded bg-[var(--ctp-surface2)] text-[var(--ctp-overlay1)] flex-shrink-0">
							{ruleSetRow.type}
						</span>
						<span class="text-sm font-medium text-[var(--ctp-text)] truncate">{ruleSetRow.tag}</span>
					</div>
				{:else}
					<div class="flex flex-col">
						<span class="text-sm font-medium text-[var(--ctp-text)] truncate">{desc.main}</span>
						{#if desc.details.length > 0}
							<span class="text-xs text-[var(--ctp-overlay1)] truncate">{desc.details.join(' · ')}</span>
						{/if}
					</div>
				{/if}
			</div>

			<!-- Arrow and Outbound/Action -->
			<div class="flex items-center gap-2 flex-shrink-0">
				<svg class="w-4 h-4 text-[var(--ctp-overlay0)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 8l4 4m0 0l-4 4m4-4H3" />
				</svg>
				{#if ruleSetRow}
					<!-- Plain mapping: switching the target is one click, no rule form. -->
					<select
						value={mappingOutboundValue(rule)}
						onclick={(e) => e.stopPropagation()}
						onchange={(e) => {
							const val = (e.target as HTMLSelectElement).value;
							if (val) onOutboundChange?.(index, val);
						}}
						class="px-2 py-1 text-xs rounded bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] text-[var(--ctp-text)] focus:outline-none focus:ring-1 focus:ring-[var(--ctp-primary)] cursor-pointer"
					>
						{#if mappingOutboundValue(rule) === ''}
							<!-- No explicit outbound: the traffic falls through to route.final.
							     Shown, disabled, so the row states where it actually goes
							     instead of rendering an empty picker. -->
							<option value="" disabled selected>
								{finalOutbound ? $t('routes.finalFallback', { values: { outbound: finalOutbound } }) : $t('routes.noRoute')}
							</option>
						{/if}
						{#each outbounds as ob}
							<option value={ob.tag}>{ob.tag}</option>
						{/each}
						<option value="__reject__">⛔ REJECT</option>
					</select>
				{:else}
					<span class="status-badge {target.type === 'reject' ? 'error' : ''} {target.type === 'action' ? 'info' : ''}">
						{target.label}
					</span>
				{/if}
			</div>

			<!-- Actions -->
			<div class="flex items-center gap-1 flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity">
				<button
					type="button"
					onclick={(e) => { e.stopPropagation(); onEdit(index); }}
					class="p-1.5 rounded-md hover:bg-[var(--ctp-surface2)] text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)] transition-colors"
					title="Edit rule"
				>
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
					</svg>
				</button>
				<button
					type="button"
					onclick={(e) => { e.stopPropagation(); onDelete(index); }}
					class="action-btn-danger"
					title="Delete rule"
				>
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
					</svg>
				</button>
			</div>

			<!-- Drop indicator -->
			{#if dropTargetIndex === index}
				<div class="absolute -top-1 left-0 right-0 h-0.5 bg-[var(--ctp-primary)]"></div>
			{/if}
		</div>
	{/each}

	{#if rules.length === 0}
		<div class="p-8 text-center text-[var(--ctp-overlay0)] bg-[var(--ctp-surface0)] rounded-lg border border-dashed border-[var(--ctp-surface2)]">
			<p>{$t('routes.noRules')}</p>
			<p class="text-sm mt-1">{$t('routes.noRulesHint')}</p>
			{#if onAdd}
				<button
					type="button"
					onclick={onAdd}
					class="mt-4 px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
				>
					+ {$t('routes.addRule')}
				</button>
			{/if}
		</div>
	{/if}
</div>
