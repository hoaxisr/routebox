<script lang="ts">
	import {
		SvelteFlow,
		Background,
		Controls,
		MiniMap,
		type Node,
		type Edge,
		type NodeTypes,
		type Connection,
		type IsValidConnection,
		Position,
		BackgroundVariant
	} from '@xyflow/svelte';
	import '@xyflow/svelte/dist/style.css';
	import { t } from 'svelte-i18n';

	import type { RouteRule, Outbound } from '$lib/types';
	import RuleNode from './nodes/RuleNode.svelte';
	import OutboundNode from './nodes/OutboundNode.svelte';
	import ContextMenu from './menus/ContextMenu.svelte';
	import { layoutNodes } from './utils/layoutEngine';
	import { rulesToNodes, nodesToEdges } from './utils/ruleConverter';

	interface Props {
		rules: RouteRule[];
		outbounds: Outbound[];
		finalOutbound?: string;
		onRuleSelect?: (ruleIndex: number | null) => void;
		onRuleOutboundChange?: (ruleIndex: number, newOutbound: string) => void;
		onRuleMove?: (fromIndex: number, toIndex: number) => void;
		onRuleCreate?: () => void;
		onRuleDelete?: (ruleIndex: number) => void;
	}

	let { rules, outbounds, finalOutbound, onRuleSelect, onRuleOutboundChange, onRuleMove, onRuleCreate, onRuleDelete }: Props = $props();

	// Context menu state
	let contextMenu = $state<{ x: number; y: number; type: 'pane' | 'rule'; ruleIndex?: number } | null>(null);

	// Move rule callbacks
	function handleMoveUp(index: number) {
		if (index > 0 && onRuleMove) {
			onRuleMove(index, index - 1);
		}
	}

	function handleMoveDown(index: number) {
		if (index < rules.length - 1 && onRuleMove) {
			onRuleMove(index, index + 1);
		}
	}

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
		const ruleNodes = rulesToNodes(rules, {
			onMoveUp: handleMoveUp,
			onMoveDown: handleMoveDown
		});
		const outboundNodes = outbounds.map((outbound, index) => ({
			id: `outbound-${outbound.tag}`,
			type: 'outbound',
			position: { x: 0, y: 0 },
			data: {
				tag: outbound.tag,
				type: outbound.type,
				index,
				isFinal: outbound.tag === finalOutbound
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
		contextMenu = null;
		if (onRuleSelect) {
			onRuleSelect(null);
		}
	}

	function handlePaneContextMenu({ event }: { event: MouseEvent }) {
		event.preventDefault();
		contextMenu = {
			x: event.clientX,
			y: event.clientY,
			type: 'pane'
		};
	}

	function handleNodeContextMenu({ node, event }: { node: Node; event: MouseEvent }) {
		event.preventDefault();
		if (node.type === 'rule') {
			contextMenu = {
				x: event.clientX,
				y: event.clientY,
				type: 'rule',
				ruleIndex: node.data.index as number
			};
		}
	}

	function closeContextMenu() {
		contextMenu = null;
	}

	// Icons for context menu
	const plusIcon = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14"/></svg>';
	const trashIcon = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 6h18M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2"/></svg>';

	// Context menu items
	let contextMenuItems = $derived.by(() => {
		if (!contextMenu) return [];

		if (contextMenu.type === 'pane') {
			return [
				{
					label: $t('routes.addRule'),
					icon: plusIcon,
					action: () => onRuleCreate?.(),
					disabled: !onRuleCreate
				}
			];
		}

		if (contextMenu.type === 'rule' && contextMenu.ruleIndex !== undefined) {
			const index = contextMenu.ruleIndex;
			return [
				{
					label: $t('common.delete'),
					icon: trashIcon,
					action: () => onRuleDelete?.(index),
					danger: true,
					disabled: !onRuleDelete
				}
			];
		}

		return [];
	});

	// Validate connections: only allow rule → outbound
	const isValidConnection: IsValidConnection = (connection) => {
		const sourceNode = nodes.find((n) => n.id === connection.source);
		const targetNode = nodes.find((n) => n.id === connection.target);

		// Only allow: rule (source) → outbound (target)
		return sourceNode?.type === 'rule' && targetNode?.type === 'outbound';
	};

	// Handle new connection
	function handleConnect(connection: Connection) {
		if (!onRuleOutboundChange) return;

		// Extract rule index from source id (format: "rule-{index}")
		const ruleIndex = parseInt(connection.source.replace('rule-', ''), 10);
		// Extract outbound tag from target id (format: "outbound-{tag}")
		const outboundTag = connection.target.replace('outbound-', '');

		if (!isNaN(ruleIndex) && outboundTag) {
			onRuleOutboundChange(ruleIndex, outboundTag);
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
		nodesConnectable={true}
		elementsSelectable={true}
		clickConnect={true}
		{isValidConnection}
		onconnect={handleConnect}
		onnodeclick={handleNodeClick}
		onpaneclick={handlePaneClick}
		onpanecontextmenu={handlePaneContextMenu}
		onnodecontextmenu={handleNodeContextMenu}
	>
		<Background variant={BackgroundVariant.Dots} gap={20} />
		<Controls position="bottom-left" />
		<MiniMap position="bottom-right" />
	</SvelteFlow>

	{#if contextMenu && contextMenuItems.length > 0}
		<ContextMenu
			x={contextMenu.x}
			y={contextMenu.y}
			items={contextMenuItems}
			onClose={closeContextMenu}
		/>
	{/if}
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
		transition: stroke 0.2s ease, stroke-width 0.2s ease;
	}

	:global(.visual-builder .svelte-flow__edge:hover .svelte-flow__edge-path) {
		stroke: var(--ctp-primary);
		stroke-width: 3;
	}

	:global(.visual-builder .svelte-flow__edge.selected .svelte-flow__edge-path) {
		stroke: var(--ctp-primary);
		stroke-width: 3;
	}

	/* Animated edge marker */
	:global(.visual-builder .svelte-flow__edge-path) {
		marker-end: url(#arrowhead);
	}

	/* Connection line while dragging */
	:global(.visual-builder .svelte-flow__connection-path) {
		stroke: var(--ctp-primary);
		stroke-width: 2;
		stroke-dasharray: 5 5;
		animation: dash 0.5s linear infinite;
	}

	@keyframes dash {
		to {
			stroke-dashoffset: -10;
		}
	}

	/* Handles */
	:global(.visual-builder .svelte-flow__handle) {
		width: 12px;
		height: 12px;
		background: var(--ctp-surface2);
		border: 2px solid var(--ctp-overlay0);
		transition: all 0.15s ease;
	}

	:global(.visual-builder .svelte-flow__handle:hover) {
		background: var(--ctp-primary);
		border-color: var(--ctp-primary);
		transform: scale(1.2);
	}

	:global(.visual-builder .svelte-flow__handle.connecting) {
		background: var(--ctp-primary);
	}

	:global(.visual-builder .svelte-flow__handle-valid) {
		background: var(--ctp-green);
		border-color: var(--ctp-green);
	}

	:global(.visual-builder .svelte-flow__handle-invalid) {
		background: var(--ctp-red);
		border-color: var(--ctp-red);
	}
</style>
