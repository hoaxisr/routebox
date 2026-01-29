<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		selected: boolean;
		variant?: 'radio' | 'checkbox';
		size?: 'default' | 'lg';
		onclick: () => void;
		children: Snippet;
	}

	let { selected, variant = 'radio', size = 'default', onclick, children }: Props = $props();
</script>

<button
	{onclick}
	class="selectable-card {size === 'lg' ? 'selectable-card-lg' : ''} {selected ? 'selected' : ''}"
>
	<div class="flex items-start gap-4">
		{#if variant === 'radio'}
			<div
				class="w-6 h-6 rounded-full border-2 flex items-center justify-center mt-0.5 flex-shrink-0"
				class:border-[var(--ctp-primary)]={selected}
				class:border-[var(--ctp-overlay0)]={!selected}
			>
				{#if selected}
					<div class="w-3 h-3 rounded-full bg-[var(--ctp-primary)]"></div>
				{/if}
			</div>
		{:else}
			<div
				class="w-5 h-5 rounded border-2 flex items-center justify-center transition-colors flex-shrink-0"
				class:border-[var(--ctp-primary)]={selected}
				class:bg-[var(--ctp-primary)]={selected}
				class:border-[var(--ctp-overlay0)]={!selected}
			>
				{#if selected}
					<svg class="w-3 h-3 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7" />
					</svg>
				{/if}
			</div>
		{/if}
		<div class="flex-1 text-left">
			{@render children()}
		</div>
	</div>
</button>
