<script lang="ts">
	import { Handle, Position, type NodeProps } from '@xyflow/svelte';
	import type { RouteRule } from '$lib/types';

	// Extended rule data with index and callbacks
	type RuleNodeData = RouteRule & {
		index: number;
		totalRules: number;
		onMoveUp?: (index: number) => void;
		onMoveDown?: (index: number) => void;
	};

	let { data, selected }: NodeProps = $props();

	// Cast data to proper type
	const ruleData = data as unknown as RuleNodeData;

	// Check if rule is logical type
	const isLogical = $derived(ruleData.type === 'logical');
	const logicalMode = $derived(isLogical ? ruleData.mode?.toUpperCase() : null);

	// Move handlers
	function handleMoveUp(e: MouseEvent) {
		e.stopPropagation();
		ruleData.onMoveUp?.(ruleData.index);
	}

	function handleMoveDown(e: MouseEvent) {
		e.stopPropagation();
		ruleData.onMoveDown?.(ruleData.index);
	}

	// Get rule_set badges
	function getRuleSetBadges(): string[] {
		return ruleData.rule_set ?? [];
	}

	// Get inbound badges
	function getInboundBadges(): string[] {
		return ruleData.inbound ?? [];
	}

	// Get a summary of the rule conditions (excluding rule_set and inbound - shown as badges)
	function getConditionsSummary(): string[] {
		const summary: string[] = [];

		if (ruleData.domain?.length) summary.push(`domain: ${ruleData.domain.length}`);
		if (ruleData.domain_suffix?.length) summary.push(`suffix: ${ruleData.domain_suffix.length}`);
		if (ruleData.domain_keyword?.length) summary.push(`keyword: ${ruleData.domain_keyword.length}`);
		if (ruleData.ip_cidr?.length) summary.push(`ip: ${ruleData.ip_cidr.length}`);
		if (ruleData.port) {
			const ports = Array.isArray(ruleData.port) ? ruleData.port : [ruleData.port];
			summary.push(`port: ${ports.join(', ')}`);
		}
		if (ruleData.protocol) {
			const proto = Array.isArray(ruleData.protocol) ? ruleData.protocol : [ruleData.protocol];
			summary.push(`protocol: ${proto.join(', ')}`);
		}
		if (ruleData.network) summary.push(`network: ${ruleData.network}`);
		if (ruleData.process_name?.length) summary.push(`process: ${ruleData.process_name.length}`);

		// For logical rules
		if (ruleData.type === 'logical' && ruleData.rules?.length) {
			summary.push(`${ruleData.rules.length} conditions`);
		}

		return summary.length > 0 ? summary : ['(any)'];
	}

	function getActionLabel(): string {
		if (ruleData.action === 'reject') return 'REJECT';
		if (ruleData.action === 'hijack-dns') return 'HIJACK DNS';
		if (ruleData.action === 'sniff') return 'SNIFF';
		if (ruleData.action === 'resolve') return 'RESOLVE';
		return ruleData.outbound || 'ROUTE';
	}

	function getActionClass(): string {
		if (ruleData.action === 'reject') return 'action-reject';
		if (ruleData.action === 'hijack-dns' || ruleData.action === 'sniff' || ruleData.action === 'resolve')
			return 'action-special';
		return 'action-route';
	}

	const conditions = $derived(getConditionsSummary());
	const actionLabel = $derived(getActionLabel());
	const actionClass = $derived(getActionClass());
	const ruleSetBadges = $derived(getRuleSetBadges());
	const inboundBadges = $derived(getInboundBadges());
	const canMoveUp = $derived(ruleData.index > 0);
	const canMoveDown = $derived(ruleData.index < ruleData.totalRules - 1);
</script>

