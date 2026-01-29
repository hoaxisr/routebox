<script lang="ts">
	import {
		SvelteFlow,
		Background,
		Controls,
		MiniMap,
		type Node,
		type Edge,
		type NodeTypes,
		Position,
		BackgroundVariant
	} from '@xyflow/svelte';
	import '@xyflow/svelte/dist/style.css';

	import type { RouteRule, Outbound } from '$lib/types';
	import RuleNode from './nodes/RuleNode.svelte';
	import OutboundNode from './nodes/OutboundNode.svelte';
	import { layoutNodes } from './utils/layoutEngine';
	import { rulesToNodes, nodesToEdges } from './utils/ruleConverter';

	interface Props {
		rules: RouteRule[];
		outbounds: Outbound[];
		onRuleSelect?: (ruleIndex: number | null) => void;
		onRuleUpdate?: (ruleIndex: number, rule: RouteRule) => void;
	}

	let { rules, outbounds, onRuleSelect, onRuleUpdate }: Props = $props();

	const nodeTypes: NodeTypes = {
		rule: RuleNode,
		outbound: OutboundNode
	};

	// Convert rules and outbounds to nodes
	let nodes = $state.raw<Node[]>([]);
	let edges = $state.raw<Edge[]>([]);

	// Selected node
	let selectedNodeId = $state<string | null>(null);

	// Reactively update nodes/edges when rules or outbounds change
	$effect(() => {
		const ruleNodes = rulesToNodes(rules);
		const outboundNodes = outbounds.map((outbound, index) => ({
			id: `outbound-${outbound.tag}`,
			type: 'outbound',
			position: { x: 0, y: 0 },
			data: {
				tag: outbound.tag,
				type: outbound.type,
				index
			},
			targetPosition: Position.Left
		}));

		const allNodes = layoutNodes(ruleNodes, outboundNodes);
		nodes = allNodes;
		edges = nodesToEdges(rules);
	});

	function handleNodeClick({ node }: { node: Node; event: MouseEvent | TouchEvent }) {
		selectedNodeId = node.id;

		if (node.type === 'rule' && onRuleSelect) {
			onRuleSelect(node.data.index as number);
		}
	}

	function handlePaneClick(_: { event: MouseEvent }) {
		selectedNodeId = null;
		if (onRuleSelect) {
			onRuleSelect(null);
		}
	}
</script>

<div class="visual-builder">
	<SvelteFlow
		{nodes}
		{edges}
		{nodeTypes}
		fitView
		nodesDraggable={false}
		nodesConnectable={false}
		elementsSelectable={true}
		onnodeclick={handleNodeClick}
		onpaneclick={handlePaneClick}
	>
		<Background variant={BackgroundVariant.Dots} gap={20} />
		<Controls position="bottom-left" />
		<MiniMap position="bottom-right" />
	</SvelteFlow>
</div>

<style>
	.visual-builder {
		width: 100%;
		height: 600px;
		border: 1px solid var(--ctp-surface0);
		border-radius: 0.5rem;
		overflow: hidden;
		background: var(--ctp-base);
	}

	:global(.visual-builder .svelte-flow) {
		background: var(--ctp-base);
	}

	:global(.visual-builder .svelte-flow__minimap) {
		background: var(--ctp-mantle);
		border: 1px solid var(--ctp-surface0);
		border-radius: 0.375rem;
	}

	:global(.visual-builder .svelte-flow__controls) {
		background: var(--ctp-mantle);
		border: 1px solid var(--ctp-surface0);
		border-radius: 0.375rem;
	}

	:global(.visual-builder .svelte-flow__controls button) {
		background: var(--ctp-surface0);
		border-color: var(--ctp-surface1);
		color: var(--ctp-text);
	}

	:global(.visual-builder .svelte-flow__controls button:hover) {
		background: var(--ctp-surface1);
	}

	:global(.visual-builder .svelte-flow__edge-path) {
		stroke: var(--ctp-overlay1);
		stroke-width: 2;
	}

	:global(.visual-builder .svelte-flow__edge.selected .svelte-flow__edge-path) {
		stroke: var(--ctp-primary);
	}
</style>
