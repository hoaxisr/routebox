<script lang="ts">
	import type { Outbound, Endpoint, DnsServer } from '$lib/types';
	import { notifications, featureFlags } from '$lib/stores';
	import { parseVless, parseHysteria2, parseShadowsocks, type ParsedVless, type ParsedHysteria2, type ParsedShadowsocks } from '$lib/utils/parsers';
	import {
		validateRequired,
		validatePort,
		validateUUID,
		validateNonEmptyArray,
		hasValidationErrors,
		isDomain
	} from '$lib/utils';
	import { t } from 'svelte-i18n';
	import HelpTooltip from '$lib/components/shared/HelpTooltip.svelte';

	interface Props {
		outbound?: Outbound;
		endpoints: Endpoint[];
		outbounds: Outbound[];
		dnsServers?: DnsServer[];
		hasDefaultResolver?: boolean;
		onSave: (outbound: Outbound) => void;
		onCancel: () => void;
	}

	let { outbound, endpoints, outbounds, dnsServers = [], hasDefaultResolver = false, onSave, onCancel }: Props = $props();

	// Form state
	let tag = $state(outbound?.tag ?? '');
	let type = $state(outbound?.type ?? 'direct');
	let selectedOutbounds = $state<string[]>(outbound?.outbounds ?? []);
	let defaultOutbound = $state(outbound?.default ?? '');
	let interruptExistConnections = $state(outbound?.interrupt_exist_connections ?? false);

	// VLESS fields
	let server = $state(outbound?.server ?? '');
	let serverPort = $state(outbound?.server_port ?? 443);
	let uuid = $state(outbound?.uuid ?? '');
	let flow = $state(outbound?.flow ?? '');
	// TLS
	let tlsEnabled = $state(outbound?.tls?.enabled ?? true);
	let tlsSni = $state(outbound?.tls?.server_name ?? '');
	let tlsFingerprint = $state(outbound?.tls?.utls?.fingerprint ?? 'chrome');
	let tlsAlpn = $state<string[]>(outbound?.tls?.alpn ?? []);
	// Reality
	let realityEnabled = $state(outbound?.tls?.reality?.enabled ?? false);
	let realityPublicKey = $state(outbound?.tls?.reality?.public_key ?? '');
	let realityShortId = $state(outbound?.tls?.reality?.short_id ?? '');
	// Transport
	let transportType = $state<'tcp' | 'ws' | 'grpc' | 'http'>(outbound?.transport?.type as 'tcp' | 'ws' | 'grpc' | 'http' ?? 'tcp');
	let wsPath = $state(outbound?.transport?.path ?? '/');
	let wsHost = $state(outbound?.transport?.headers?.Host ?? '');
	let grpcServiceName = $state(outbound?.transport?.service_name ?? '');

	// Hysteria2 fields
	let hy2Password = $state(outbound?.password ?? '');
	let hy2Insecure = $state(outbound?.tls?.insecure ?? false);
	let hy2ObfsType = $state(outbound?.obfs?.type ?? '');
	let hy2ObfsPassword = $state(outbound?.obfs?.password ?? '');
	// Hysteria2 port hopping
	let hy2ServerPorts = $state(outbound?.server_ports ?? '');
	let hy2HopInterval = $state(outbound?.hop_interval ?? '');
	let hy2UpMbps = $state(outbound?.up_mbps ?? 0);
	let hy2DownMbps = $state(outbound?.down_mbps ?? 0);

	// Shadowsocks fields
	let ssMethod = $state(outbound?.method ?? '2022-blake3-aes-128-gcm');
	let ssPassword = $state(outbound?.password ?? '');
	let ssPlugin = $state(outbound?.plugin ?? '');
	let ssPluginOpts = $state(outbound?.plugin_opts ?? '');
	let ssNetwork = $state(outbound?.network ?? '');
	let ssUdpOverTcp = $state(outbound?.udp_over_tcp ?? false);
	let ssMuxEnabled = $state(outbound?.multiplex?.enabled ?? false);
	let ssMuxProtocol = $state(outbound?.multiplex?.protocol ?? 'h2mux');
	let ssMuxMaxConns = $state(outbound?.multiplex?.max_connections ?? 0);
	let ssMuxMinStreams = $state(outbound?.multiplex?.min_streams ?? 0);
	let ssMuxMaxStreams = $state(outbound?.multiplex?.max_streams ?? 0);
	let ssMuxPadding = $state(outbound?.multiplex?.padding ?? false);

	// ShadowTLS fields
	let stlsVersion = $state(outbound?.version ?? 3);
	let stlsPassword = $state(outbound?.password ?? '');
	let stlsDetour = $state(outbound?.detour ?? '');
	let stlsTlsSni = $state(outbound?.tls?.server_name ?? '');
	let stlsTlsFingerprint = $state(outbound?.tls?.utls?.fingerprint ?? '');
	let stlsInsecure = $state(outbound?.tls?.insecure ?? false);

	// AnyTLS fields
	let atPassword = $state(outbound?.password ?? '');
	let atTlsSni = $state(outbound?.tls?.server_name ?? '');
	let atTlsFingerprint = $state(outbound?.tls?.utls?.fingerprint ?? '');
	let atInsecure = $state(outbound?.tls?.insecure ?? false);
	let atIdleCheckInterval = $state(outbound?.idle_session_check_interval ?? '');
	let atIdleTimeout = $state(outbound?.idle_session_timeout ?? '');
	let atMinIdleSession = $state(outbound?.min_idle_session ?? 0);

	// URLTest specific fields
	let urltestUrl = $state(outbound?.url ?? '');
	let urltestInterval = $state(outbound?.interval ?? '3m');
	let urltestTolerance = $state(outbound?.tolerance ?? 150);
	let urltestIdleTimeout = $state(outbound?.idle_timeout ?? '');

	// Domain resolver (sing-box 1.12+)
	let domainResolver = $state(outbound?.domain_resolver ?? '');

	// Show domain resolver only when server is a domain and no default resolver
	let showDomainResolver = $derived(
		$featureFlags['domain_resolver'] &&
		isDomain(server) &&
		!hasDefaultResolver
	);

	// Import modal state
	let showImport = $state(false);
	let importText = $state('');
	let importError = $state('');

	let errors = $state<Record<string, string>>({});

	// Available outbounds for selector (exclude self)
	$effect(() => {
		if (type === 'selector' || type === 'urltest') {
			// Include endpoints as available options
			const endpointTags = endpoints.map((e) => e.tag);
			// Include other outbounds (exclude current one being edited)
			const otherOutbounds = outbounds.filter((o) => o.tag !== outbound?.tag).map((o) => o.tag);
			availableForSelection = [...endpointTags, ...otherOutbounds];
		}
	});

	let availableForSelection = $state<string[]>([]);

	function toggleOutboundSelection(t: string) {
		if (selectedOutbounds.includes(t)) {
			selectedOutbounds = selectedOutbounds.filter((o) => o !== t);
			if (defaultOutbound === t) {
				defaultOutbound = selectedOutbounds[0] ?? '';
			}
		} else {
			selectedOutbounds = [...selectedOutbounds, t];
			if (!defaultOutbound) {
				defaultOutbound = t;
			}
		}
	}

	function parseImportLink() {
		importError = '';
		const text = importText.trim();

		if (!text) {
			importError = 'Please paste a VLESS or Hysteria2 link';
			return;
		}

		// Try VLESS
		if (text.startsWith('vless://')) {
			const result = parseVless(text);
			if (!result.success || !result.config) {
				importError = result.error || 'Failed to parse VLESS link';
				return;
			}
			applyVlessConfig(result.config as ParsedVless);
			showImport = false;
			importText = '';
			notifications.success('VLESS configuration imported');
			return;
		}

		// Try Hysteria2
		if (text.startsWith('hy2://') || text.startsWith('hysteria2://')) {
			const result = parseHysteria2(text);
			if (!result.success || !result.config) {
				importError = result.error || 'Failed to parse Hysteria2 link';
				return;
			}
			applyHysteria2Config(result.config as ParsedHysteria2);
			showImport = false;
			importText = '';
			notifications.success('Hysteria2 configuration imported');
			return;
		}

		// Try Shadowsocks
		if (text.startsWith('ss://')) {
			const result = parseShadowsocks(text);
			if (!result.success || !result.config) {
				importError = result.error || 'Failed to parse Shadowsocks link';
				return;
			}
			applyShadowsocksConfig(result.config as ParsedShadowsocks);
			showImport = false;
			importText = '';
			notifications.success('Shadowsocks configuration imported');
			return;
		}

		importError = 'Unknown link format. Supported: vless://, hy2://, hysteria2://, ss://';
	}

	function applyVlessConfig(config: ParsedVless) {
		type = 'vless';
		tag = config.name.replace(/[^a-zA-Z0-9-_]/g, '-');
		server = config.server;
		serverPort = config.port;
		uuid = config.uuid;
		flow = config.flow || '';

		// TLS
		tlsEnabled = config.security === 'tls' || config.security === 'reality';
		tlsSni = config.sni || config.server;
		tlsFingerprint = config.fingerprint || 'chrome';
		tlsAlpn = config.alpn || [];

		// Reality
		realityEnabled = config.security === 'reality';
		realityPublicKey = config.pbk || '';
		realityShortId = config.sid || '';

		// Transport
		transportType = config.transport || 'tcp';
		wsPath = config.path || '/';
		wsHost = config.host || '';
		grpcServiceName = config.serviceName || '';
	}

	function applyHysteria2Config(config: ParsedHysteria2) {
		type = 'hysteria2';
		tag = config.name.replace(/[^a-zA-Z0-9-_]/g, '-');
		server = config.server;
		serverPort = config.port;
		hy2Password = config.password;
		tlsSni = config.sni || config.server;
		hy2Insecure = config.insecure || false;
		hy2ObfsType = config.obfs || '';
		hy2ObfsPassword = config.obfsPassword || '';
	}

	function applyShadowsocksConfig(config: ParsedShadowsocks) {
		type = 'shadowsocks';
		tag = config.name.replace(/[^a-zA-Z0-9-_]/g, '-');
		server = config.server;
		serverPort = config.port;
		ssMethod = config.method;
		ssPassword = config.password;
		ssPlugin = config.plugin || '';
		ssPluginOpts = config.pluginOpts || '';
	}

	function validate(): boolean {
		errors = {};

		// Tag is always required
		const tagResult = validateRequired(tag, 'Tag');
		if (!tagResult.valid) errors['tag'] = tagResult.error!;

		// Selector/URLTest validation
		if (type === 'selector' || type === 'urltest') {
			const outboundsResult = validateNonEmptyArray(selectedOutbounds, 'Outbounds');
			if (!outboundsResult.valid) errors['outbounds'] = outboundsResult.error!;
		}

		// Validate server-based outbound types
		const serverBasedTypes = ['vless', 'hysteria2', 'shadowsocks', 'shadowtls', 'anytls'];
		if (serverBasedTypes.includes(type)) {
			const serverResult = validateRequired(server, 'Server');
			if (!serverResult.valid) errors['server'] = serverResult.error!;

			const portResult = validatePort(serverPort);
			if (!portResult.valid) errors['serverPort'] = portResult.error!;
		}

		// Type-specific validation
		if (type === 'vless') {
			const uuidResult = validateUUID(uuid);
			if (!uuidResult.valid) errors['uuid'] = uuidResult.error!;

			if (realityEnabled) {
				const pbkResult = validateRequired(realityPublicKey, 'Reality public key');
				if (!pbkResult.valid) errors['realityPublicKey'] = pbkResult.error!;
			}
		}

		if (type === 'hysteria2') {
			const pwResult = validateRequired(hy2Password, 'Password');
			if (!pwResult.valid) errors['password'] = pwResult.error!;
		}

		if (type === 'shadowsocks') {
			const methodResult = validateRequired(ssMethod, 'Method');
			if (!methodResult.valid) errors['method'] = methodResult.error!;

			const pwResult = validateRequired(ssPassword, 'Password');
			if (!pwResult.valid) errors['password'] = pwResult.error!;
		}

		if (type === 'shadowtls' && stlsVersion >= 2) {
			const pwResult = validateRequired(stlsPassword, 'Password');
			if (!pwResult.valid) errors['password'] = pwResult.error!;
		}

		if (type === 'anytls') {
			const pwResult = validateRequired(atPassword, 'Password');
			if (!pwResult.valid) errors['password'] = pwResult.error!;
		}

		if (hasValidationErrors(errors)) {
			notifications.error(errors[Object.keys(errors)[0]]);
			return false;
		}

		return true;
	}

	function handleSubmit() {
		if (!validate()) return;

		const ob: Outbound = {
			type,
			tag: tag.trim()
		};

		if (type === 'selector' || type === 'urltest') {
			ob.outbounds = selectedOutbounds;
			if (defaultOutbound) {
				ob.default = defaultOutbound;
			}
			if (interruptExistConnections) {
				ob.interrupt_exist_connections = true;
			}
		}

		// URLTest specific fields
		if (type === 'urltest') {
			if (urltestUrl.trim()) ob.url = urltestUrl.trim();
			if (urltestInterval && urltestInterval !== '3m') ob.interval = urltestInterval;
			if (urltestTolerance && urltestTolerance !== 150) ob.tolerance = urltestTolerance;
			if (urltestIdleTimeout.trim()) ob.idle_timeout = urltestIdleTimeout.trim();
		}

		if (type === 'vless') {
			(ob as any).server = server.trim();
			(ob as any).server_port = serverPort;
			(ob as any).uuid = uuid.trim();
			if (flow) (ob as any).flow = flow;

			if (tlsEnabled) {
				(ob as any).tls = {
					enabled: true,
					server_name: tlsSni || server.trim()
				};

				if (tlsFingerprint) {
					(ob as any).tls.utls = {
						enabled: true,
						fingerprint: tlsFingerprint
					};
				}

				if (tlsAlpn.length > 0) {
					(ob as any).tls.alpn = tlsAlpn;
				}

				if (realityEnabled) {
					(ob as any).tls.reality = {
						enabled: true,
						public_key: realityPublicKey,
						short_id: realityShortId || ''
					};
				}
			}

			if (transportType !== 'tcp') {
				if (transportType === 'ws') {
					(ob as any).transport = {
						type: 'ws',
						path: wsPath || '/'
					};
					if (wsHost) {
						(ob as any).transport.headers = { Host: wsHost };
					}
				} else if (transportType === 'grpc') {
					(ob as any).transport = {
						type: 'grpc',
						service_name: grpcServiceName || ''
					};
				} else if (transportType === 'http') {
					(ob as any).transport = {
						type: 'http',
						path: wsPath || '/'
					};
					if (wsHost) {
						(ob as any).transport.host = [wsHost];
					}
				}
			}
		}

		if (type === 'hysteria2') {
			(ob as any).server = server.trim();
			(ob as any).server_port = serverPort;
			(ob as any).password = hy2Password.trim();

			// Port hopping
			if (hy2ServerPorts.trim()) (ob as any).server_ports = hy2ServerPorts.trim();
			if (hy2HopInterval.trim()) (ob as any).hop_interval = hy2HopInterval.trim();

			// Bandwidth limits
			if (hy2UpMbps > 0) (ob as any).up_mbps = hy2UpMbps;
			if (hy2DownMbps > 0) (ob as any).down_mbps = hy2DownMbps;

			(ob as any).tls = {
				enabled: true,
				server_name: tlsSni || server.trim(),
				insecure: hy2Insecure
			};

			if (hy2ObfsType) {
				(ob as any).obfs = {
					type: hy2ObfsType,
					password: hy2ObfsPassword || ''
				};
			}
		}

		if (type === 'shadowsocks') {
			(ob as any).server = server.trim();
			(ob as any).server_port = serverPort;
			(ob as any).method = ssMethod;
			(ob as any).password = ssPassword.trim();
			if (ssPlugin.trim()) (ob as any).plugin = ssPlugin.trim();
			if (ssPluginOpts.trim()) (ob as any).plugin_opts = ssPluginOpts.trim();
			if (ssNetwork) (ob as any).network = ssNetwork;
			if (ssUdpOverTcp) (ob as any).udp_over_tcp = true;
			if (ssMuxEnabled) {
				const mux: Record<string, unknown> = { enabled: true, protocol: ssMuxProtocol };
				if (ssMuxMaxConns > 0) mux.max_connections = ssMuxMaxConns;
				if (ssMuxMinStreams > 0) mux.min_streams = ssMuxMinStreams;
				if (ssMuxMaxStreams > 0) mux.max_streams = ssMuxMaxStreams;
				if (ssMuxPadding) mux.padding = true;
				(ob as any).multiplex = mux;
			}
		}

		if (type === 'shadowtls') {
			(ob as any).server = server.trim();
			(ob as any).server_port = serverPort;
			(ob as any).version = stlsVersion;
			if (stlsPassword.trim()) (ob as any).password = stlsPassword.trim();
			(ob as any).tls = {
				enabled: true,
				server_name: stlsTlsSni || server.trim(),
				insecure: stlsInsecure
			};
			if (stlsTlsFingerprint) {
				(ob as any).tls.utls = { enabled: true, fingerprint: stlsTlsFingerprint };
			}
			if (stlsDetour) (ob as any).detour = stlsDetour;
		}

		if (type === 'anytls') {
			(ob as any).server = server.trim();
			(ob as any).server_port = serverPort;
			(ob as any).password = atPassword.trim();
			(ob as any).tls = {
				enabled: true,
				server_name: atTlsSni || server.trim(),
				insecure: atInsecure
			};
			if (atTlsFingerprint) {
				(ob as any).tls.utls = { enabled: true, fingerprint: atTlsFingerprint };
			}
			if (atIdleCheckInterval.trim()) (ob as any).idle_session_check_interval = atIdleCheckInterval.trim();
			if (atIdleTimeout.trim()) (ob as any).idle_session_timeout = atIdleTimeout.trim();
			if (atMinIdleSession > 0) (ob as any).min_idle_session = atMinIdleSession;
		}

		// Add domain_resolver for server-based outbounds (sing-box 1.12+)
		const serverBasedTypes = ['vless', 'hysteria2', 'shadowsocks', 'shadowtls', 'anytls'];
		if (serverBasedTypes.includes(type) && domainResolver.trim()) {
			(ob as any).domain_resolver = domainResolver.trim();
		}

		onSave(ob);
	}

	const outboundTypes = [
		{ value: 'direct', labelKey: 'outbounds.types.direct', descKey: 'outbounds.directDesc' },
		{ value: 'block', labelKey: 'outbounds.types.block', descKey: 'outbounds.blockDesc' },
		{ value: 'selector', labelKey: 'outbounds.types.selector', descKey: 'outbounds.selectorDesc' },
		{ value: 'urltest', labelKey: 'outbounds.types.urltest', descKey: 'outbounds.urltestDesc' },
		{ value: 'vless', labelKey: 'outbounds.vless', descKey: 'outbounds.vlessDesc' },
		{ value: 'hysteria2', labelKey: 'outbounds.hysteria2', descKey: 'outbounds.hysteria2Desc' },
		{ value: 'shadowsocks', labelKey: 'outbounds.shadowsocks', descKey: 'outbounds.shadowsocksDesc' },
		{ value: 'shadowtls', labelKey: 'outbounds.shadowtls', descKey: 'outbounds.shadowtlsDesc' },
		{ value: 'anytls', labelKey: 'outbounds.anytls', descKey: 'outbounds.anytlsDesc' }
	];

	const ssMethods = [
		'2022-blake3-aes-128-gcm', '2022-blake3-aes-256-gcm', '2022-blake3-chacha20-poly1305',
		'aes-128-gcm', 'aes-192-gcm', 'aes-256-gcm',
		'chacha20-ietf-poly1305', 'xchacha20-ietf-poly1305', 'none'
	];

	const fingerprints = ['chrome', 'firefox', 'safari', 'edge', 'ios', 'android', 'random', 'randomized'];
