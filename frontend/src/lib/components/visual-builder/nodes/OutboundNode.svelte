<script lang="ts">
	import { Handle, Position, type NodeProps } from '@xyflow/svelte';

	interface OutboundNodeData {
		tag: string;
		type: string;
		index: number;
		isFinal?: boolean;
	}

	let { data, selected }: NodeProps = $props();

	// Cast data to proper type
	const outboundData = data as unknown as OutboundNodeData;
	const isFinal = $derived(outboundData.isFinal ?? false);

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

<div class="outbound-node" class:selected class:is-final={isFinal}>
	<Handle type="target" position={Position.Left} />

	<div class="outbound-content">
		<div class="outbound-icon {typeColor}">
			{typeIcon}
		</div>
		<div class="outbound-info">
			<div class="outbound-tag-row">
				<span class="outbound-tag">{outboundData.tag}</span>
				{#if isFinal}
					<span class="default-badge">default</span>
				{/if}
			</div>
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
		transform: translateY(-1px);
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
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

	.outbound-node.is-final {
		border-color: var(--ctp-yellow);
		box-shadow: 0 0 0 1px color-mix(in srgb, var(--ctp-yellow) 20%, transparent);
	}

	.outbound-info {
		min-width: 0;
	}

	.outbound-tag-row {
		display: flex;
		align-items: center;
		gap: 0.375rem;
	}

	.outbound-tag {
		color: var(--ctp-text);
		font-weight: 600;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.default-badge {
		padding: 0.0625rem 0.25rem;
		border-radius: 0.1875rem;
		font-size: 0.5rem;
		font-weight: 600;
		text-transform: uppercase;
		background: var(--ctp-yellow);
		color: var(--ctp-base);
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
