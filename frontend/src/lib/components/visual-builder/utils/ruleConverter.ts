import type { Node, Edge } from '@xyflow/svelte';
import { Position } from '@xyflow/svelte';
import type { RouteRule } from '$lib/types';

interface RuleNodeCallbacks {
	onMoveUp?: (index: number) => void;
	onMoveDown?: (index: number) => void;
}

/**
 * Convert route rules to Svelte Flow nodes
 */
export function rulesToNodes(rules: RouteRule[], callbacks?: RuleNodeCallbacks): Node[] {
	const totalRules = rules.length;
	return rules.map((rule, index) => ({
		id: `rule-${index}`,
		type: 'rule',
		position: { x: 0, y: 0 }, // Will be set by layoutEngine
		data: {
			...rule,
			index,
			totalRules,
			onMoveUp: callbacks?.onMoveUp,
			onMoveDown: callbacks?.onMoveDown
		},
		sourcePosition: Position.Right
	}));
}

/**
 * Convert rules to edges (connections from rules to outbounds)
 */
export function nodesToEdges(rules: RouteRule[]): Edge[] {
	const edges: Edge[] = [];

	rules.forEach((rule, index) => {
		// Skip rules without outbound (like reject, sniff, etc.)
		const targetOutbound = getTargetOutbound(rule);
		if (!targetOutbound) return;

		edges.push({
			id: `edge-${index}`,
			source: `rule-${index}`,
			target: `outbound-${targetOutbound}`,
			animated: false,
			style: 'stroke-width: 2px;'
		});
	});

	return edges;
}

/**
 * Get the target outbound for a rule
 */
function getTargetOutbound(rule: RouteRule): string | null {
	// Rules with explicit outbound
	if (rule.outbound) {
		return rule.outbound;
	}

	// Actions that don't have outbound
	if (
		rule.action === 'reject' ||
		rule.action === 'hijack-dns' ||
		rule.action === 'sniff' ||
		rule.action === 'resolve'
	) {
		return null;
	}

	return null;
}

/**
 * Get a human-readable summary of rule conditions
 */
export function getRuleConditionsSummary(rule: RouteRule): string {
	const parts: string[] = [];

	if (rule.domain?.length) parts.push(`domain:${rule.domain.length}`);
	if (rule.domain_suffix?.length) parts.push(`suffix:${rule.domain_suffix.length}`);
	if (rule.domain_keyword?.length) parts.push(`keyword:${rule.domain_keyword.length}`);
	if (rule.ip_cidr?.length) parts.push(`ip:${rule.ip_cidr.length}`);
	if (rule.port?.length) parts.push(`port:${rule.port.join(',')}`);
	if (rule.rule_set?.length) parts.push(`ruleset:${rule.rule_set.length}`);
	if (rule.protocol?.length) parts.push(`proto:${rule.protocol.join(',')}`);
	if (rule.network) parts.push(`net:${rule.network}`);

	if (rule.type === 'logical' && rule.rules?.length) {
		parts.push(`${rule.mode}:${rule.rules.length}`);
	}

	return parts.length > 0 ? parts.join(' ') : 'any';
}
