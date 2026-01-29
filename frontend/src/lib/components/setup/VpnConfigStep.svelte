<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { ParsedConfig } from '$lib/utils/parsers';
	import FileDropZone from '$lib/components/shared/FileDropZone.svelte';

	interface Props {
		stepNumber: number;
		vpnInput: string;
		parsedVpn: ParsedConfig | null;
		error: string;
		onInput: (value: string) => void;
	}

	let { stepNumber, vpnInput = $bindable(), parsedVpn, error = $bindable(), onInput }: Props = $props();

	function getTypeLabel(type: string): string {
		switch (type) {
			case 'vless': return 'VLESS';
			case 'hy2': return 'Hysteria2';
			case 'awg': return 'AmneziaWG';
			default: return type;
		}
	}

	function handleFileRead(content: string) {
		vpnInput = content;
		onInput(content);
	}
</script>

<div class="space-y-6">
	<div>
		<h2 class="text-xl font-semibold text-[var(--ctp-text)]">{stepNumber}. {$t('setup.vpn.title')}</h2>
		<p class="text-[var(--ctp-overlay1)] mt-1">
			{$t('setup.vpn.description')}
		</p>
	</div>

	<FileDropZone
		bind:value={vpnInput}
		placeholder={$t('setup.vpn.placeholder')}
		accept=".conf,.txt,.json"
		{error}
		onInput={onInput}
		onFileRead={handleFileRead}
		onError={(err) => error = err}
	/>

	{#if parsedVpn}
		<div class="alert-box primary">
			<div class="flex items-center gap-2">
				<svg class="w-5 h-5 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
				</svg>
				<span class="font-medium text-[var(--ctp-primary)]">{$t('setup.vpn.parseSuccess')}</span>
			</div>
			<div class="mt-2 text-sm text-[var(--ctp-text)]">
				<span class="text-[var(--ctp-overlay1)]">{$t('setup.vpn.type')}:</span> {getTypeLabel(parsedVpn.type)} —
				<span class="text-[var(--ctp-overlay1)]">{$t('setup.vpn.name')}:</span> {parsedVpn.name}
			</div>
		</div>
	{/if}
</div>
