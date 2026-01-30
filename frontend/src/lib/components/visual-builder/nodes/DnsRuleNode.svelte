<script lang="ts">
	import { Handle, Position, type NodeProps } from '@xyflow/svelte';
	import type { DnsRule } from '$lib/types';

	type DnsRuleNodeData = DnsRule & {
		index: number;
		totalRules: number;
		onMoveUp?: (index: number) => void;
		onMoveDown?: (index: number) => void;
	};

	let { data, selected }: NodeProps = $props();

	const ruleData = data as unknown as DnsRuleNodeData;

	function handleMoveUp(e: MouseEvent) {
		e.stopPropagation();
		ruleData.onMoveUp?.(ruleData.index);
	}

	function handleMoveDown(e: MouseEvent) {
		e.stopPropagation();
		ruleData.onMoveDown?.(ruleData.index);
	}

	function getRuleSetBadges(): string[] {
		return ruleData.rule_set ?? [];
	}

	function getConditionsSummary(): string[] {
		const summary: string[] = [];

		if (ruleData.domain?.length) summary.push(`domain: ${ruleData.domain.length}`);
		if (ruleData.domain_suffix?.length) summary.push(`suffix: ${ruleData.domain_suffix.length}`);
		if (ruleData.domain_keyword?.length) summary.push(`keyword: ${ruleData.domain_keyword.length}`);
		if (ruleData.domain_regex?.length) summary.push(`regex: ${ruleData.domain_regex.length}`);
		if (ruleData.ip_cidr?.length) summary.push(`ip: ${ruleData.ip_cidr.length}`);
		if (ruleData.query_type?.length) summary.push(`type: ${ruleData.query_type.join(', ')}`);

		return summary.length > 0 ? summary : ['(any)'];
	}

	function getActionLabel(): string {
		if (ruleData.action === 'reject') return 'REJECT';
		if (ruleData.action === 'predefined') return 'PREDEFINED';
		return ruleData.server || 'ROUTE';
	}

	function getActionClass(): string {
		if (ruleData.action === 'reject') return 'action-reject';
		if (ruleData.action === 'predefined') return 'action-special';
		return 'action-route';
	}

	const conditions = $derived(getConditionsSummary());
	const actionLabel = $derived(getActionLabel());
	const actionClass = $derived(getActionClass());
	const ruleSetBadges = $derived(getRuleSetBadges());
	const canMoveUp = $derived(ruleData.index > 0);
	const canMoveDown = $derived(ruleData.index < ruleData.totalRules - 1);
</script>

<div class="dns-rule-node" class:selected>
	<div class="rule-header">
		<span class="rule-index">#{ruleData.index + 1}</span>
		<span class="rule-action {actionClass}">{actionLabel}</span>
	</div>

	{#if ruleSetBadges.length > 0}
		<div class="badges-section">
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
	.dns-rule-node {
		background: var(--ctp-mantle);
		border: 2px solid var(--ctp-surface1);
		border-radius: 0.5rem;
		padding: 0.75rem;
		min-width: 180px;
		max-width: 220px;
		font-size: 0.75rem;
		transition: all 0.15s ease;
		position: relative;
		border-left: 3px solid var(--ctp-sapphire);
	}

	.dns-rule-node:hover {
		border-color: var(--ctp-overlay0);
		border-left-color: var(--ctp-sapphire);
		transform: translateY(-1px);
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
	}

	.dns-rule-node.selected {
		border-color: var(--ctp-primary);
		border-left-color: var(--ctp-sapphire);
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
		background: var(--ctp-sapphire);
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
		background: var(--ctp-sapphire);
		border-color: var(--ctp-sapphire);
		color: white;
	}

	.reorder-btn:disabled {
		opacity: 0.3;
		cursor: not-allowed;
	}

	:global(.dns-rule-node .svelte-flow__handle) {
		width: 10px;
		height: 10px;
		background: var(--ctp-sapphire);
		border: 2px solid var(--ctp-mantle);
	}
</style>
