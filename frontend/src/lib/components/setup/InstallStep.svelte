<script lang="ts">
	import { t } from 'svelte-i18n';

	interface Props {
		checking: boolean;
		onCheck: () => void;
	}

	let { checking, onCheck }: Props = $props();
</script>

<div class="space-y-6">
	<div>
		<h2 class="text-xl font-semibold text-[var(--ctp-text)]">1. {$t('setup.install.title')}</h2>
		<p class="text-[var(--ctp-overlay1)] mt-1">
			{$t('setup.install.description')}
		</p>
	</div>

	<div class="alert-box warning">
		<div class="flex items-center gap-2">
			<svg class="w-5 h-5 text-[var(--ctp-yellow)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
			</svg>
			<span class="font-medium text-[var(--ctp-yellow)]">{$t('setup.install.notFound')}</span>
		</div>
	</div>

	<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
		<p class="text-sm text-[var(--ctp-text)]">{$t('setup.install.commandHint')}</p>

		<code class="block p-3 bg-[var(--ctp-crust)] rounded text-sm text-[var(--ctp-green)] font-mono break-all">
			curl -fsSL https://raw.githubusercontent.com/hoaxisr/amnezia-box/main/docs/installation/tools/install-amnezia-box.sh | sh
		</code>

		<p class="text-xs text-[var(--ctp-overlay1)]">
			{$t('setup.install.afterInstall')}
		</p>
	</div>

	<button
		onclick={onCheck}
		disabled={checking}
		class="w-full px-6 py-3 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 disabled:opacity-50 transition-opacity flex items-center justify-center gap-2"
	>
		{#if checking}
			<svg class="w-5 h-5 animate-spin" fill="none" viewBox="0 0 24 24">
				<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
				<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
			</svg>
			{$t('setup.install.checking')}
		{:else}
			<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
			</svg>
			{$t('setup.install.checkButton')}
		{/if}
	</button>
</div>
