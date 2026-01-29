<script lang="ts">
	import { t } from 'svelte-i18n';

	interface Props {
		method: 'default' | 'drop';
		noDrop: boolean;
	}

	let {
		method = $bindable(),
		noDrop = $bindable()
	}: Props = $props();
</script>

<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-3">
	<div>
		<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.rejectMethod')}</label>
		<div class="flex gap-2">
			<button type="button" onclick={() => method = 'default'}
				class="flex-1 toggle-btn {method === 'default' ? 'selected' : ''}">
				{$t('common.default')}
			</button>
			<button type="button" onclick={() => method = 'drop'}
				class="flex-1 toggle-btn {method === 'drop' ? 'selected' : ''}">
				{$t('routes.rejectDrop')}
			</button>
		</div>
		<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.rejectMethodHint')}</p>
	</div>
	<label class="flex items-center gap-2 cursor-pointer">
		<input type="checkbox" bind:checked={noDrop}
			class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]" />
		<div>
			<span class="text-sm text-[var(--ctp-text)]">{$t('routes.rejectNoDrop')}</span>
			<p class="text-xs text-[var(--ctp-overlay0)]">{$t('routes.rejectNoDropHint')}</p>
		</div>
	</label>
</div>
