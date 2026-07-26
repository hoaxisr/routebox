<script lang="ts">
	import type { Outbound, Endpoint, DnsServer, TLSConfig, TransportConfig, MultiplexConfig, ObfsConfig } from '$lib/types';
	import { notifications } from '$lib/stores';
	import { parsePortRanges, parseKeyValuePairs, formatKeyValuePairs, normalizeMieruPort } from '$lib/utils/parsers';
	import type { ParsedVless, ParsedTrojan, ParsedHysteria2, ParsedShadowsocks, ParsedNaive, ParsedMieru } from '$lib/utils/parsers';
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
	import TrojanForm from './outbound/TrojanForm.svelte';
	import Hysteria2Form from './outbound/Hysteria2Form.svelte';
	import ShadowsocksForm from './outbound/ShadowsocksForm.svelte';
	import ShadowtlsForm from './outbound/ShadowtlsForm.svelte';
	import AnytlsForm from './outbound/AnytlsForm.svelte';
	import NaiveForm from './outbound/NaiveForm.svelte';
	import MieruForm from './outbound/MieruForm.svelte';
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
	let serverPort = $state(outbound?.server_port ?? (outbound?.type === 'mieru' ? 0 : 443));
	let domainResolver = $state(outbound?.domain_resolver ?? '');

	// mieru stores `transport` as a plain string ("TCP"/"UDP"); vless/trojan use
	// the object form. Narrow once so the object-form initializers below type-check.
	const outboundStreamTransport = typeof outbound?.transport === 'object' ? outbound.transport : undefined;

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
		type: (outboundStreamTransport?.type ?? 'tcp') as 'tcp' | 'ws' | 'http' | 'grpc' | 'quic' | 'httpupgrade' | 'xhttp',
		path: outboundStreamTransport?.path ?? '/',
		// Host matrix mirrors build side: ws→headers.Host, httpupgrade/xhttp→top-level host string, http→host array.
		host: outboundStreamTransport?.headers?.Host
			?? (outboundStreamTransport?.type === 'httpupgrade' || outboundStreamTransport?.type === 'xhttp'
				? ((outboundStreamTransport as unknown as Record<string, unknown>)?.host as string ?? '')
				: (outboundStreamTransport?.host?.[0] ?? '')),
		service_name: outboundStreamTransport?.service_name ?? ''
	});

	// Trojan state
	let trojanPassword = $state(outbound?.type === 'trojan' ? (outbound?.password ?? '') : '');
	let trojanTls = $state<TLSConfig>({
		enabled: outbound?.type === 'trojan' ? (outbound?.tls?.enabled ?? true) : true,
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
	let trojanTransport = $state({
		type: (outbound?.type === 'trojan'
			? (outboundStreamTransport?.type === 'ws' || outboundStreamTransport?.type === 'grpc' || outboundStreamTransport?.type === 'httpupgrade'
				? outboundStreamTransport.type
				: 'tcp')
			: 'tcp') as 'tcp' | 'ws' | 'grpc' | 'httpupgrade',
		path: outboundStreamTransport?.path ?? '/',
		// Host matrix mirrors build side: ws→headers.Host, httpupgrade→top-level host string, http→host array.
		host: outboundStreamTransport?.headers?.Host
			?? (outboundStreamTransport?.type === 'httpupgrade'
				? ((outboundStreamTransport as unknown as Record<string, unknown>)?.host as string ?? '')
				: (outboundStreamTransport?.host?.[0] ?? '')),
		service_name: outboundStreamTransport?.service_name ?? ''
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
	let hy2ServerPorts = $state(outbound?.server_ports?.map(s => s.replace(':', '-')).join(', ') ?? '');
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

	// Naive state
	let nvUsername = $state(outbound?.username ?? '');
	let nvPassword = $state(outbound?.password ?? '');
	let nvSni = $state(outbound?.tls?.server_name ?? '');
	let nvCaCert = $state(outbound?.tls?.certificate?.join('\n') ?? '');
	let nvInsecureConcurrency = $state(outbound?.insecure_concurrency ?? 0);
	let nvExtraHeaders = $state(formatKeyValuePairs(outbound?.extra_headers));
	let nvQuic = $state(outbound?.quic ?? false);
	let nvQuicCC = $state(outbound?.quic_congestion_control ?? '');
	let nvUdpOverTcp = $state(outbound?.udp_over_tcp ?? false);

	// Mieru state (server/serverPort are the shared fields above — naive pattern)
	let mieruPorts = $state(outbound?.type === 'mieru' ? (outbound.server_ports ?? []).join(', ') : '');
	let mieruTransport = $state<'TCP' | 'UDP'>(outbound?.type === 'mieru' && outbound.transport === 'UDP' ? 'UDP' : 'TCP');
	let mieruUsername = $state(outbound?.type === 'mieru' ? (outbound.username ?? '') : '');
	let mieruPassword = $state(outbound?.type === 'mieru' ? (outbound.password ?? '') : '');
	let mieruMux = $state(outbound?.type === 'mieru' ? (outbound.multiplexing ?? '') : '');
	let mieruTrafficPattern = $state(outbound?.type === 'mieru' ? (outbound.traffic_pattern ?? '') : '');

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
	function handleImport(config: ParsedVless | ParsedTrojan | ParsedHysteria2 | ParsedShadowsocks | ParsedNaive | ParsedMieru) {
		if (config.type === 'vless') {
			applyVlessConfig(config);
		} else if (config.type === 'trojan') {
			applyTrojanConfig(config);
		} else if (config.type === 'hy2') {
			applyHysteria2Config(config);
		} else if (config.type === 'ss') {
			applyShadowsocksConfig(config);
		} else if (config.type === 'naive') {
			applyNaiveConfig(config);
		} else {
			applyMieruConfig(config);
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
			type: (config.transport || 'tcp') as 'tcp' | 'ws' | 'http' | 'grpc' | 'quic' | 'httpupgrade' | 'xhttp',
			path: config.path || '/',
			host: config.host || '',
			service_name: config.serviceName || ''
		};
	}

	function applyTrojanConfig(config: ParsedTrojan) {
		type = 'trojan';
		tag = config.name.replace(/[^a-zA-Z0-9-_]/g, '-');
		server = config.server;
		serverPort = config.port;
		trojanPassword = config.password;
		trojanTls = {
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
		trojanTransport = {
			type: (config.transport || 'tcp') as 'tcp' | 'ws' | 'grpc' | 'httpupgrade',
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

	function applyNaiveConfig(config: ParsedNaive) {
		type = 'naive';
		tag = config.name.replace(/[^a-zA-Z0-9-_]/g, '-');
		server = config.server;
		serverPort = config.port;
		nvUsername = config.username ?? '';
		nvPassword = config.password ?? '';
		nvSni = config.server;
		nvQuic = config.quic ?? false;
		nvCaCert = '';
		nvInsecureConcurrency = 0;
		nvExtraHeaders = '';
		nvQuicCC = '';
		nvUdpOverTcp = false;
	}

	function applyMieruConfig(config: ParsedMieru) {
		type = 'mieru';
		tag = (config.name || `mieru-${config.server}`).replace(/[^a-zA-Z0-9-_]/g, '-');
		server = config.server;
		serverPort = config.server_port ?? 0;
		mieruPorts = (config.server_ports ?? []).join(', ');
		mieruTransport = config.transport;
		mieruUsername = config.username;
		mieruPassword = config.password;
		mieruMux = config.multiplexing ?? '';
		mieruTrafficPattern = config.traffic_pattern ?? '';
	}

	function validate(): boolean {
		errors = {};

		const tagResult = validateRequired(tag, 'Tag');
		if (!tagResult.valid) errors['tag'] = tagResult.error!;

		if (type === 'selector' || type === 'urltest') {
			const outboundsResult = validateNonEmptyArray(selectedOutbounds, 'Outbounds');
			if (!outboundsResult.valid) errors['outbounds'] = outboundsResult.error!;
		}

		const serverBasedTypes = ['vless', 'trojan', 'hysteria2', 'shadowsocks', 'shadowtls', 'anytls', 'naive'];
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

		if (type === 'trojan') {
			const pwResult = validateRequired(trojanPassword, 'Password');
			if (!pwResult.valid) errors['password'] = pwResult.error!;
			if (trojanTls.reality?.enabled) {
				const pbkResult = validateRequired(trojanTls.reality.public_key || '', 'Reality public key');
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

		if (type === 'naive') {
			const hasUser = nvUsername.trim().length > 0;
			const hasPass = nvPassword.length > 0;
			if (hasUser !== hasPass) {
				errors['username'] = $t('validation.credentialsPaired');
				errors['password'] = $t('validation.credentialsPaired');
			}
		}

		if (type === 'mieru') {
			const serverResult = validateRequired(server, 'Server');
			if (!serverResult.valid) errors['server'] = serverResult.error!;
			const extraPorts = mieruPorts.split(',').map((s) => s.trim()).filter(Boolean);
			if (serverPort <= 0 && extraPorts.length === 0) {
				errors['serverPort'] = $t('outbounds.mieruForm.portRequired');
			}
			for (const p of extraPorts) {
				if (normalizeMieruPort(p) === null) {
					errors['serverPorts'] = $t('outbounds.mieruForm.portsInvalid');
					break;
				}
			}
			const userResult = validateRequired(mieruUsername, 'Username');
			if (!userResult.valid) errors['username'] = userResult.error!;
			const pwResult = validateRequired(mieruPassword, 'Password');
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
			if (type === 'selector' && defaultOutbound) ob.default = defaultOutbound;
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
			// Vision-flow strip: xtls-rprx-vision requires raw TCP. A non-raw
			// transport (ws/grpc/http/httpupgrade) + vision passes amnezia-box
			// `check` but breaks per-connection at runtime, so drop the flow here
			// (mirrors the inbound build-time strip in serverInbound.ts).
			const isRawTransport = vlessTransport.type === 'tcp';
			if (vlessFlow && (vlessFlow !== 'xtls-rprx-vision' || isRawTransport)) {
				ob.flow = vlessFlow;
			}
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
				} else if (vlessTransport.type === 'httpupgrade') {
					ob.transport = { type: 'httpupgrade', path: vlessTransport.path || '/' };
					if (vlessTransport.host) {
						(ob.transport as unknown as Record<string, unknown>).host = vlessTransport.host;
					}
				} else if (vlessTransport.type === 'xhttp') {
					// xHTTP (sing-box awg3-xhttp fork): top-level host string like httpupgrade; mode defaults to auto.
					ob.transport = { type: 'xhttp', path: vlessTransport.path || '/' };
					if (vlessTransport.host) {
						(ob.transport as unknown as Record<string, unknown>).host = vlessTransport.host;
					}
				}
			}
		}

		if (type === 'trojan') {
			ob.server = server.trim();
			ob.server_port = serverPort;
			ob.password = trojanPassword.trim();
			if (trojanTls.enabled) {
				ob.tls = { enabled: true, server_name: trojanTls.server_name || server.trim() };
				if (trojanTls.utls?.fingerprint) {
					ob.tls.utls = { enabled: true, fingerprint: trojanTls.utls.fingerprint };
				}
				if (trojanTls.alpn && trojanTls.alpn.length > 0) {
					ob.tls.alpn = trojanTls.alpn;
				}
				if (trojanTls.reality?.enabled) {
					ob.tls.reality = { enabled: true, public_key: trojanTls.reality.public_key, short_id: trojanTls.reality.short_id || '' };
				}
			}
			// Host matrix: ws→headers.Host, httpupgrade→top-level host string, grpc→service_name, raw(tcp)→omit.
			if (trojanTransport.type === 'ws') {
				ob.transport = { type: 'ws', path: trojanTransport.path || '/' };
				if (trojanTransport.host) ob.transport.headers = { Host: trojanTransport.host };
			} else if (trojanTransport.type === 'grpc') {
				ob.transport = { type: 'grpc', service_name: trojanTransport.service_name || '' };
			} else if (trojanTransport.type === 'httpupgrade') {
				ob.transport = { type: 'httpupgrade', path: trojanTransport.path || '/' };
				if (trojanTransport.host) (ob.transport as unknown as Record<string, unknown>).host = trojanTransport.host;
			}
		}

		if (type === 'hysteria2') {
			ob.server = server.trim();
			ob.server_port = serverPort;
			ob.password = hy2Password.trim();
			const parsedPorts = parsePortRanges(hy2ServerPorts);
			if (parsedPorts.length > 0) ob.server_ports = parsedPorts;
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

		if (type === 'naive') {
			ob.server = server.trim();
			ob.server_port = serverPort;
			if (nvUsername.trim()) {
				ob.username = nvUsername.trim();
				ob.password = nvPassword;
			}
			ob.tls = {
				enabled: true,
				server_name: nvSni.trim() || server.trim()
			};
			if (nvCaCert.trim()) {
				ob.tls.certificate = nvCaCert.trim().split('\n').filter((line) => line.trim().length > 0);
			}
			if (nvInsecureConcurrency > 0) ob.insecure_concurrency = nvInsecureConcurrency;
			const headers = parseKeyValuePairs(nvExtraHeaders);
			if (Object.keys(headers).length > 0) ob.extra_headers = headers;
			if (nvQuic) {
				ob.quic = true;
				if (nvQuicCC) ob.quic_congestion_control = nvQuicCC;
			}
			if (nvUdpOverTcp) ob.udp_over_tcp = true;
		}

		if (type === 'mieru') {
			ob.server = server.trim();
			if (serverPort > 0) ob.server_port = serverPort;
			const ports = mieruPorts
				.split(',')
				.map((s) => s.trim())
				.filter(Boolean)
				.map((s) => normalizeMieruPort(s) ?? s) // validate() guarantees non-null
				.map((s) => (s.includes('-') ? s : `${s}-${s}`)); // degenerate range — fork rejects bare "N"
			if (ports.length > 0) ob.server_ports = ports;
			ob.transport = mieruTransport;
			ob.username = mieruUsername.trim();
			ob.password = mieruPassword;
			if (mieruMux) ob.multiplexing = mieruMux;
			if (mieruTrafficPattern.trim()) ob.traffic_pattern = mieruTrafficPattern.trim();
		}

		// Add domain_resolver for server-based outbounds
		const serverBasedTypes = ['vless', 'trojan', 'hysteria2', 'shadowsocks', 'shadowtls', 'anytls', 'naive', 'mieru'];
		if (serverBasedTypes.includes(type) && domainResolver.trim()) {
			ob.domain_resolver = domainResolver.trim();
		}

		onSave(ob);
	}

	// Proxy protocols (2x2 block). Trojan/Shadowsocks/ShadowTLS/AnyTLS are deprecated in
	// RouteBox and removed from the picker (#22); their forms/parsers/validators stay so
	// existing configs still load and edit.
	const protocolTypes = [
		{ value: 'vless', labelKey: 'outbounds.vless', descKey: 'outbounds.vlessDesc' },
		{ value: 'hysteria2', labelKey: 'outbounds.hysteria2', descKey: 'outbounds.hysteria2Desc' },
		{ value: 'naive', labelKey: 'outbounds.naive', descKey: 'outbounds.naiveDesc' },
		{ value: 'mieru', labelKey: 'outbounds.mieru', descKey: 'outbounds.mieruDesc' }
	];
	// Groups (selector/urltest) and built-in outbounds (direct/block).
	const groupTypes = [
		{ value: 'selector', labelKey: 'outbounds.types.selector', descKey: 'outbounds.selectorDesc' },
		{ value: 'urltest', labelKey: 'outbounds.types.urltest', descKey: 'outbounds.urltestDesc' },
		{ value: 'direct', labelKey: 'outbounds.types.direct', descKey: 'outbounds.directDesc' },
		{ value: 'block', labelKey: 'outbounds.types.block', descKey: 'outbounds.blockDesc' }
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
	{#snippet typeButton(ot: { value: string; labelKey: string; descKey: string })}
		<button
			type="button"
			onclick={() => type = ot.value}
			class="type-btn {type === ot.value ? 'selected' : ''}"
		>
			<div class="type-label">{$t(ot.labelKey)}</div>
			<div class="type-desc">{$t(ot.descKey)}</div>
		</button>
	{/snippet}
	<div class="space-y-4">
		<label class="block text-sm font-medium text-[var(--ctp-subtext1)]">{$t('common.type')}</label>
		<div>
			<div class="text-xs font-medium text-[var(--ctp-overlay0)] uppercase tracking-wide mb-2">{$t('outbounds.pickerProtocols')}</div>
			<div class="grid grid-cols-2 gap-2">
				{#each protocolTypes as ot}{@render typeButton(ot)}{/each}
			</div>
		</div>
		<div>
			<div class="text-xs font-medium text-[var(--ctp-overlay0)] uppercase tracking-wide mb-2">{$t('outbounds.pickerGroups')}</div>
			<div class="grid grid-cols-2 sm:grid-cols-4 gap-2">
				{#each groupTypes as ot}{@render typeButton(ot)}{/each}
			</div>
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

	{#if type === 'trojan'}
		<TrojanForm
			bind:server
			bind:serverPort
			bind:password={trojanPassword}
			bind:tls={trojanTls}
			bind:transport={trojanTransport}
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

	{#if type === 'naive'}
		<NaiveForm
			bind:server
			bind:serverPort
			bind:username={nvUsername}
			bind:password={nvPassword}
			bind:sni={nvSni}
			bind:caCert={nvCaCert}
			bind:insecureConcurrency={nvInsecureConcurrency}
			bind:extraHeaders={nvExtraHeaders}
			bind:quic={nvQuic}
			bind:quicCongestionControl={nvQuicCC}
			bind:udpOverTcp={nvUdpOverTcp}
			bind:domainResolver
			{dnsServers}
			{hasDefaultResolver}
			{errors}
			onImport={() => showImport = true}
		/>
	{/if}

	{#if type === 'mieru'}
		<MieruForm
			bind:server
			bind:serverPort
			bind:ports={mieruPorts}
			bind:transport={mieruTransport}
			bind:username={mieruUsername}
			bind:password={mieruPassword}
			bind:multiplexing={mieruMux}
			bind:trafficPattern={mieruTrafficPattern}
			bind:domainResolver
			{dnsServers}
			{hasDefaultResolver}
			serverLocked
			{errors}
			onImport={() => showImport = true}
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
		protocol={type as 'vless' | 'trojan' | 'hysteria2' | 'shadowsocks' | 'naive' | 'mieru'}
		onImport={handleImport}
		onClose={() => showImport = false}
	/>
{/if}
