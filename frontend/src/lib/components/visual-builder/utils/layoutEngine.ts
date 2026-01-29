import type { Node } from '@xyflow/svelte';

const RULE_COLUMN_X = 50;
const OUTBOUND_COLUMN_X = 400;
const NODE_VERTICAL_GAP = 100;
const NODE_HEIGHT_ESTIMATE = 80;

/**
 * Auto-layout nodes in a two-column layout:
 * - Rules on the left
 * - Outbounds on the right
 */
export function layoutNodes(ruleNodes: Node[], outboundNodes: Node[]): Node[] {
	// Position rule nodes on the left
	const positionedRules = ruleNodes.map((node, index) => ({
		...node,
		position: {
			x: RULE_COLUMN_X,
			y: index * NODE_VERTICAL_GAP
		}
	}));

	// Position outbound nodes on the right, centered vertically
	const rulesHeight = ruleNodes.length * NODE_VERTICAL_GAP;
	const outboundsHeight = outboundNodes.length * NODE_VERTICAL_GAP;
	const outboundStartY = Math.max(0, (rulesHeight - outboundsHeight) / 2);

	const positionedOutbounds = outboundNodes.map((node, index) => ({
		...node,
		position: {
			x: OUTBOUND_COLUMN_X,
			y: outboundStartY + index * NODE_VERTICAL_GAP
		}
	}));

	return [...positionedRules, ...positionedOutbounds];
}

/**
 * Calculate optimal viewport for the graph
 */
export function calculateViewport(nodeCount: number): { zoom: number; x: number; y: number } {
	const totalHeight = nodeCount * NODE_VERTICAL_GAP;
	const zoom = Math.min(1, 600 / (totalHeight + 200));

	return {
		zoom: Math.max(0.5, zoom),
		x: 0,
		y: 0
	};
}
