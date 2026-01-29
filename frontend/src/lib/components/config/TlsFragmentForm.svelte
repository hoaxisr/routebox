<script lang="ts">
	import type { TlsFragment, TlsRecordFragment } from '$lib/types';
	import { t } from 'svelte-i18n';

	let {
		tlsFragment = $bindable<TlsFragment>({}),
		tlsRecordFragment = $bindable<TlsRecordFragment>({})
	}: {
		tlsFragment: TlsFragment;
		tlsRecordFragment: TlsRecordFragment;
	} = $props();

	let fragmentEnabled = $state(tlsFragment?.enabled ?? false);
	let fragmentSize = $state(tlsFragment?.size ?? '');
	let fragmentSleep = $state(tlsFragment?.sleep ?? '');
	let fragmentFallbackDelay = $state(tlsFragment?.fallback_delay ?? '');
	let recordEnabled = $state(tlsRecordFragment?.enabled ?? false);
	let recordSize = $state(tlsRecordFragment?.size ?? '');

	$effect(() => {
		if (fragmentEnabled) {
			tlsFragment = {
				enabled: true,
				...(fragmentSize ? { size: fragmentSize } : {}),
				...(fragmentSleep ? { sleep: fragmentSleep } : {}),
				...(fragmentFallbackDelay ? { fallback_delay: fragmentFallbackDelay } : {})
			};
		} else {
			tlsFragment = {};
		}
	});

	$effect(() => {
		if (recordEnabled) {
			tlsRecordFragment = {
				enabled: true,
				...(recordSize ? { size: recordSize } : {})
			};
		} else {
			tlsRecordFragment = {};
		}
	});
</script>

<div class="space-y-4">
	<!-- TLS Fragment -->
	<div class="p-3 rounded-lg bg-[var(--ctp-mantle)] border border-[var(--ctp-surface0)]">
		<label class="flex items-center gap-3 cursor-pointer">
			<input type="checkbox" bind:checked={fragmentEnabled} class="toggle-checkbox" />
			<div>
				<span class="text-sm font-medium text-[var(--ctp-text)]">{$t('routes.tlsFragment')}</span>
				<p class="text-xs text-[var(--ctp-overlay0)]">{$t('routes.tlsFragmentHint')}</p>
			</div>
		</label>

		{#if fragmentEnabled}
			<div class="mt-3 space-y-3 pl-7">
				<div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
					<div>
						<label for="tls-frag-size" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.tlsFragmentSize')}</label>
						<input id="tls-frag-size" type="text" bind:value={fragmentSize}
							placeholder="40:100" class="input-field w-full" />
						<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.tlsRangeHint')}</p>
					</div>
					<div>
						<label for="tls-frag-sleep" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.tlsFragmentSleep')}</label>
						<input id="tls-frag-sleep" type="text" bind:value={fragmentSleep}
							placeholder="10:15" class="input-field w-full" />
						<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.tlsRangeHint')}</p>
					</div>
				</div>
				<div>
					<label for="tls-frag-delay" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.tlsFragmentFallbackDelay')}</label>
					<input id="tls-frag-delay" type="text" bind:value={fragmentFallbackDelay}
						placeholder="300ms" class="input-field w-full" />
				</div>
			</div>
		{/if}
	</div>

	<!-- TLS Record Fragment -->
	<div class="p-3 rounded-lg bg-[var(--ctp-mantle)] border border-[var(--ctp-surface0)]">
		<label class="flex items-center gap-3 cursor-pointer">
			<input type="checkbox" bind:checked={recordEnabled} class="toggle-checkbox" />
			<div>
				<span class="text-sm font-medium text-[var(--ctp-text)]">{$t('routes.tlsRecordFragment')}</span>
				<p class="text-xs text-[var(--ctp-overlay0)]">{$t('routes.tlsRecordFragmentHint')}</p>
			</div>
		</label>

		{#if recordEnabled}
			<div class="mt-3 pl-7">
				<label for="tls-rec-size" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.tlsFragmentSize')}</label>
				<input id="tls-rec-size" type="text" bind:value={recordSize}
					placeholder="40:100" class="input-field w-full" />
				<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.tlsRangeHint')}</p>
			</div>
		{/if}
	</div>
</div>
