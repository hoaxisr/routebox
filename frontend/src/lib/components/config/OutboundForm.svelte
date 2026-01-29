<script lang="ts">
	import type { Outbound, Endpoint, DnsServer, TLSConfig, TransportConfig, MultiplexConfig, ObfsConfig } from '$lib/types';
	import { notifications } from '$lib/stores';
	import type { ParsedVless, ParsedHysteria2, ParsedShadowsocks } from '$lib/utils/parsers';
	import {
		validateRequired,
		validatePort,
		validateUUID,
		validateNonEmptyArray,
		hasValidationErrors
	} from '$lib/utils';
	import { t } from 'svelte-i18n';

	// Sub-components
	import SelectorForm from './outbound/SelectorForm.svelte';
	import VlessForm from './outbound/VlessForm.svelte';
	import Hysteria2Form from './outbound/Hysteria2Form.svelte';
	import ShadowsocksForm from './outbound/ShadowsocksForm.svelte';
	import ShadowtlsForm from './outbound/ShadowtlsForm.svelte';
	import AnytlsForm from './outbound/AnytlsForm.svelte';
	import ImportModal from './outbound/ImportModal.svelte';

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
	let errors = $state<Record<string, string>>({});

	// Import modal state
	let showImport = $state(false);

	// Selector/URLTest state
	let selectedOutbounds = $state<string[]>(outbound?.outbounds ?? []);
	let defaultOutbound = $state(outbound?.default ?? '');
	let interruptExistConnections = $state(outbound?.interrupt_exist_connections ?? false);
	let urltestUrl = $state(outbound?.url ?? '');
	let urltestInterval = $state(outbound?.interval ?? '3m');
	let urltestTolerance = $state(outbound?.tolerance ?? 150);
	let urltestIdleTimeout = $state(outbound?.idle_timeout ?? '');

	// Common server-based fields
	let server = $state(outbound?.server ?? '');
	let serverPort = $state(outbound?.server_port ?? 443);
	let domainResolver = $state(outbound?.domain_resolver ?? '');

	// VLESS state
	let vlessUuid = $state(outbound?.uuid ?? '');
	let vlessFlow = $state(outbound?.flow ?? '');
	let vlessTls = $state<TLSConfig>({
		enabled: outbound?.tls?.enabled ?? true,
		server_name: outbound?.tls?.server_name ?? '',
		insecure: outbound?.tls?.insecure ?? false,
		alpn: outbound?.tls?.alpn ?? [],
		utls: { enabled: true, fingerprint: outbound?.tls?.utls?.fingerprint ?? 'chrome' },
		reality: {
			enabled: outbound?.tls?.reality?.enabled ?? false,
			public_key: outbound?.tls?.reality?.public_key ?? '',
			short_id: outbound?.tls?.reality?.short_id ?? ''
		}
	});
	// VlessForm expects transport.host as string (for input binding), not string[]
	// We convert when building the outbound object
	let vlessTransport = $state({
		type: (outbound?.transport?.type ?? 'tcp') as 'tcp' | 'ws' | 'http' | 'grpc' | 'quic' | 'httpupgrade',
		path: outbound?.transport?.path ?? '/',
		host: outbound?.transport?.headers?.Host ?? (outbound?.transport?.host?.[0] ?? ''),
		service_name: outbound?.transport?.service_name ?? ''
	});

	// Hysteria2 state
	let hy2Password = $state(outbound?.password ?? '');
	let hy2Tls = $state<TLSConfig>({
		enabled: true,
		server_name: outbound?.tls?.server_name ?? '',
		insecure: outbound?.tls?.insecure ?? false,
		utls: { fingerprint: outbound?.tls?.utls?.fingerprint ?? '' }
	});
	let hy2Obfs = $state<ObfsConfig | undefined>(outbound?.obfs);
	let hy2ServerPorts = $state(outbound?.server_ports ?? '');
	let hy2HopInterval = $state(outbound?.hop_interval ?? '');
	let hy2UpMbps = $state(outbound?.up_mbps ?? 0);
	let hy2DownMbps = $state(outbound?.down_mbps ?? 0);

	// Shadowsocks state
	let ssMethod = $state(outbound?.method ?? '2022-blake3-aes-128-gcm');
	let ssPassword = $state(outbound?.password ?? '');
	let ssPlugin = $state(outbound?.plugin ?? '');
	let ssPluginOpts = $state(outbound?.plugin_opts ?? '');
	let ssNetwork = $state(outbound?.network ?? '');
	let ssUdpOverTcp = $state(outbound?.udp_over_tcp ?? false);
	let ssMux = $state<MultiplexConfig>({
		enabled: outbound?.multiplex?.enabled ?? false,
		protocol: outbound?.multiplex?.protocol ?? 'h2mux',
		max_connections: outbound?.multiplex?.max_connections ?? 0,
		min_streams: outbound?.multiplex?.min_streams ?? 0,
		max_streams: outbound?.multiplex?.max_streams ?? 0,
		padding: outbound?.multiplex?.padding ?? false
	});

	// ShadowTLS state
	let stlsVersion = $state(outbound?.version ?? 3);
	let stlsPassword = $state(outbound?.password ?? '');
	let stlsDetour = $state(outbound?.detour ?? '');
	let stlsTls = $state<TLSConfig>({
		enabled: true,
		server_name: outbound?.tls?.server_name ?? '',
		insecure: outbound?.tls?.insecure ?? false,
		utls: { fingerprint: outbound?.tls?.utls?.fingerprint ?? '' }
	});

	// AnyTLS state
	let atPassword = $state(outbound?.password ?? '');
	let atTls = $state<TLSConfig>({
		enabled: true,
		server_name: outbound?.tls?.server_name ?? '',
		insecure: outbound?.tls?.insecure ?? false,
		utls: { fingerprint: outbound?.tls?.utls?.fingerprint ?? '' }
	});
	let atIdleCheckInterval = $state(outbound?.idle_session_check_interval ?? '');
	let atIdleTimeout = $state(outbound?.idle_session_timeout ?? '');
	let atMinIdleSession = $state(outbound?.min_idle_session ?? 0);

	// Available outbounds for selector
	let availableForSelection = $derived.by(() => {
		if (type === 'selector' || type === 'urltest') {
			const endpointTags = endpoints.map((e) => e.tag);
			const otherOutbounds = outbounds.filter((o) => o.tag !== outbound?.tag).map((o) => o.tag);
			return [...endpointTags, ...otherOutbounds];
		}
		return [];
	});

	// Import handlers
	function handleImport(config: ParsedVless | ParsedHysteria2 | ParsedShadowsocks) {
		if ('uuid' in config) {
			applyVlessConfig(config as ParsedVless);
		} else if ('obfs' in config || 'insecure' in config) {
			applyHysteria2Config(config as ParsedHysteria2);
		} else {
			applyShadowsocksConfig(config as ParsedShadowsocks);
		}
		showImport = false;
	}

	function applyVlessConfig(config: ParsedVless) {
		type = 'vless';
		tag = config.name.replace(/[^a-zA-Z0-9-_]/g, '-');
		server = config.server;
		serverPort = config.port;
		vlessUuid = config.uuid;
		vlessFlow = config.flow || '';
		vlessTls = {
			enabled: config.security === 'tls' || config.security === 'reality',
			server_name: config.sni || config.server,
			alpn: config.alpn || [],
			utls: { enabled: true, fingerprint: config.fingerprint || 'chrome' },
			reality: {
				enabled: config.security === 'reality',
				public_key: config.pbk || '',
				short_id: config.sid || ''
			}
		};
		vlessTransport = {
			type: (config.transport || 'tcp') as 'tcp' | 'ws' | 'http' | 'grpc' | 'quic' | 'httpupgrade',
			path: config.path || '/',
			host: config.host || '',
			service_name: config.serviceName || ''
		};
	}

	function applyHysteria2Config(config: ParsedHysteria2) {
		type = 'hysteria2';
		tag = config.name.replace(/[^a-zA-Z0-9-_]/g, '-');
		server = config.server;
		serverPort = config.port;
		hy2Password = config.password;
		hy2Tls = {
			enabled: true,
			server_name: config.sni || config.server,
			insecure: config.insecure || false,
			utls: { fingerprint: '' }
		};
		if (config.obfs) {
			hy2Obfs = { type: config.obfs as 'salamander', password: config.obfsPassword || '' };
		}
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

		const tagResult = validateRequired(tag, 'Tag');
		if (!tagResult.valid) errors['tag'] = tagResult.error!;

		if (type === 'selector' || type === 'urltest') {
			const outboundsResult = validateNonEmptyArray(selectedOutbounds, 'Outbounds');
			if (!outboundsResult.valid) errors['outbounds'] = outboundsResult.error!;
		}

		const serverBasedTypes = ['vless', 'hysteria2', 'shadowsocks', 'shadowtls', 'anytls'];
		if (serverBasedTypes.includes(type)) {
			const serverResult = validateRequired(server, 'Server');
			if (!serverResult.valid) errors['server'] = serverResult.error!;
			const portResult = validatePort(serverPort);
			if (!portResult.valid) errors['serverPort'] = portResult.error!;
		}

		if (type === 'vless') {
			const uuidResult = validateUUID(vlessUuid);
			if (!uuidResult.valid) errors['uuid'] = uuidResult.error!;
			if (vlessTls.reality?.enabled) {
				const pbkResult = validateRequired(vlessTls.reality.public_key || '', 'Reality public key');
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

		const ob: Outbound = { type, tag: tag.trim() };

		if (type === 'selector' || type === 'urltest') {
			ob.outbounds = selectedOutbounds;
			if (defaultOutbound) ob.default = defaultOutbound;
			if (interruptExistConnections) ob.interrupt_exist_connections = true;
		}

		if (type === 'urltest') {
			if (urltestUrl.trim()) ob.url = urltestUrl.trim();
			if (urltestInterval && urltestInterval !== '3m') ob.interval = urltestInterval;
			if (urltestTolerance && urltestTolerance !== 150) ob.tolerance = urltestTolerance;
			if (urltestIdleTimeout.trim()) ob.idle_timeout = urltestIdleTimeout.trim();
		}

		if (type === 'vless') {
			ob.server = server.trim();
			ob.server_port = serverPort;
			ob.uuid = vlessUuid.trim();
			if (vlessFlow) ob.flow = vlessFlow;
			if (vlessTls.enabled) {
				ob.tls = {
					enabled: true,
					server_name: vlessTls.server_name || server.trim()
				};
				if (vlessTls.utls?.fingerprint) {
					ob.tls.utls = { enabled: true, fingerprint: vlessTls.utls.fingerprint };
				}
				if (vlessTls.alpn && vlessTls.alpn.length > 0) {
					ob.tls.alpn = vlessTls.alpn;
				}
				if (vlessTls.reality?.enabled) {
					ob.tls.reality = {
						enabled: true,
						public_key: vlessTls.reality.public_key,
						short_id: vlessTls.reality.short_id || ''
					};
				}
			}
			if (vlessTransport.type !== 'tcp') {
				if (vlessTransport.type === 'ws') {
					ob.transport = { type: 'ws', path: vlessTransport.path || '/' };
					if (vlessTransport.host) {
						ob.transport.headers = { Host: vlessTransport.host };
					}
				} else if (vlessTransport.type === 'grpc') {
					ob.transport = { type: 'grpc', service_name: vlessTransport.service_name || '' };
				} else if (vlessTransport.type === 'http') {
					ob.transport = { type: 'http', path: vlessTransport.path || '/' };
					if (vlessTransport.host) {
						ob.transport.host = [vlessTransport.host];
					}
				}
			}
		}

		if (type === 'hysteria2') {
			ob.server = server.trim();
			ob.server_port = serverPort;
			ob.password = hy2Password.trim();
			if (hy2ServerPorts.trim()) ob.server_ports = hy2ServerPorts.trim();
			if (hy2HopInterval.trim()) ob.hop_interval = hy2HopInterval.trim();
			if (hy2UpMbps > 0) ob.up_mbps = hy2UpMbps;
			if (hy2DownMbps > 0) ob.down_mbps = hy2DownMbps;
			ob.tls = {
				enabled: true,
				server_name: hy2Tls.server_name || server.trim(),
				insecure: hy2Tls.insecure
			};
			if (hy2Obfs?.type) {
				ob.obfs = { type: hy2Obfs.type, password: hy2Obfs.password || '' };
			}
		}

		if (type === 'shadowsocks') {
			ob.server = server.trim();
			ob.server_port = serverPort;
			ob.method = ssMethod;
			ob.password = ssPassword.trim();
			if (ssPlugin.trim()) ob.plugin = ssPlugin.trim();
			if (ssPluginOpts.trim()) ob.plugin_opts = ssPluginOpts.trim();
			if (ssNetwork) ob.network = ssNetwork;
			if (ssUdpOverTcp) ob.udp_over_tcp = true;
			if (ssMux.enabled) {
				const mux: MultiplexConfig = { enabled: true, protocol: ssMux.protocol };
				if (ssMux.max_connections && ssMux.max_connections > 0) mux.max_connections = ssMux.max_connections;
				if (ssMux.min_streams && ssMux.min_streams > 0) mux.min_streams = ssMux.min_streams;
				if (ssMux.max_streams && ssMux.max_streams > 0) mux.max_streams = ssMux.max_streams;
				if (ssMux.padding) mux.padding = true;
				ob.multiplex = mux;
			}
		}

		if (type === 'shadowtls') {
			ob.server = server.trim();
			ob.server_port = serverPort;
			ob.version = stlsVersion;
			if (stlsPassword.trim()) ob.password = stlsPassword.trim();
			ob.tls = {
				enabled: true,
				server_name: stlsTls.server_name || server.trim(),
				insecure: stlsTls.insecure
			};
			if (stlsTls.utls?.fingerprint) {
				ob.tls.utls = { enabled: true, fingerprint: stlsTls.utls.fingerprint };
			}
			if (stlsDetour) ob.detour = stlsDetour;
		}

		if (type === 'anytls') {
			ob.server = server.trim();
			ob.server_port = serverPort;
			ob.password = atPassword.trim();
			ob.tls = {
				enabled: true,
				server_name: atTls.server_name || server.trim(),
				insecure: atTls.insecure
			};
			if (atTls.utls?.fingerprint) {
				ob.tls.utls = { enabled: true, fingerprint: atTls.utls.fingerprint };
			}
			if (atIdleCheckInterval.trim()) ob.idle_session_check_interval = atIdleCheckInterval.trim();
			if (atIdleTimeout.trim()) ob.idle_session_timeout = atIdleTimeout.trim();
			if (atMinIdleSession > 0) ob.min_idle_session = atMinIdleSession;
		}

		// Add domain_resolver for server-based outbounds
		const serverBasedTypes = ['vless', 'hysteria2', 'shadowsocks', 'shadowtls', 'anytls'];
		if (serverBasedTypes.includes(type) && domainResolver.trim()) {
			ob.domain_resolver = domainResolver.trim();
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

	<!-- Type-specific forms -->
	{#if type === 'selector' || type === 'urltest'}
		<SelectorForm
			{type}
			availableOutbounds={availableForSelection}
			bind:selectedOutbounds
			bind:defaultOutbound
			bind:interruptExistConnections
			bind:testUrl={urltestUrl}
			bind:testInterval={urltestInterval}
			bind:tolerance={urltestTolerance}
			bind:idleTimeout={urltestIdleTimeout}
			{errors}
		/>
	{/if}

	{#if type === 'vless'}
		<VlessForm
			bind:server
			bind:serverPort
			bind:uuid={vlessUuid}
			bind:flow={vlessFlow}
			bind:tls={vlessTls}
			bind:transport={vlessTransport}
			bind:domainResolver
			{dnsServers}
			{hasDefaultResolver}
			{errors}
			onImport={() => showImport = true}
		/>
	{/if}

	{#if type === 'hysteria2'}
		<Hysteria2Form
			bind:server
			bind:serverPort
			bind:password={hy2Password}
			bind:tls={hy2Tls}
			bind:obfs={hy2Obfs}
			bind:serverPorts={hy2ServerPorts}
			bind:hopInterval={hy2HopInterval}
			bind:upMbps={hy2UpMbps}
			bind:downMbps={hy2DownMbps}
			bind:domainResolver
			{dnsServers}
			{hasDefaultResolver}
			{errors}
			onImport={() => showImport = true}
		/>
	{/if}

	{#if type === 'shadowsocks'}
		<ShadowsocksForm
			bind:server
			bind:serverPort
			bind:method={ssMethod}
			bind:password={ssPassword}
			bind:plugin={ssPlugin}
			bind:pluginOpts={ssPluginOpts}
			bind:network={ssNetwork}
			bind:udpOverTcp={ssUdpOverTcp}
			bind:multiplex={ssMux}
			bind:domainResolver
			{dnsServers}
			{hasDefaultResolver}
			{errors}
			onImport={() => showImport = true}
		/>
	{/if}

	{#if type === 'shadowtls'}
		<ShadowtlsForm
			bind:server
			bind:serverPort
			bind:version={stlsVersion}
			bind:password={stlsPassword}
			bind:tls={stlsTls}
			bind:detour={stlsDetour}
			bind:domainResolver
			{dnsServers}
			{hasDefaultResolver}
			{outbounds}
			currentTag={outbound?.tag}
			{errors}
		/>
	{/if}

	{#if type === 'anytls'}
		<AnytlsForm
			bind:server
			bind:serverPort
			bind:password={atPassword}
			bind:tls={atTls}
			bind:idleCheckInterval={atIdleCheckInterval}
			bind:idleTimeout={atIdleTimeout}
			bind:minIdleSession={atMinIdleSession}
			bind:domainResolver
			{dnsServers}
			{hasDefaultResolver}
			{errors}
		/>
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
	<ImportModal
		protocol={type as 'vless' | 'hysteria2' | 'shadowsocks'}
		onImport={handleImport}
		onClose={() => showImport = false}
	/>
{/if}
