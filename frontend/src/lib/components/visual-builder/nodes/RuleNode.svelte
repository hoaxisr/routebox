<script lang="ts">
	import { Handle, Position, type NodeProps } from '@xyflow/svelte';
	import type { RouteRule } from '$lib/types';

	// Extended rule data with index
	type RuleNodeData = RouteRule & { index: number };

	let { data, selected }: NodeProps = $props();

	// Cast data to proper type
	const ruleData = data as unknown as RuleNodeData;

	// Get a summary of the rule conditions
	function getConditionsSummary(): string[] {
		const summary: string[] = [];

		if (ruleData.domain?.length) summary.push(`domain: ${ruleData.domain.length}`);
		if (ruleData.domain_suffix?.length) summary.push(`suffix: ${ruleData.domain_suffix.length}`);
		if (ruleData.domain_keyword?.length) summary.push(`keyword: ${ruleData.domain_keyword.length}`);
		if (ruleData.ip_cidr?.length) summary.push(`ip: ${ruleData.ip_cidr.length}`);
		if (ruleData.port?.length) summary.push(`port: ${ruleData.port.join(', ')}`);
		if (ruleData.rule_set?.length) summary.push(`rule_set: ${ruleData.rule_set.length}`);
		if (ruleData.protocol?.length) summary.push(`protocol: ${ruleData.protocol.join(', ')}`);
		if (ruleData.network) summary.push(`network: ${ruleData.network}`);
		if (ruleData.inbound?.length) summary.push(`inbound: ${ruleData.inbound.length}`);
		if (ruleData.process_name?.length) summary.push(`process: ${ruleData.process_name.length}`);

		// For logical rules
		if (ruleData.type === 'logical' && ruleData.rules?.length) {
			summary.push(`${ruleData.mode?.toUpperCase()}: ${ruleData.rules.length} conditions`);
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
</script>

<div class="rule-node" class:selected>
	<div class="rule-header">
		<span class="rule-index">#{ruleData.index + 1}</span>
		<span class="rule-action {actionClass}">{actionLabel}</span>
	</div>

	<div class="rule-conditions">
		{#each conditions.slice(0, 3) as condition}
			<div class="condition-line">{condition}</div>
		{/each}
		{#if conditions.length > 3}
			<div class="condition-more">+{conditions.length - 3} more</div>
		{/if}
	</div>

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

	.rule-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 0.5rem;
		padding-bottom: 0.5rem;
		border-bottom: 1px solid var(--ctp-surface0);
	}

	.rule-index {
		color: var(--ctp-overlay1);
		font-weight: 500;
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

	:global(.rule-node .svelte-flow__handle) {
		width: 10px;
		height: 10px;
		background: var(--ctp-primary);
		border: 2px solid var(--ctp-mantle);
	}
</style>
