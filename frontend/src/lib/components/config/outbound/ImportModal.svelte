<script lang="ts">
	import { t } from 'svelte-i18n';
	import { notifications } from '$lib/stores';
	import {
		parseVless,
		parseHysteria2,
		parseShadowsocks,
		type ParsedVless,
		type ParsedHysteria2,
		type ParsedShadowsocks,
		type ParsedNaive
	} from '$lib/utils/parsers';

	interface Props {
		protocol: 'vless' | 'hysteria2' | 'shadowsocks' | 'naive';
		onImport: (config: ParsedVless | ParsedHysteria2 | ParsedShadowsocks | ParsedNaive) => void;
		onClose: () => void;
	}

	let { protocol, onImport, onClose }: Props = $props();

	let importText = $state('');
	let importError = $state('');

	const protocolNames = {
		vless: 'VLESS',
		hysteria2: 'Hysteria2',
		shadowsocks: 'Shadowsocks',
		naive: 'Naive'
	};

	const placeholders = {
		vless: 'vless://uuid@server:port?params#name',
		hysteria2: 'hy2://password@server:port?params#name',
		shadowsocks: 'ss://BASE64(method:password)@server:port#name',
		naive: 'naive+https://user:password@server:port#name'
	};

	const linkPrefixes = {
		vless: 'vless://',
		hysteria2: 'hy2:// or hysteria2://',
		shadowsocks: 'ss://',
		naive: 'naive+https://'
	};

	function parseImportLink() {
		importError = '';
		const text = importText.trim();

		if (!text) {
			importError = 'Please paste a link';
			return;
		}

		// Try VLESS
		if (text.startsWith('vless://')) {
			const result = parseVless(text);
			if (!result.success || !result.config) {
				importError = result.error || 'Failed to parse VLESS link';
				return;
			}
			onImport(result.config as ParsedVless);
			notifications.success('VLESS configuration imported');
			onClose();
			return;
		}

		// Try Hysteria2
		if (text.startsWith('hy2://') || text.startsWith('hysteria2://')) {
			const result = parseHysteria2(text);
			if (!result.success || !result.config) {
				importError = result.error || 'Failed to parse Hysteria2 link';
				return;
			}
			onImport(result.config as ParsedHysteria2);
			notifications.success('Hysteria2 configuration imported');
			onClose();
			return;
		}

		// Try Shadowsocks
		if (text.startsWith('ss://')) {
			const result = parseShadowsocks(text);
			if (!result.success || !result.config) {
				importError = result.error || 'Failed to parse Shadowsocks link';
				return;
			}
			onImport(result.config as ParsedShadowsocks);
			notifications.success('Shadowsocks configuration imported');
			onClose();
			return;
		}

		importError = 'Unknown link format. Supported: vless://, hy2://, hysteria2://, ss://';
	}

	function handleClose() {
		importText = '';
		importError = '';
		onClose();
	}
</script>

<div class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
	<div class="bg-[var(--ctp-surface0)] rounded-xl p-6 w-full max-w-lg mx-4 shadow-xl">
		<div class="flex items-center justify-between mb-4">
			<h3 class="text-lg font-semibold text-[var(--ctp-text)]">
				{$t('common.import')} {protocolNames[protocol]} {$t('outbounds.configuration')}
			</h3>
			<button
				onclick={handleClose}
				class="p-1 hover:bg-[var(--ctp-surface1)] rounded transition-colors"
			>
				<svg class="w-5 h-5 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
				</svg>
			</button>
		</div>

		<p class="text-sm text-[var(--ctp-subtext0)] mb-4">
			{$t('outbounds.pasteLink', { values: { protocol: linkPrefixes[protocol] } })}
		</p>

		<textarea
			bind:value={importText}
			placeholder={placeholders[protocol]}
			rows="4"
			class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm resize-none"
		></textarea>

		{#if importError}
			<p class="mt-2 text-sm text-[var(--ctp-red)]">{importError}</p>
		{/if}

		<div class="flex justify-end gap-3 mt-4">
			<button
				onclick={handleClose}
				class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
			>
				{$t('common.cancel')}
			</button>
			<button
				onclick={parseImportLink}
				class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
			>
				{$t('common.import')}
			</button>
		</div>
	</div>
</div>
