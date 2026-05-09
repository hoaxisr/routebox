<script lang="ts">
	import { t } from 'svelte-i18n';
	import Modal from '$lib/components/shared/Modal.svelte';

	interface Props {
		open: boolean;
		busy?: boolean;
		onClose: () => void;
		onConfirm: () => void;
	}

	let { open, busy = false, onClose, onConfirm }: Props = $props();
</script>

<Modal {open} {onClose} title={$t('breakdown.resetConfirmTitle')} size="sm">
	<p class="text-sm text-[var(--ctp-text)]">
		{$t('breakdown.resetConfirmBody')}
	</p>

	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<button
				type="button"
				onclick={onClose}
				disabled={busy}
				class="px-3 py-1.5 text-sm rounded-md border border-[var(--ctp-surface2)] text-[var(--ctp-text)] hover:bg-[var(--ctp-surface1)] disabled:opacity-50"
			>
				{$t('common.cancel')}
			</button>
			<button
				type="button"
				onclick={onConfirm}
				disabled={busy}
				class="action-btn-danger px-3 py-1.5 text-sm rounded-md disabled:opacity-50"
			>
				{busy ? $t('common.loading') : $t('breakdown.resetConfirmButton')}
			</button>
		</div>
	{/snippet}
</Modal>
