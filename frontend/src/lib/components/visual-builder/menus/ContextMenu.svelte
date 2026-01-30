<script lang="ts">
	import { t } from 'svelte-i18n';

	interface MenuItem {
		label: string;
		icon?: string;
		action: () => void;
		danger?: boolean;
		disabled?: boolean;
	}

	interface Props {
		x: number;
		y: number;
		items: MenuItem[];
		onClose: () => void;
	}

	let { x, y, items, onClose }: Props = $props();

	function handleClick(item: MenuItem) {
		if (item.disabled) return;
		item.action();
		onClose();
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') {
			onClose();
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- Backdrop to close on click outside -->
<div
	class="context-backdrop"
	onclick={onClose}
	oncontextmenu={(e) => { e.preventDefault(); onClose(); }}
></div>

<div class="context-menu" style="left: {x}px; top: {y}px;">
	{#each items as item}
		<button
			class="context-item"
			class:danger={item.danger}
			class:disabled={item.disabled}
			onclick={() => handleClick(item)}
			disabled={item.disabled}
		>
			{#if item.icon}
				<span class="context-icon">{@html item.icon}</span>
			{/if}
			<span class="context-label">{item.label}</span>
		</button>
	{/each}
</div>

<style>
	.context-backdrop {
		position: fixed;
		inset: 0;
		z-index: 999;
	}

	.context-menu {
		position: fixed;
		z-index: 1000;
		min-width: 160px;
		background: var(--ctp-mantle);
		border: 1px solid var(--ctp-surface1);
		border-radius: 0.5rem;
		padding: 0.25rem;
		box-shadow: 0 4px 16px rgba(0, 0, 0, 0.2);
	}

	.context-item {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		padding: 0.5rem 0.75rem;
		border: none;
		background: transparent;
		color: var(--ctp-text);
		font-size: 0.8125rem;
		text-align: left;
		border-radius: 0.375rem;
		cursor: pointer;
		transition: background 0.1s ease;
	}

	.context-item:hover:not(:disabled) {
		background: var(--ctp-surface0);
	}

	.context-item.danger {
		color: var(--ctp-red);
	}

	.context-item.danger:hover:not(:disabled) {
		background: color-mix(in srgb, var(--ctp-red) 15%, transparent);
	}

	.context-item:disabled,
	.context-item.disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.context-icon {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 16px;
		height: 16px;
	}

	.context-icon :global(svg) {
		width: 16px;
		height: 16px;
	}

	.context-label {
		flex: 1;
	}
</style>
