<script lang="ts">
	import { Handle, Position, type NodeProps } from '@xyflow/svelte';

	interface DnsServerNodeData {
		tag: string;
		type: string;
		server?: string;
		detour?: string;
		index: number;
		isFinal?: boolean;
	}

	let { data, selected }: NodeProps = $props();

	const serverData = data as unknown as DnsServerNodeData;
	const isFinal = $derived(serverData.isFinal ?? false);
	const hasDetour = $derived(!!serverData.detour);

	function getTypeColor(): string {
		switch (serverData.type) {
			case 'local':
				return 'type-local';
			case 'fakeip':
				return 'type-fakeip';
			case 'https':
				return 'type-doh';
			case 'tls':
				return 'type-dot';
			default:
				return 'type-dns';
		}
	}

	function getTypeIcon(): string {
		switch (serverData.type) {
			case 'local':
				return 'L';
			case 'fakeip':
				return 'F';
			case 'https':
				return 'H';
			case 'tls':
				return 'T';
			case 'udp':
				return 'U';
			case 'tcp':
				return 'C';
			default:
				return 'D';
		}
	}

	const typeColor = $derived(getTypeColor());
	const typeIcon = $derived(getTypeIcon());
</script>

<div class="dns-server-node" class:selected class:is-final={isFinal}>
	<Handle type="target" position={Position.Left} />

	<div class="server-content">
		<div class="server-icon {typeColor}">
			{typeIcon}
		</div>
		<div class="server-info">
			<div class="server-tag-row">
				<span class="server-tag">{serverData.tag}</span>
				{#if isFinal}
					<span class="default-badge">default</span>
				{/if}
			</div>
			<div class="server-type">{serverData.type}</div>
			{#if hasDetour}
				<div class="server-detour">
					<span class="detour-badge">{serverData.detour}</span>
				</div>
			{/if}
		</div>
	</div>
</div>

<style>
	.dns-server-node {
		background: var(--ctp-mantle);
		border: 2px solid var(--ctp-surface1);
		border-radius: 0.5rem;
		padding: 0.75rem;
		min-width: 140px;
		font-size: 0.75rem;
		transition: all 0.15s ease;
	}

	.dns-server-node:hover {
		border-color: var(--ctp-overlay0);
		transform: translateY(-1px);
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
	}

	.dns-server-node.selected {
		border-color: var(--ctp-primary);
		box-shadow: 0 0 0 2px color-mix(in srgb, var(--ctp-primary) 25%, transparent);
	}

	.dns-server-node.is-final {
		border-color: var(--ctp-yellow);
		box-shadow: 0 0 0 1px color-mix(in srgb, var(--ctp-yellow) 20%, transparent);
	}

	.server-content {
		display: flex;
		align-items: flex-start;
		gap: 0.75rem;
	}

	.server-icon {
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

	.type-local {
		background: var(--ctp-green);
	}

	.type-fakeip {
		background: var(--ctp-mauve);
	}

	.type-doh {
		background: var(--ctp-blue);
	}

	.type-dot {
		background: var(--ctp-teal);
	}

	.type-dns {
		background: var(--ctp-primary);
	}

	.server-info {
		min-width: 0;
	}

	.server-tag-row {
		display: flex;
		align-items: center;
		gap: 0.375rem;
	}

	.server-tag {
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

	.server-type {
		color: var(--ctp-overlay1);
		font-size: 0.65rem;
		text-transform: uppercase;
	}

	.server-detour {
		margin-top: 0.25rem;
	}

	.detour-badge {
		padding: 0.0625rem 0.375rem;
		border-radius: 0.25rem;
		font-size: 0.5625rem;
		font-weight: 500;
		background: color-mix(in srgb, var(--ctp-peach) 20%, transparent);
		color: var(--ctp-peach);
		border: 1px solid var(--ctp-peach);
	}

	:global(.dns-server-node .svelte-flow__handle) {
		width: 10px;
		height: 10px;
		background: var(--ctp-overlay1);
		border: 2px solid var(--ctp-mantle);
	}
</style>
