<script lang="ts">
	import { t } from 'svelte-i18n';
	import { notifications } from '$lib/stores';
	import {
		parseVless,
		parseTrojan,
		parseHysteria2,
		parseShadowsocks,
		parseNaive,
		parseMieruLink,
		type ParsedVless,
		type ParsedTrojan,
		type ParsedHysteria2,
		type ParsedShadowsocks,
		type ParsedNaive,
		type ParsedMieru
	} from '$lib/utils/parsers';

	interface Props {
		protocol: 'vless' | 'trojan' | 'hysteria2' | 'shadowsocks' | 'naive' | 'mieru';
		onImport: (config: ParsedVless | ParsedTrojan | ParsedHysteria2 | ParsedShadowsocks | ParsedNaive | ParsedMieru) => void;
		onClose: () => void;
	}

	let { protocol, onImport, onClose }: Props = $props();

	let importText = $state('');
	let importError = $state('');
	let mieruMixed = $state<{ uri: string; transports: ('TCP' | 'UDP')[] } | null>(null);

	const protocolNames = {
		vless: 'VLESS',
		trojan: 'Trojan',
		hysteria2: 'Hysteria2',
		shadowsocks: 'Shadowsocks',
		naive: 'NaiveProxy',
		mieru: 'Mieru'
	};

	const placeholders = {
		vless: 'vless://uuid@server:port?params#name',
		trojan: 'trojan://password@server:port?params#name',
		hysteria2: 'hy2://password@server:port?params#name',
		shadowsocks: 'ss://BASE64(method:password)@server:port#name',
		naive: 'naive+https://user:password@server:port#name',
		mieru: 'mierus://user:pass@host?profile=name&port=443&protocol=TCP'
	};

	const linkPrefixes = {
		vless: 'vless://',
		trojan: 'trojan://',
		hysteria2: 'hy2:// or hysteria2://',
		shadowsocks: 'ss://',
		naive: 'naive+https:// or naive+quic://',
		mieru: 'mierus://'
	};

	function parseImportLink() {
		importError = '';
		mieruMixed = null;
		const text = importText.trim();

		if (!text) {
			importError = $t('outbounds.importEmptyLink');
			return;
		}

		// Try VLESS
		if (text.startsWith('vless://')) {
			const result = parseVless(text);
			if (!result.success || !result.config) {
				importError = result.error || $t('outbounds.importParseFailed', { values: { protocol: protocolNames.vless } });
				return;
			}
			onImport(result.config as ParsedVless);
			notifications.success($t('outbounds.importSuccess', { values: { protocol: protocolNames.vless } }));
			onClose();
			return;
		}

		// Try Trojan
		if (text.startsWith('trojan://')) {
			const result = parseTrojan(text);
			if (!result.success || !result.config) {
				importError = result.error || $t('outbounds.importParseFailed', { values: { protocol: protocolNames.trojan } });
				return;
			}
			onImport(result.config as ParsedTrojan);
			notifications.success($t('outbounds.importSuccess', { values: { protocol: protocolNames.trojan } }));
			onClose();
			return;
		}

		// Try Hysteria2
		if (text.startsWith('hy2://') || text.startsWith('hysteria2://')) {
			const result = parseHysteria2(text);
			if (!result.success || !result.config) {
				importError = result.error || $t('outbounds.importParseFailed', { values: { protocol: protocolNames.hysteria2 } });
				return;
			}
			onImport(result.config as ParsedHysteria2);
			notifications.success($t('outbounds.importSuccess', { values: { protocol: protocolNames.hysteria2 } }));
			onClose();
			return;
		}

		// Try Shadowsocks
		if (text.startsWith('ss://')) {
			const result = parseShadowsocks(text);
			if (!result.success || !result.config) {
				importError = result.error || $t('outbounds.importParseFailed', { values: { protocol: protocolNames.shadowsocks } });
				return;
			}
			onImport(result.config as ParsedShadowsocks);
			notifications.success($t('outbounds.importSuccess', { values: { protocol: protocolNames.shadowsocks } }));
			onClose();
			return;
		}

		// Try NaiveProxy
		if (text.startsWith('naive+https://') || text.startsWith('naive+quic://')) {
			const result = parseNaive(text);
			if (!result.success || !result.config) {
				importError = result.error || $t('outbounds.importParseFailed', { values: { protocol: protocolNames.naive } });
				return;
			}
			onImport(result.config as ParsedNaive);
			notifications.success($t('outbounds.importSuccess', { values: { protocol: protocolNames.naive } }));
			onClose();
			return;
		}

		// Try Mieru
		if (text.startsWith('mierus://')) {
			const result = parseMieruLink(text);
			if (!result.success && result.mieruTransports) {
				mieruMixed = { uri: text, transports: result.mieruTransports };
				return;
			}
			if (!result.success || !result.config) {
				importError = result.error || $t('outbounds.importParseFailed', { values: { protocol: protocolNames.mieru } });
				return;
			}
			onImport(result.config as ParsedMieru);
			notifications.success($t('outbounds.importSuccess', { values: { protocol: protocolNames.mieru } }));
			onClose();
			return;
		}

		importError = $t('outbounds.importUnknownFormat');
	}

	function chooseMieruTransport(tr: 'TCP' | 'UDP') {
		if (!mieruMixed) return;
		const result = parseMieruLink(mieruMixed.uri, tr);
		mieruMixed = null;
		if (!result.success || !result.config) {
			importError = result.error || $t('outbounds.importParseFailed', { values: { protocol: protocolNames.mieru } });
			return;
		}
		onImport(result.config as ParsedMieru);
		notifications.success($t('outbounds.importSuccess', { values: { protocol: protocolNames.mieru } }));
		onClose();
	}

	function handleClose() {
		importText = '';
		importError = '';
		mieruMixed = null;
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
			oninput={() => (mieruMixed = null)}
			placeholder={placeholders[protocol]}
			rows="4"
			class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm resize-none"
		></textarea>

		{#if importError}
			<p class="mt-2 text-sm text-[var(--ctp-red)]">{importError}</p>
		{/if}

		{#if mieruMixed}
			<div class="mt-3 p-3 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg">
				<p class="text-sm text-[var(--ctp-subtext0)] mb-2">{$t('outbounds.mieruForm.mixedPrompt')}</p>
				<div class="flex gap-2">
					{#each mieruMixed.transports as tr}
						<button
							type="button"
							onclick={() => chooseMieruTransport(tr)}
							class="toggle-btn"
						>
							{tr}
						</button>
					{/each}
				</div>
			</div>
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
