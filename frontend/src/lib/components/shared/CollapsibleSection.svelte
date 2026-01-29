<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		title: string;
		count?: number;
		expanded?: boolean;
		onToggle?: () => void;
		headerAction?: Snippet;
		children: Snippet;
	}

	let { title, count, expanded = $bindable(false), onToggle, headerAction, children }: Props = $props();

	function handleToggle() {
		expanded = !expanded;
		onToggle?.();
	}
</script>

<div class="bg-[var(--ctp-surface0)] rounded-xl overflow-hidden">
	<div class="px-4 py-3 bg-[var(--ctp-surface1)] border-b border-[var(--ctp-surface2)] flex items-center justify-between">
		<button
			onclick={handleToggle}
			class="flex items-center gap-2 hover:text-[var(--ctp-text)] transition-colors"
		>
			<svg
				class="w-4 h-4 text-[var(--ctp-overlay1)] transition-transform {expanded ? 'rotate-90' : ''}"
				fill="none"
				stroke="currentColor"
				viewBox="0 0 24 24"
			>
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
			</svg>
			<span class="font-medium text-[var(--ctp-subtext1)]">{title}</span>
			{#if count !== undefined}
				<span class="text-sm text-[var(--ctp-overlay0)]">({count})</span>
			{/if}
		</button>
		{#if headerAction}
			{@render headerAction()}
		{/if}
	</div>

	{#if expanded}
		{@render children()}
	{/if}
</div>