</script>

<form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="space-y-6">
	<!-- Tag -->
	<div>
		<label for="tag" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('common.tag')} *</label>
		<input
			id="tag"
			type="text"
			bind:value={tag}
			placeholder="proxy"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['tag'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
		/>
		{#if errors['tag']}
			<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['tag']}</p>
		{/if}
	</div>

	<!-- Type -->
	<div>
		<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('common.type')}</label>
		<div class="grid grid-cols-3 gap-2">
			{#each outboundTypes as ot}
				<button
					type="button"
					onclick={() => type = ot.value}
					class="type-btn {type === ot.value ? 'selected' : ''}"
				>
					<div class="type-label">{$t(ot.labelKey)}</div>
					<div class="type-desc">{$t(ot.descKey)}</div>
				</button>
			{/each}
		</div>
	</div>

	<!-- Type-specific options -->
	{#if type === 'selector' || type === 'urltest'}
		<div>
			<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">
				{$t('outbounds.selectedOutbounds')} *
				<span class="font-normal text-[var(--ctp-overlay0)]">({selectedOutbounds.length} {$t('proxies.selected').toLowerCase()})</span>
			</label>
			{#if errors['outbounds']}
				<p class="mb-2 text-sm text-[var(--ctp-red)]">{errors['outbounds']}</p>
			{/if}
			<div class="max-h-48 overflow-y-auto bg-[var(--ctp-surface0)] rounded-lg border border-[var(--ctp-surface2)]">
				{#if availableForSelection.length === 0}
					<div class="p-4 text-center text-[var(--ctp-overlay0)]">
						{$t('outbounds.noOutboundsAvailable')}
					</div>
				{:else}
					{#each availableForSelection as tag}
						<button
							type="button"
							onclick={() => toggleOutboundSelection(tag)}
							class="w-full px-4 py-2 flex items-center justify-between hover:bg-[var(--ctp-surface1)] transition-colors {selectedOutbounds.includes(tag) ? 'bg-[var(--ctp-surface1)]' : ''}"
						>
							<span class="text-[var(--ctp-text)]">{tag}</span>
							<span class="flex items-center gap-2">
								{#if defaultOutbound === tag}
									<span class="status-badge">{$t('common.default').toLowerCase()}</span>
								{/if}
								{#if selectedOutbounds.includes(tag)}
									<svg class="w-5 h-5 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
									</svg>
								{/if}
							</span>
						</button>
					{/each}
				{/if}
			</div>
		</div>

		{#if selectedOutbounds.length > 0}
			<div>
				<label for="default" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.defaultOutbound')}</label>
				<select
					id="default"
					bind:value={defaultOutbound}
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				>
					{#each selectedOutbounds as tag}
						<option value={tag}>{tag}</option>
					{/each}
				</select>
			</div>

			<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
				<input
					type="checkbox"
					bind:checked={interruptExistConnections}
					class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
				/>
				{$t('outbounds.interruptExisting')}
			</label>
		{/if}

		<!-- URLTest specific settings -->
		{#if type === 'urltest'}
			<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
				<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">{$t('outbounds.urlTestSettings')}</h3>
				<div>
					<label for="urltestUrl" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('outbounds.testUrl')}</label>
					<input
						id="urltestUrl"
						type="url"
						bind:value={urltestUrl}
						placeholder="https://www.gstatic.com/generate_204"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
					/>
				</div>
				<div class="grid grid-cols-3 gap-4">
					<div>
						<label for="urltestInterval" class="flex items-center gap-1 text-xs text-[var(--ctp-overlay0)] mb-1">
							{$t('outbounds.testInterval')}
							<HelpTooltip text={$t('help.interval')} />
						</label>
						<input
							id="urltestInterval"
							type="text"
							bind:value={urltestInterval}
							placeholder="3m"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
						/>
					</div>
					<div>
						<label for="urltestTolerance" class="flex items-center gap-1 text-xs text-[var(--ctp-overlay0)] mb-1">
							{$t('outbounds.tolerance')} (ms)
							<HelpTooltip text={$t('help.tolerance')} />
						</label>
						<input
							id="urltestTolerance"
							type="number"
							bind:value={urltestTolerance}
							min="0"
							placeholder="150"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
						/>
					</div>
					<div>
						<label for="urltestIdleTimeout" class="flex items-center gap-1 text-xs text-[var(--ctp-overlay0)] mb-1">
							{$t('outbounds.idleTimeout')}
							<HelpTooltip text={$t('help.idleTimeout')} />
						</label>
						<input
							id="urltestIdleTimeout"
							type="text"
							bind:value={urltestIdleTimeout}
							placeholder="30m"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
						/>
					</div>
				</div>
			</div>
		{/if}
	{/if}

	{#if type === 'vless'}
		<!-- Import Button -->
		<button
			type="button"
			onclick={() => showImport = true}
			class="w-full px-4 py-2 bg-[var(--ctp-surface0)] border border-dashed border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-subtext1)] hover:border-[var(--ctp-primary)] hover:text-[var(--ctp-primary)] transition-colors flex items-center justify-center gap-2"
		>
			<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
			</svg>
			{$t('outbounds.importFromVless')}
		</button>


		<!-- Server & Port -->
		<div class="grid grid-cols-3 gap-4">
			<div class="col-span-2">
				<label for="server" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('common.server')} *</label>
				<input
					id="server"
					type="text"
					bind:value={server}
					placeholder="example.com"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['server'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
				/>
			</div>
			<div>
				<label for="port" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('common.port')} *</label>
				<input
					id="port"
					type="number"
					bind:value={serverPort}
					min="1"
					max="65535"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['serverPort'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
				/>
			</div>
		</div>

		<!-- Domain Resolver -->
		{#if showDomainResolver}
			<div>
				<label for="domainResolver" class="flex items-center gap-1 text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('outbounds.domainResolver')}
				<HelpTooltip text={$t('help.domainResolver')} />
			</label>
				<select
					id="domainResolver"
					bind:value={domainResolver}
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				>
					<option value="">{$t('common.none')}</option>
					{#each dnsServers as dns}
						<option value={dns.tag}>{dns.tag}</option>
					{/each}
				</select>
				<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('outbounds.domainResolverHint')}</p>
			</div>
		{/if}

		<!-- UUID -->
		<div>
			<label for="uuid" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.uuid')} *</label>
			<input
				id="uuid"
				type="text"
				bind:value={uuid}
				placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['uuid'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
			/>
		</div>

		<!-- Flow -->
		<div>
			<label for="flow" class="flex items-center gap-1 text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('outbounds.flow')}
				<HelpTooltip text={$t('help.flow')} />
			</label>
			<select
				id="flow"
				bind:value={flow}
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
			>
				<option value="">{$t('common.none')}</option>
				<option value="xtls-rprx-vision">xtls-rprx-vision</option>
			</select>
		</div>

		<!-- TLS Settings -->
		<div class="space-y-4">
			<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
				<input type="checkbox" bind:checked={tlsEnabled} />
				{$t('outbounds.enableTls')}
			</label>

			{#if tlsEnabled}
				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="sni" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.sni')}</label>
						<input
							id="sni"
							type="text"
							bind:value={tlsSni}
							placeholder="Server name"
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>
					<div>
						<label for="fingerprint" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.fingerprint')}</label>
						<select
							id="fingerprint"
							bind:value={tlsFingerprint}
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						>
							{#each fingerprints as fp}
								<option value={fp}>{fp}</option>
							{/each}
						</select>
					</div>
				</div>

				<!-- Reality -->
				<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
					<input type="checkbox" bind:checked={realityEnabled} />
					{$t('outbounds.enableReality')}
					<HelpTooltip text={$t('help.reality')} />
				</label>

				{#if realityEnabled}
					<div class="grid grid-cols-2 gap-4">
						<div>
							<label for="pbk" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.publicKey')} *</label>
							<input
								id="pbk"
								type="text"
								bind:value={realityPublicKey}
								placeholder="Reality public key"
								class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['realityPublicKey'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
							/>
						</div>
						<div>
							<label for="sid" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.shortId')}</label>
							<input
								id="sid"
								type="text"
								bind:value={realityShortId}
								placeholder="Short ID"
								class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
							/>
						</div>
					</div>
				{/if}
			{/if}
		</div>

		<!-- Transport -->
		<div class="space-y-4">
			<label class="block text-sm font-medium text-[var(--ctp-subtext1)]">{$t('outbounds.transport')}</label>
			<div class="grid grid-cols-4 gap-2">
				{#each ['tcp', 'ws', 'grpc', 'http'] as tp}
					<button
						type="button"
						onclick={() => transportType = tp as any}
						class="toggle-btn text-sm {transportType === tp ? 'selected' : ''}"
					>
						{tp.toUpperCase()}
					</button>
				{/each}
			</div>

			{#if transportType === 'ws' || transportType === 'http'}
				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="wsPath" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.path')}</label>
						<input
							id="wsPath"
							type="text"
							bind:value={wsPath}
							placeholder="/"
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>
					<div>
						<label for="wsHost" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.host')}</label>
						<input
							id="wsHost"
							type="text"
							bind:value={wsHost}
							placeholder="Host header"
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>
				</div>
			{/if}

			{#if transportType === 'grpc'}
				<div>
					<label for="serviceName" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.serviceName')}</label>
					<input
						id="serviceName"
						type="text"
						bind:value={grpcServiceName}
						placeholder="grpc service name"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
				</div>
			{/if}
		</div>
	{/if}

	{#if type === 'hysteria2'}
		<!-- Import Button -->
		<button
			type="button"
			onclick={() => showImport = true}
			class="w-full px-4 py-2 bg-[var(--ctp-surface0)] border border-dashed border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-subtext1)] hover:border-[var(--ctp-primary)] hover:text-[var(--ctp-primary)] transition-colors flex items-center justify-center gap-2"
		>
			<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
			</svg>
			{$t('outbounds.importFromHy2')}
		</button>

		<!-- Server & Port -->
		<div class="grid grid-cols-3 gap-4">
			<div class="col-span-2">
				<label for="hy2server" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('common.server')} *</label>
				<input
					id="hy2server"
					type="text"
					bind:value={server}
					placeholder="example.com"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['server'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
				/>
			</div>
			<div>
				<label for="hy2port" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('common.port')} *</label>
				<input
					id="hy2port"
					type="number"
					bind:value={serverPort}
					min="1"
					max="65535"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['serverPort'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
				/>
			</div>
		</div>

		<!-- Domain Resolver -->
		{#if showDomainResolver}
			<div>
				<label for="hy2DomainResolver" class="flex items-center gap-1 text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('outbounds.domainResolver')}
				<HelpTooltip text={$t('help.domainResolver')} />
			</label>
				<select
					id="hy2DomainResolver"
					bind:value={domainResolver}
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				>
					<option value="">{$t('common.none')}</option>
					{#each dnsServers as dns}
						<option value={dns.tag}>{dns.tag}</option>
					{/each}
				</select>
				<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('outbounds.domainResolverHint')}</p>
			</div>
		{/if}

		<!-- Password -->
		<div>
			<label for="hy2password" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.password')} *</label>
			<input
				id="hy2password"
				type="password"
				bind:value={hy2Password}
				placeholder="Authentication password"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['password'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
			/>
		</div>

		<!-- TLS SNI -->
		<div>
			<label for="hy2sni" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.sni')}</label>
			<input
				id="hy2sni"
				type="text"
				bind:value={tlsSni}
				placeholder="Server name (optional)"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
			/>
		</div>

		<!-- Insecure -->
		<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
			<input type="checkbox" bind:checked={hy2Insecure} />
			{$t('outbounds.skipCertVerification')}
		</label>

		<!-- Port Hopping -->
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
			<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">{$t('outbounds.portHopping')} ({$t('common.optional').toLowerCase()})</h3>
			<div class="grid grid-cols-2 gap-4">
				<div>
					<label for="hy2ServerPorts" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('outbounds.serverPorts')}</label>
					<input
						id="hy2ServerPorts"
						type="text"
						bind:value={hy2ServerPorts}
						placeholder="1000-2000,3000-4000"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
					/>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('outbounds.portRangesHint')}</p>
				</div>
				<div>
					<label for="hy2HopInterval" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('outbounds.hopInterval')}</label>
					<input
						id="hy2HopInterval"
						type="text"
						bind:value={hy2HopInterval}
						placeholder="30s"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
					/>
				</div>
			</div>
		</div>

		<!-- Bandwidth Limits -->
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
			<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">{$t('outbounds.bandwidthLimits')} ({$t('common.optional').toLowerCase()})</h3>
			<div class="grid grid-cols-2 gap-4">
				<div>
					<label for="hy2UpMbps" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('outbounds.uploadMbps')}</label>
					<input
						id="hy2UpMbps"
						type="number"
						bind:value={hy2UpMbps}
						min="0"
						placeholder="0 = unlimited"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
					/>
				</div>
				<div>
					<label for="hy2DownMbps" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('outbounds.downloadMbps')}</label>
					<input
						id="hy2DownMbps"
						type="number"
						bind:value={hy2DownMbps}
						min="0"
						placeholder="0 = unlimited"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
					/>
				</div>
			</div>
		</div>

		<!-- Obfuscation -->
		<div class="space-y-4">
			<div>
				<label for="obfsType" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.obfuscationType')}</label>
				<select
					id="obfsType"
					bind:value={hy2ObfsType}
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				>
					<option value="">{$t('common.none')}</option>
					<option value="salamander">Salamander</option>
				</select>
			</div>

			{#if hy2ObfsType}
				<div>
					<label for="obfsPassword" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.obfuscationPassword')}</label>
					<input
						id="obfsPassword"
						type="text"
						bind:value={hy2ObfsPassword}
						placeholder="Obfuscation password"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
				</div>
			{/if}
		</div>
	{/if}

	{#if type === 'shadowsocks'}
		<!-- Import Button -->
		<button
			type="button"
			onclick={() => showImport = true}
			class="w-full px-4 py-2 bg-[var(--ctp-surface0)] border border-dashed border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-subtext1)] hover:border-[var(--ctp-primary)] hover:text-[var(--ctp-primary)] transition-colors flex items-center justify-center gap-2"
		>
			<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
			</svg>
			{$t('outbounds.importFromSs')}
		</button>

		<!-- Server & Port -->
		<div class="grid grid-cols-3 gap-4">
			<div class="col-span-2">
				<label for="ssServer" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('common.server')} *</label>
				<input id="ssServer" type="text" bind:value={server} placeholder="example.com"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['server'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}" />
			</div>
			<div>
				<label for="ssPort" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('common.port')} *</label>
				<input id="ssPort" type="number" bind:value={serverPort} min="1" max="65535"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['serverPort'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}" />
			</div>
		</div>

		<!-- Domain Resolver -->
		{#if showDomainResolver}
			<div>
				<label for="ssDomainResolver" class="flex items-center gap-1 text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('outbounds.domainResolver')}
				<HelpTooltip text={$t('help.domainResolver')} />
			</label>
				<select
					id="ssDomainResolver"
					bind:value={domainResolver}
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				>
					<option value="">{$t('common.none')}</option>
					{#each dnsServers as dns}
						<option value={dns.tag}>{dns.tag}</option>
					{/each}
				</select>
				<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('outbounds.domainResolverHint')}</p>
			</div>
		{/if}

		<!-- Method -->
		<div>
			<label for="ssMethod" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.method')} *</label>
			<select id="ssMethod" bind:value={ssMethod}
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['method'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}">
				{#each ssMethods as m}
					<option value={m}>{m}</option>
				{/each}
			</select>
		</div>

		<!-- Password -->
		<div>
			<label for="ssPassword" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.password')} *</label>
			<input id="ssPassword" type="password" bind:value={ssPassword} placeholder="Password"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['password'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}" />
		</div>

		<!-- Plugin -->
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="ssPlugin" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.plugin')}</label>
				<input id="ssPlugin" type="text" bind:value={ssPlugin} placeholder="obfs-local"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
			</div>
			<div>
				<label for="ssPluginOpts" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.pluginOpts')}</label>
				<input id="ssPluginOpts" type="text" bind:value={ssPluginOpts} placeholder="obfs=http;obfs-host=..."
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
			</div>
		</div>

		<!-- Network & UDP over TCP -->
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="ssNetwork" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.networkType')}</label>
				<select id="ssNetwork" bind:value={ssNetwork}
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]">
					<option value="">{$t('common.default')} (TCP + UDP)</option>
					<option value="tcp">TCP</option>
					<option value="udp">UDP</option>
				</select>
			</div>
			<div class="flex items-end pb-2">
				<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
					<input type="checkbox" bind:checked={ssUdpOverTcp} />
					{$t('outbounds.udpOverTcp')}
				</label>
			</div>
		</div>

		<!-- Multiplexing -->
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
			<label class="flex items-center gap-2 text-sm font-medium text-[var(--ctp-subtext1)]">
				<input type="checkbox" bind:checked={ssMuxEnabled} />
				{$t('outbounds.multiplexing')}
				<HelpTooltip text={$t('help.multiplex')} />
			</label>

			{#if ssMuxEnabled}
				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="ssMuxProtocol" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('outbounds.muxProtocol')}</label>
						<select id="ssMuxProtocol" bind:value={ssMuxProtocol}
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm">
							<option value="h2mux">h2mux</option>
							<option value="smux">smux</option>
							<option value="yamux">yamux</option>
						</select>
					</div>
					<div>
						<label for="ssMuxMaxConns" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('outbounds.maxConnections')}</label>
						<input id="ssMuxMaxConns" type="number" bind:value={ssMuxMaxConns} min="0"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm" />
					</div>
				</div>
				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="ssMuxMinStreams" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('outbounds.minStreams')}</label>
						<input id="ssMuxMinStreams" type="number" bind:value={ssMuxMinStreams} min="0"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm" />
					</div>
					<div>
						<label for="ssMuxMaxStreams" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('outbounds.maxStreams')}</label>
						<input id="ssMuxMaxStreams" type="number" bind:value={ssMuxMaxStreams} min="0"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm" />
					</div>
				</div>
				<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
					<input type="checkbox" bind:checked={ssMuxPadding} />
					{$t('outbounds.padding')}
				</label>
			{/if}
		</div>
	{/if}

	{#if type === 'shadowtls'}
		<!-- Server & Port -->
		<div class="grid grid-cols-3 gap-4">
			<div class="col-span-2">
				<label for="stlsServer" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('common.server')} *</label>
				<input id="stlsServer" type="text" bind:value={server} placeholder="example.com"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['server'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}" />
			</div>
			<div>
				<label for="stlsPort" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('common.port')} *</label>
				<input id="stlsPort" type="number" bind:value={serverPort} min="1" max="65535"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['serverPort'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}" />
			</div>
		</div>

		<!-- Domain Resolver -->
		{#if showDomainResolver}
			<div>
				<label for="stlsDomainResolver" class="flex items-center gap-1 text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('outbounds.domainResolver')}
				<HelpTooltip text={$t('help.domainResolver')} />
			</label>
				<select
					id="stlsDomainResolver"
					bind:value={domainResolver}
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				>
					<option value="">{$t('common.none')}</option>
					{#each dnsServers as dns}
						<option value={dns.tag}>{dns.tag}</option>
					{/each}
				</select>
				<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('outbounds.domainResolverHint')}</p>
			</div>
		{/if}

		<!-- Version -->
		<div>
			<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('outbounds.version')}</label>
			<div class="grid grid-cols-3 gap-2">
				{#each [1, 2, 3] as v}
					<button type="button" onclick={() => stlsVersion = v}
						class="toggle-btn text-sm {stlsVersion === v ? 'selected' : ''}">
						v{v}
					</button>
				{/each}
			</div>
		</div>

		<!-- Password (v2/v3) -->
		{#if stlsVersion >= 2}
			<div>
				<label for="stlsPassword" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.password')} *</label>
				<input id="stlsPassword" type="password" bind:value={stlsPassword} placeholder="Password"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['password'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}" />
			</div>
		{/if}

		<!-- TLS -->
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="stlsSni" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.sni')}</label>
				<input id="stlsSni" type="text" bind:value={stlsTlsSni} placeholder="Server name"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
			</div>
			<div>
				<label for="stlsFingerprint" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.fingerprint')}</label>
				<select id="stlsFingerprint" bind:value={stlsTlsFingerprint}
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]">
					<option value="">{$t('common.none')}</option>
					{#each fingerprints as fp}
						<option value={fp}>{fp}</option>
					{/each}
				</select>
			</div>
		</div>

		<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
			<input type="checkbox" bind:checked={stlsInsecure} />
			{$t('outbounds.skipCertVerification')}
		</label>

		<!-- Detour -->
		<div>
			<label for="stlsDetour" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.detour')}</label>
			<select id="stlsDetour" bind:value={stlsDetour}
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]">
				<option value="">{$t('common.none')}</option>
				{#each outbounds.filter(o => o.tag !== outbound?.tag) as ob}
					<option value={ob.tag}>{ob.tag}</option>
				{/each}
			</select>
			<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('outbounds.detourHint')}</p>
		</div>

		<!-- Chain hint -->
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 text-sm text-[var(--ctp-overlay1)]">
			{$t('outbounds.shadowtlsChainHint')}
		</div>
	{/if}

	{#if type === 'anytls'}
		<!-- Server & Port -->
		<div class="grid grid-cols-3 gap-4">
			<div class="col-span-2">
				<label for="atServer" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('common.server')} *</label>
				<input id="atServer" type="text" bind:value={server} placeholder="example.com"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['server'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}" />
			</div>
			<div>
				<label for="atPort" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('common.port')} *</label>
				<input id="atPort" type="number" bind:value={serverPort} min="1" max="65535"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['serverPort'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}" />
			</div>
		</div>

		<!-- Domain Resolver -->
		{#if showDomainResolver}
			<div>
				<label for="atDomainResolver" class="flex items-center gap-1 text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('outbounds.domainResolver')}
				<HelpTooltip text={$t('help.domainResolver')} />
			</label>
				<select
					id="atDomainResolver"
					bind:value={domainResolver}
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				>
					<option value="">{$t('common.none')}</option>
					{#each dnsServers as dns}
						<option value={dns.tag}>{dns.tag}</option>
					{/each}
				</select>
				<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('outbounds.domainResolverHint')}</p>
			</div>
		{/if}

		<!-- Password -->
		<div>
			<label for="atPassword" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.password')} *</label>
			<input id="atPassword" type="password" bind:value={atPassword} placeholder="Password"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['password'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}" />
		</div>

		<!-- TLS -->
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="atSni" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.sni')}</label>
				<input id="atSni" type="text" bind:value={atTlsSni} placeholder="Server name"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
			</div>
			<div>
				<label for="atFingerprint" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.fingerprint')}</label>
				<select id="atFingerprint" bind:value={atTlsFingerprint}
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]">
					<option value="">{$t('common.none')}</option>
					{#each fingerprints as fp}
						<option value={fp}>{fp}</option>
					{/each}
				</select>
			</div>
		</div>

		<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
			<input type="checkbox" bind:checked={atInsecure} />
			{$t('outbounds.skipCertVerification')}
		</label>

		<!-- Idle Session Settings -->
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
			<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">{$t('common.advanced')} ({$t('common.optional').toLowerCase()})</h3>
			<div class="grid grid-cols-3 gap-4">
				<div>
					<label for="atIdleCheck" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('outbounds.idleSessionCheckInterval')}</label>
					<input id="atIdleCheck" type="text" bind:value={atIdleCheckInterval} placeholder="30s"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm" />
				</div>
				<div>
					<label for="atIdleTimeout" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('outbounds.idleSessionTimeout')}</label>
					<input id="atIdleTimeout" type="text" bind:value={atIdleTimeout} placeholder="30s"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm" />
				</div>
				<div>
					<label for="atMinIdle" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('outbounds.minIdleSession')}</label>
					<input id="atMinIdle" type="number" bind:value={atMinIdleSession} min="0"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm" />
				</div>
			</div>
		</div>
	{/if}

	{#if type === 'direct' || type === 'block'}
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 text-sm text-[var(--ctp-overlay1)]">
			{#if type === 'direct'}
				{$t('outbounds.directDesc')}
			{:else}
				{$t('outbounds.blockDesc')}
			{/if}
		</div>
	{/if}

	<!-- Actions -->
	<div class="flex justify-end gap-3 pt-4 border-t border-[var(--ctp-surface2)]">
		<button
			type="button"
			onclick={onCancel}
			class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
		>
			{$t('common.cancel')}
		</button>
		<button
			type="submit"
			class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
		>
			{outbound ? $t('common.saveChanges') : $t('outbounds.createOutbound')}
		</button>
	</div>
</form>

<!-- Import Modal -->
{#if showImport}
	<div class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
		<div class="bg-[var(--ctp-surface0)] rounded-xl p-6 w-full max-w-lg mx-4 shadow-xl">
			<div class="flex items-center justify-between mb-4">
				<h3 class="text-lg font-semibold text-[var(--ctp-text)]">
					{$t('common.import')} {type === 'vless' ? 'VLESS' : type === 'hysteria2' ? 'Hysteria2' : 'Shadowsocks'} {$t('outbounds.configuration')}
				</h3>
				<button
					onclick={() => { showImport = false; importText = ''; importError = ''; }}
					class="p-1 hover:bg-[var(--ctp-surface1)] rounded transition-colors"
				>
					<svg class="w-5 h-5 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>

			<p class="text-sm text-[var(--ctp-subtext0)] mb-4">
				{$t('outbounds.pasteLink', { values: { protocol: type === 'vless' ? 'vless://' : type === 'hysteria2' ? 'hy2:// or hysteria2://' : 'ss://' } })}
			</p>

			<textarea
				bind:value={importText}
				placeholder="{type === 'vless' ? 'vless://uuid@server:port?params#name' : type === 'hysteria2' ? 'hy2://password@server:port?params#name' : 'ss://BASE64(method:password)@server:port#name'}"
				rows="4"
				class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm resize-none"
			></textarea>

			{#if importError}
				<p class="mt-2 text-sm text-[var(--ctp-red)]">{importError}</p>
			{/if}

			<div class="flex justify-end gap-3 mt-4">
				<button
					onclick={() => { showImport = false; importText = ''; importError = ''; }}
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
{/if}
