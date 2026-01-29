<script lang="ts">
	import { t } from 'svelte-i18n';

	interface Props {
		timeout: string;
	}

	let { timeout = $bindable() }: Props = $props();
</script>

<div class="bg-[var(--ctp-surface0)] rounded-lg p-4">
	<p class="text-sm text-[var(--ctp-overlay1)] mb-3">{$t('routes.sniffDescription')}</p>
	<div>
		<label for="sniff-timeout" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
			{$t('routes.timeout')}
		</label>
		<input id="sniff-timeout" type="text" bind:value={timeout} placeholder="300ms"
			class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.timeoutHint')}</p>
	</div>
</div>
