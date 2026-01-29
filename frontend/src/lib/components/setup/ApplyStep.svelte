<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { ParsedConfig } from '$lib/utils/parsers';

	interface Props {
		stepNumber: number;
		parsedVpn: ParsedConfig | null;
		usageMode: 'router' | 'proxy';
		selectedRuleSets: string[];
		routingMode: 'split' | 'all';
		proxyPort: number;
		machineIP: string;
		applying: boolean;
		applied: boolean;
		onApply: () => void;
	}

	let { stepNumber, parsedVpn, usageMode, selectedRuleSets, routingMode, proxyPort, machineIP, applying, applied, onApply }: Props = $props();

	function getTypeLabel(type: string): string {
		switch (type) {
			case 'vless': return 'VLESS';
			case 'hy2': return 'Hysteria2';
			case 'awg': return 'AmneziaWG';
			default: return type;
		}
	}
</script>

<div class="space-y-6">
	<div>
		<h2 class="text-xl font-semibold text-[var(--ctp-text)]">
			{stepNumber}. {$t('setup.apply.title')}
		</h2>
		<p class="text-[var(--ctp-overlay1)] mt-1">
			{$t('setup.apply.description')}
		</p>
	</div>

	{#if !applied}
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-3 text-sm">
			<div class="flex justify-between">
				<span class="text-[var(--ctp-overlay1)]">{$t('setup.apply.vpnLabel')}:</span>
				<span class="text-[var(--ctp-text)]">{parsedVpn?.name} ({getTypeLabel(parsedVpn?.type || '')})</span>
			</div>
			<div class="flex justify-between">
				<span class="text-[var(--ctp-overlay1)]">{$t('setup.apply.modeLabel')}:</span>
				<span class="text-[var(--ctp-text)]">{usageMode === 'router' ? $t('setup.apply.modeRouter') : $t('setup.apply.modeProxy')}</span>
			</div>
			{#if usageMode === 'router'}
				<div class="flex justify-between">
					<span class="text-[var(--ctp-overlay1)]">{$t('setup.apply.ruleSetsLabel')}:</span>
					<span class="text-[var(--ctp-text)]">{selectedRuleSets.length > 0 ? selectedRuleSets.join(', ') : $t('setup.apply.noneSelected')}</span>
				</div>
				<div class="flex justify-between">
					<span class="text-[var(--ctp-overlay1)]">{$t('setup.apply.routingLabel')}:</span>
					<span class="text-[var(--ctp-text)]">{routingMode === 'split' ? $t('setup.apply.routingSplit') : $t('setup.apply.routingAll')}</span>
				</div>
			{:else}
				<div class="flex justify-between">
					<span class="text-[var(--ctp-overlay1)]">{$t('setup.apply.addressLabel')}:</span>
					<span class="text-[var(--ctp-text)]">{machineIP}:{proxyPort} (SOCKS5 + HTTP)</span>
				</div>
			{/if}
		</div>

		<button
			onclick={onApply}
			disabled={applying}
			class="w-full px-6 py-4 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 disabled:opacity-50 transition-opacity flex items-center justify-center gap-2 text-lg font-medium"
		>
			{#if applying}
				<svg class="w-6 h-6 animate-spin" fill="none" viewBox="0 0 24 24">
					<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
					<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
				</svg>
				{$t('setup.apply.applying')}
			{:else}
				<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
				</svg>
				{$t('setup.apply.applyButton')}
			{/if}
		</button>
	{:else}
		<div class="text-center py-8">
			<div class="w-20 h-20 mx-auto mb-4 rounded-full flex items-center justify-center" style="background-color: color-mix(in srgb, var(--ctp-primary) 20%, transparent);">
				<svg class="w-10 h-10 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
				</svg>
			</div>
			<h3 class="text-xl font-semibold text-[var(--ctp-text)]">{$t('setup.apply.complete')}</h3>
			<p class="text-[var(--ctp-overlay1)] mt-2">
				{$t('setup.apply.completeDescription')}
			</p>
		</div>
	{/if}
</div>
