<script lang="ts">
	import { t } from 'svelte-i18n';

	interface Props {
		/** The outbound/endpoint tag being measured — named in the aria-label. */
		tag: string;
		running: boolean;
		disabled?: boolean;
		onclick: () => void;
	}

	let { tag, running, disabled = false, onclick }: Props = $props();
</script>

<!-- A gauge, not the lightning bolt: on the Proxies page the bolt already means
     the latency probe, and two bolts side by side would say the same thing twice. -->
<button
	{onclick}
	disabled={running || disabled}
	class="p-2 hover:bg-[var(--ctp-surface2)] rounded-lg transition-colors disabled:opacity-40"
	title={$t('outbounds.speedTest')}
	aria-label="{$t('outbounds.speedTest')} {tag}"
>
	{#if running}
		<svg class="w-5 h-5 animate-spin text-[var(--ctp-primary)]" fill="none" viewBox="0 0 24 24">
			<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
			<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
		</svg>
	{:else}
		<svg class="w-5 h-5 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
			<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4.5 18a7.5 7.5 0 1115 0" />
			<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 14.5l3.5-4" />
		</svg>
	{/if}
</button>
