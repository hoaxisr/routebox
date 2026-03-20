<script lang="ts">
	import { t } from 'svelte-i18n';

	let {
		tlsFragment = $bindable(false),
		tlsFragmentFallbackDelay = $bindable(''),
		tlsRecordFragment = $bindable(false)
	}: {
		tlsFragment: boolean;
		tlsFragmentFallbackDelay: string;
		tlsRecordFragment: boolean;
	} = $props();
</script>

<div class="space-y-4">
	<!-- TLS Fragment -->
	<div class="p-3 rounded-lg bg-[var(--ctp-mantle)] border border-[var(--ctp-surface0)]">
		<label class="flex items-center gap-3 cursor-pointer">
			<input type="checkbox" bind:checked={tlsFragment} class="toggle-checkbox" />
			<div>
				<span class="text-sm font-medium text-[var(--ctp-text)]">{$t('routes.tlsFragment')}</span>
				<p class="text-xs text-[var(--ctp-overlay0)]">{$t('routes.tlsFragmentHint')}</p>
			</div>
		</label>

		{#if tlsFragment}
			<div class="mt-3 pl-7">
				<label for="tls-frag-delay" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.tlsFragmentFallbackDelay')}</label>
				<input id="tls-frag-delay" type="text" bind:value={tlsFragmentFallbackDelay}
					placeholder="300ms" class="input-field w-full" />
			</div>
		{/if}
	</div>

	<!-- TLS Record Fragment -->
	<div class="p-3 rounded-lg bg-[var(--ctp-mantle)] border border-[var(--ctp-surface0)]">
		<label class="flex items-center gap-3 cursor-pointer">
			<input type="checkbox" bind:checked={tlsRecordFragment} class="toggle-checkbox" />
			<div>
				<span class="text-sm font-medium text-[var(--ctp-text)]">{$t('routes.tlsRecordFragment')}</span>
				<p class="text-xs text-[var(--ctp-overlay0)]">{$t('routes.tlsRecordFragmentHint')}</p>
			</div>
		</label>
	</div>
</div>
