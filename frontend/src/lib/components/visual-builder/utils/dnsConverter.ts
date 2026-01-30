import type { Node, Edge } from '@xyflow/svelte';
import { Position } from '@xyflow/svelte';
import type { DnsRule, DnsServer } from '$lib/types';

interface DnsRuleNodeCallbacks {
	onMoveUp?: (index: number) => void;
	onMoveDown?: (index: number) => void;
}

/**
 * Convert DNS rules to Svelte Flow nodes
 */
export function dnsRulesToNodes(rules: DnsRule[], callbacks?: DnsRuleNodeCallbacks): Node[] {
	const totalRules = rules.length;
	return rules.map((rule, index) => ({
		id: `dns-rule-${index}`,
		type: 'dnsRule',
		position: { x: 0, y: 0 },
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
 * Convert DNS servers to Svelte Flow nodes
 */
export function dnsServersToNodes(servers: DnsServer[], finalServer?: string): Node[] {
	return servers.map((server, index) => ({
		id: `dns-server-${server.tag}`,
		type: 'dnsServer',
		position: { x: 0, y: 0 },
		data: {
			tag: server.tag,
			type: server.type,
			server: server.server,
			detour: server.detour,
			index,
			isFinal: server.tag === finalServer
		},
		targetPosition: Position.Left
	}));
}

/**
 * Convert DNS rules to edges (connections from rules to servers)
 */
export function dnsRulesToEdges(rules: DnsRule[]): Edge[] {
	const edges: Edge[] = [];

	rules.forEach((rule, index) => {
		// Skip rules without server (reject, predefined)
		if (!rule.server || rule.action === 'reject' || rule.action === 'predefined') return;

		edges.push({
			id: `dns-edge-${index}`,
			source: `dns-rule-${index}`,
			target: `dns-server-${rule.server}`,
			animated: false,
			style: 'stroke-width: 2px;'
		});
	});

	return edges;
}

/**
 * Layout DNS nodes in two columns
 */
export function layoutDnsNodes(ruleNodes: Node[], serverNodes: Node[]): Node[] {
	const RULE_X = 50;
	const SERVER_X = 400;
	const START_Y = 50;
	const RULE_SPACING = 120;
	const SERVER_SPACING = 100;

	// Layout rule nodes
	const layoutedRules = ruleNodes.map((node, index) => ({
		...node,
		position: { x: RULE_X, y: START_Y + index * RULE_SPACING }
	}));

	// Layout server nodes
	const layoutedServers = serverNodes.map((node, index) => ({
		...node,
		position: { x: SERVER_X, y: START_Y + index * SERVER_SPACING }
	}));

	return [...layoutedRules, ...layoutedServers];
}
