<script lang="ts">
	import { onMount, onDestroy, type Snippet } from 'svelte';

	interface Props {
		open: boolean;
		title: string;
		size?: 'sm' | 'md' | 'lg' | 'xl';
		onClose: () => void;
		children: Snippet;
		footer?: Snippet;
	}

	let { open, title, size = 'md', onClose, children, footer }: Props = $props();

	const sizeClasses = {
		sm: 'max-w-sm',
		md: 'max-w-lg',
		lg: 'max-w-2xl',
		xl: 'max-w-4xl'
	};

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && open) {
			onClose();
		}
	}

	function handleBackdropClick(e: MouseEvent) {
		if (e.target === e.currentTarget) {
			onClose();
		}
	}

	onMount(() => {
		window.addEventListener('keydown', handleKeydown);
	});

	onDestroy(() => {
		window.removeEventListener('keydown', handleKeydown);
	});
</script>

{#if open}
	<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
		onclick={handleBackdropClick}
		onkeydown={handleKeydown}
		role="dialog"
		aria-modal="true"
		aria-labelledby="modal-title"
		tabindex="-1"
	>
		<div
			class="bg-[var(--ctp-base)] rounded-xl shadow-xl w-full {sizeClasses[size]} mx-4 max-h-[90vh] flex flex-col"
		>
			<!-- Header -->
			<div class="flex items-center justify-between px-6 py-4 border-b border-[var(--ctp-surface2)]">
				<h2 id="modal-title" class="text-lg font-semibold text-[var(--ctp-text)]">{title}</h2>
				<button
					onclick={onClose}
					class="p-1 hover:bg-[var(--ctp-surface1)] rounded-lg transition-colors"
					aria-label="Close modal"
				>
					<svg class="w-5 h-5 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>

			<!-- Content -->
			<div class="flex-1 overflow-y-auto p-6">
				{@render children()}
			</div>

			<!-- Footer (optional) -->
			{#if footer}
				<div class="px-6 py-4 border-t border-[var(--ctp-surface2)] bg-[var(--ctp-surface0)]">
					{@render footer()}
				</div>
			{/if}
		</div>
	</div>
{/if}
