<script lang="ts">
	import { t } from 'svelte-i18n';

	interface Props {
		onEdit?: () => void;
		onDelete?: () => void;
		editTitle?: string;
		deleteTitle?: string;
	}

	let { onEdit, onDelete, editTitle, deleteTitle }: Props = $props();
</script>

<div class="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
	{#if onEdit}
		<button
			type="button"
			onclick={(e) => { e.stopPropagation(); onEdit?.(); }}
			class="p-1.5 rounded-md hover:bg-[var(--ctp-surface2)] text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)] transition-colors"
			title={editTitle || $t('common.edit')}
		>
			<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
			</svg>
		</button>
	{/if}
	{#if onDelete}
		<button
			type="button"
			onclick={(e) => { e.stopPropagation(); onDelete?.(); }}
			class="action-btn-danger"
			title={deleteTitle || $t('common.delete')}
		>
			<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
			</svg>
		</button>
	{/if}
</div>