<div class="rule-node" class:selected class:logical={isLogical}>
	<div class="rule-header">
		<div class="rule-index-wrapper">
			<span class="rule-index">#{ruleData.index + 1}</span>
			{#if isLogical}
				<span class="rule-type-badge logical">{logicalMode}</span>
			{/if}
		</div>
		<span class="rule-action {actionClass}">{actionLabel}</span>
	</div>

	<!-- Badges section -->
	{#if ruleSetBadges.length > 0 || inboundBadges.length > 0}
		<div class="badges-section">
			{#each inboundBadges as inbound}
				<span class="badge inbound-badge">{inbound}</span>
			{/each}
			{#each ruleSetBadges.slice(0, 2) as ruleSet}
				<span class="badge ruleset-badge">{ruleSet}</span>
			{/each}
			{#if ruleSetBadges.length > 2}
				<span class="badge more-badge">+{ruleSetBadges.length - 2}</span>
			{/if}
		</div>
	{/if}

	<div class="rule-conditions">
		{#each conditions.slice(0, 3) as condition}
			<div class="condition-line">{condition}</div>
		{/each}
		{#if conditions.length > 3}
			<div class="condition-more">+{conditions.length - 3} more</div>
		{/if}
	</div>

	<!-- Reorder buttons (shown when selected) -->
	{#if selected}
		<div class="reorder-buttons">
			<button
				class="reorder-btn"
				disabled={!canMoveUp}
				onclick={handleMoveUp}
				title="Move up"
			>
				<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<path d="M18 15l-6-6-6 6"/>
				</svg>
			</button>
			<button
				class="reorder-btn"
				disabled={!canMoveDown}
				onclick={handleMoveDown}
				title="Move down"
			>
				<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<path d="M6 9l6 6 6-6"/>
				</svg>
			</button>
		</div>
	{/if}

	<Handle type="source" position={Position.Right} />
</div>

<style>
	.rule-node {
		background: var(--ctp-mantle);
		border: 2px solid var(--ctp-surface1);
		border-radius: 0.5rem;
		padding: 0.75rem;
		min-width: 180px;
		max-width: 220px;
		font-size: 0.75rem;
		transition: all 0.15s ease;
		position: relative;
	}

	.rule-node:hover {
		border-color: var(--ctp-overlay0);
		transform: translateY(-1px);
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
	}

	.rule-node.selected {
		border-color: var(--ctp-primary);
		box-shadow: 0 0 0 2px color-mix(in srgb, var(--ctp-primary) 25%, transparent);
	}

	.rule-node.logical {
		border-left: 3px solid var(--ctp-blue);
	}

	.rule-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 0.5rem;
		padding-bottom: 0.5rem;
		border-bottom: 1px solid var(--ctp-surface0);
	}

	.rule-index-wrapper {
		display: flex;
		align-items: center;
		gap: 0.375rem;
	}

	.rule-index {
		color: var(--ctp-overlay1);
		font-weight: 500;
	}

	.rule-type-badge {
		padding: 0.0625rem 0.25rem;
		border-radius: 0.1875rem;
		font-size: 0.5625rem;
		font-weight: 600;
		text-transform: uppercase;
	}

	.rule-type-badge.logical {
		background: var(--ctp-blue);
		color: white;
	}

	.rule-action {
		padding: 0.125rem 0.5rem;
		border-radius: 0.25rem;
		font-weight: 600;
		font-size: 0.65rem;
		text-transform: uppercase;
	}

	.action-route {
		background: var(--ctp-primary);
		color: white;
	}

	.action-reject {
		background: var(--ctp-red);
		color: white;
	}

	.action-special {
		background: var(--ctp-surface2);
		color: var(--ctp-text);
	}

	/* Badges section */
	.badges-section {
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem;
		margin-bottom: 0.5rem;
	}

	.badge {
		padding: 0.0625rem 0.375rem;
		border-radius: 0.25rem;
		font-size: 0.5625rem;
		font-weight: 500;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		max-width: 80px;
	}

	.ruleset-badge {
		background: var(--ctp-surface2);
		color: var(--ctp-subtext0);
		border: 1px solid var(--ctp-surface1);
	}

	.inbound-badge {
		background: color-mix(in srgb, var(--ctp-teal) 20%, transparent);
		color: var(--ctp-teal);
		border: 1px solid var(--ctp-teal);
	}

	.more-badge {
		background: var(--ctp-surface1);
		color: var(--ctp-overlay1);
	}

	.rule-conditions {
		color: var(--ctp-subtext1);
		line-height: 1.4;
	}

	.condition-line {
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.condition-more {
		color: var(--ctp-overlay1);
		font-style: italic;
		margin-top: 0.25rem;
	}

	/* Reorder buttons */
	.reorder-buttons {
		position: absolute;
		left: -28px;
		top: 50%;
		transform: translateY(-50%);
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.reorder-btn {
		width: 20px;
		height: 20px;
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--ctp-surface1);
		border: 1px solid var(--ctp-surface2);
		border-radius: 0.25rem;
		color: var(--ctp-subtext1);
		cursor: pointer;
		transition: all 0.15s ease;
	}

	.reorder-btn:hover:not(:disabled) {
		background: var(--ctp-primary);
		border-color: var(--ctp-primary);
		color: white;
	}

	.reorder-btn:disabled {
		opacity: 0.3;
		cursor: not-allowed;
	}

	:global(.rule-node .svelte-flow__handle) {
		width: 10px;
		height: 10px;
		background: var(--ctp-primary);
		border: 2px solid var(--ctp-mantle);
	}
</style>
