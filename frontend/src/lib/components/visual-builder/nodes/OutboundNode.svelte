<script lang="ts">
	import { Handle, Position, type NodeProps } from '@xyflow/svelte';

	interface OutboundNodeData {
		tag: string;
		type: string;
		index: number;
	}

	let { data, selected }: NodeProps = $props();

	// Cast data to proper type
	const outboundData = data as unknown as OutboundNodeData;

	function getTypeColor(): string {
		switch (outboundData.type) {
			case 'direct':
				return 'type-direct';
			case 'block':
				return 'type-block';
			case 'selector':
				return 'type-selector';
			case 'urltest':
				return 'type-urltest';
			default:
				return 'type-proxy';
		}
	}

	function getTypeIcon(): string {
		switch (outboundData.type) {
			case 'direct':
				return 'D';
			case 'block':
				return 'B';
			case 'selector':
				return 'S';
			case 'urltest':
				return 'U';
			case 'wireguard':
			case 'amnezia-wg':
				return 'W';
			default:
				return 'P';
		}
	}

	const typeColor = $derived(getTypeColor());
	const typeIcon = $derived(getTypeIcon());
</script>

<div class="outbound-node" class:selected>
	<Handle type="target" position={Position.Left} />

	<div class="outbound-content">
		<div class="outbound-icon {typeColor}">
			{typeIcon}
		</div>
		<div class="outbound-info">
			<div class="outbound-tag">{outboundData.tag}</div>
			<div class="outbound-type">{outboundData.type}</div>
		</div>
	</div>
</div>

<style>
	.outbound-node {
		background: var(--ctp-mantle);
		border: 2px solid var(--ctp-surface1);
		border-radius: 0.5rem;
		padding: 0.75rem;
		min-width: 140px;
		font-size: 0.75rem;
		transition: all 0.15s ease;
	}

	.outbound-node:hover {
		border-color: var(--ctp-overlay0);
	}

	.outbound-node.selected {
		border-color: var(--ctp-primary);
		box-shadow: 0 0 0 2px color-mix(in srgb, var(--ctp-primary) 25%, transparent);
	}

	.outbound-content {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	.outbound-icon {
		width: 2rem;
		height: 2rem;
		border-radius: 0.375rem;
		display: flex;
		align-items: center;
		justify-content: center;
		font-weight: 700;
		font-size: 0.875rem;
		color: white;
		flex-shrink: 0;
	}

	.type-direct {
		background: var(--ctp-green);
	}

	.type-block {
		background: var(--ctp-red);
	}

	.type-selector {
		background: var(--ctp-blue);
	}

	.type-urltest {
		background: var(--ctp-lavender);
	}

	.type-proxy {
		background: var(--ctp-primary);
	}

	.outbound-info {
		min-width: 0;
	}

	.outbound-tag {
		color: var(--ctp-text);
		font-weight: 600;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.outbound-type {
		color: var(--ctp-overlay1);
		font-size: 0.65rem;
		text-transform: uppercase;
	}

	:global(.outbound-node .svelte-flow__handle) {
		width: 10px;
		height: 10px;
		background: var(--ctp-overlay1);
		border: 2px solid var(--ctp-mantle);
	}
</style>
