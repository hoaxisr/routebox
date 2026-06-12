// Link parsers for VPN configuration import

import type { Endpoint, OutboundTyped } from '$lib/types';

export interface ParsedVless {
	type: 'vless';
	name: string;
	server: string;
	port: number;
	uuid: string;
	flow?: string;
	security?: string;
	// TLS options
	sni?: string;
	fingerprint?: string;
	alpn?: string[];
	// Transport
	transport?: 'tcp' | 'ws' | 'grpc' | 'http';
	path?: string;
	host?: string;
	serviceName?: string;
	// Reality
	pbk?: string;
	sid?: string;
}

export interface ParsedHysteria2 {
	type: 'hy2';
	name: string;
	server: string;
	port: number;
	password: string;
	sni?: string;
	insecure?: boolean;
	obfs?: string;
	obfsPassword?: string;
}

export interface ParsedShadowsocks {
	type: 'ss';
	name: string;
	server: string;
	port: number;
	method: string;
	password: string;
	plugin?: string;
	pluginOpts?: string;
}

export interface ParsedNaive {
	type: 'naive';
	name: string;
	server: string;
	port: number;
	username?: string;
	password?: string;
	quic?: boolean;
}

export interface ParsedAWG {
	type: 'awg';
	name: string;
	privateKey: string;
	address: string[];
	dns?: string[];
	mtu?: number;
	// Peer
	peerPublicKey: string;
	peerEndpoint: string;
	peerPort: number;
	presharedKey?: string;
	allowedIps: string[];
	persistentKeepalive?: number;
	// Obfuscation (Amnezia-specific)
	jc?: number;
	jmin?: number;
	jmax?: number;
	s1?: number;
	s2?: number;
	s3?: number;
	s4?: number;
	h1?: string;
	h2?: string;
	h3?: string;
	h4?: string;
	// Init parameters (can be large binary data as base64)
	i1?: string;
	i2?: string;
	i3?: string;
	i4?: string;
	i5?: string;
}

export type ParsedConfig = ParsedVless | ParsedHysteria2 | ParsedShadowsocks | ParsedNaive | ParsedAWG;

export interface ParseResult {
	success: boolean;
	config?: ParsedConfig;
	error?: string;
}

/**
 * Parse a vless:// URI
 * Format: vless://uuid@server:port?params#name
 */
export function parseVless(uri: string): ParseResult {
	try {
		if (!uri.startsWith('vless://')) {
			return { success: false, error: 'Invalid VLESS URI: must start with vless://' };
		}

		// Remove scheme
		const content = uri.slice(8);

		// Split by # to get name
		const [mainPart, ...nameParts] = content.split('#');
		const name = nameParts.length > 0 ? decodeURIComponent(nameParts.join('#')) : 'VLESS';

		// Split by @ to get uuid and server
		const atIndex = mainPart.indexOf('@');
		if (atIndex === -1) {
			return { success: false, error: 'Invalid VLESS URI: missing @' };
		}

		const uuid = mainPart.slice(0, atIndex);
		const serverPart = mainPart.slice(atIndex + 1);

		// Split by ? to get params
		const [hostPort, queryString] = serverPart.split('?');

		// Parse host:port
		const colonIndex = hostPort.lastIndexOf(':');
		if (colonIndex === -1) {
			return { success: false, error: 'Invalid VLESS URI: missing port' };
		}

		const server = hostPort.slice(0, colonIndex);
		const port = parseInt(hostPort.slice(colonIndex + 1), 10);

		if (isNaN(port) || port < 1 || port > 65535) {
			return { success: false, error: 'Invalid VLESS URI: invalid port' };
		}

		// Parse query params
		const params = new URLSearchParams(queryString || '');

		const config: ParsedVless = {
			type: 'vless',
			name,
			server,
			port,
			uuid,
		};

		// Optional params
		if (params.get('flow')) config.flow = params.get('flow')!;
		if (params.get('security')) config.security = params.get('security')!;
		if (params.get('sni')) config.sni = params.get('sni')!;
		if (params.get('fp')) config.fingerprint = params.get('fp')!;
		if (params.get('alpn')) config.alpn = params.get('alpn')!.split(',');
		if (params.get('type')) config.transport = params.get('type') as ParsedVless['transport'];
		if (params.get('path')) config.path = decodeURIComponent(params.get('path')!);
		if (params.get('host')) config.host = params.get('host')!;
		if (params.get('serviceName')) config.serviceName = params.get('serviceName')!;
		if (params.get('pbk')) config.pbk = params.get('pbk')!;
		if (params.get('sid')) config.sid = params.get('sid')!;

		return { success: true, config };
	} catch (err) {
		return { success: false, error: `Failed to parse VLESS URI: ${err}` };
	}
}

/**
 * Parse a hy2:// or hysteria2:// URI
 * Format: hy2://password@server:port?params#name
 */
export function parseHysteria2(uri: string): ParseResult {
	try {
		let content: string;
		if (uri.startsWith('hy2://')) {
			content = uri.slice(6);
		} else if (uri.startsWith('hysteria2://')) {
			content = uri.slice(12);
		} else {
			return { success: false, error: 'Invalid Hysteria2 URI: must start with hy2:// or hysteria2://' };
		}

		// Split by # to get name
		const [mainPart, ...nameParts] = content.split('#');
		const name = nameParts.length > 0 ? decodeURIComponent(nameParts.join('#')) : 'Hysteria2';

		// Split by @ to get password and server
		const atIndex = mainPart.indexOf('@');
		if (atIndex === -1) {
			return { success: false, error: 'Invalid Hysteria2 URI: missing @' };
		}

		const password = decodeURIComponent(mainPart.slice(0, atIndex));
		const serverPart = mainPart.slice(atIndex + 1);

		// Split by ? to get params
		const [hostPort, queryString] = serverPart.split('?');

		// Parse host:port
		const colonIndex = hostPort.lastIndexOf(':');
		if (colonIndex === -1) {
			return { success: false, error: 'Invalid Hysteria2 URI: missing port' };
		}

		const server = hostPort.slice(0, colonIndex);
		const port = parseInt(hostPort.slice(colonIndex + 1), 10);

		if (isNaN(port) || port < 1 || port > 65535) {
			return { success: false, error: 'Invalid Hysteria2 URI: invalid port' };
		}

		// Parse query params
		const params = new URLSearchParams(queryString || '');

		const config: ParsedHysteria2 = {
			type: 'hy2',
			name,
			server,
			port,
			password,
		};

		// Optional params
		if (params.get('sni')) config.sni = params.get('sni')!;
		if (params.get('insecure') === '1') config.insecure = true;
		if (params.get('obfs')) config.obfs = params.get('obfs')!;
		if (params.get('obfs-password')) config.obfsPassword = params.get('obfs-password')!;

		return { success: true, config };
	} catch (err) {
		return { success: false, error: `Failed to parse Hysteria2 URI: ${err}` };
	}
}

/**
 * Decode SIP002 userinfo: try plain base64 first, then base64url
 * (RFC 4648 §5: '-' for '+', '_' for '/', padding often stripped),
 * finally fall back to percent-encoded plaintext.
 */
function decodeSsUserinfo(userinfo: string): string {
	try {
		return atob(userinfo);
	} catch {
		// not plain base64 — try base64url below
	}
	try {
		const normalized = userinfo.replace(/-/g, '+').replace(/_/g, '/');
		const padded = normalized + '='.repeat((4 - (normalized.length % 4)) % 4);
		return atob(padded);
	} catch {
		// plain "method:password" userinfo always contains ':', which is never
		// valid base64, so it safely lands here
		return decodeURIComponent(userinfo);
	}
}

/**
 * Parse an ss:// URI (SIP002 format)
 * Format: ss://BASE64(method:password)@server:port#name
 * Also handles: ss://BASE64(method:password)@server:port/?plugin=xxx#name
 */
export function parseShadowsocks(uri: string): ParseResult {
	try {
		if (!uri.startsWith('ss://')) {
			return { success: false, error: 'Invalid Shadowsocks URI: must start with ss://' };
		}

		const content = uri.slice(5);

		// Split by # to get name
		const [mainPart, ...nameParts] = content.split('#');
		const name = nameParts.length > 0 ? decodeURIComponent(nameParts.join('#')) : 'Shadowsocks';

		let server: string, port: number, method: string, password: string;
		let plugin: string | undefined;
		let pluginOpts: string | undefined;

		// Two formats:
		// 1. ss://BASE64(method:password)@server:port
		// 2. ss://method:password@server:port (plain, less common)
		const atIndex = mainPart.lastIndexOf('@');
		if (atIndex === -1) {
			// Entire userinfo+host might be base64 encoded
			// Try decoding the whole thing (some clients encode everything)
			try {
				const decoded = atob(mainPart.split('?')[0].split('/')[0]);
				const decodedAtIndex = decoded.lastIndexOf('@');
				if (decodedAtIndex === -1) {
					return { success: false, error: 'Invalid Shadowsocks URI: missing @' };
				}
				const userinfo = decoded.slice(0, decodedAtIndex);
				const hostPart = decoded.slice(decodedAtIndex + 1);
				const colonIdx = userinfo.indexOf(':');
				if (colonIdx === -1) {
					return { success: false, error: 'Invalid Shadowsocks URI: missing method:password' };
				}
				method = userinfo.slice(0, colonIdx);
				password = userinfo.slice(colonIdx + 1);
				const hostColonIdx = hostPart.lastIndexOf(':');
				server = hostPart.slice(0, hostColonIdx);
				port = parseInt(hostPart.slice(hostColonIdx + 1), 10);
			} catch {
				return { success: false, error: 'Invalid Shadowsocks URI: cannot decode base64' };
			}
		} else {
			const userinfo = mainPart.slice(0, atIndex);
			const serverPart = mainPart.slice(atIndex + 1);

			// Parse server:port (may have query params)
			const [hostPort, queryString] = serverPart.split('?');
			// Remove trailing slash if present
			const cleanHostPort = hostPort.replace(/\/$/, '');
			const colonIndex = cleanHostPort.lastIndexOf(':');
			if (colonIndex === -1) {
				return { success: false, error: 'Invalid Shadowsocks URI: missing port' };
			}
			server = cleanHostPort.slice(0, colonIndex);
			port = parseInt(cleanHostPort.slice(colonIndex + 1), 10);

			// Parse query params for plugin
			if (queryString) {
				const params = new URLSearchParams(queryString);
				if (params.get('plugin')) {
					const pluginStr = decodeURIComponent(params.get('plugin')!);
					const semiIdx = pluginStr.indexOf(';');
					if (semiIdx !== -1) {
						plugin = pluginStr.slice(0, semiIdx);
						pluginOpts = pluginStr.slice(semiIdx + 1);
					} else {
						plugin = pluginStr;
					}
				}
			}

			// Decode userinfo (base64, base64url, or plain method:password)
			const decoded = decodeSsUserinfo(userinfo);
			const colonIdx = decoded.indexOf(':');
			if (colonIdx === -1) {
				return { success: false, error: 'Invalid Shadowsocks URI: missing method:password' };
			}
			method = decoded.slice(0, colonIdx);
			password = decoded.slice(colonIdx + 1);
		}

		if (isNaN(port) || port < 1 || port > 65535) {
			return { success: false, error: 'Invalid Shadowsocks URI: invalid port' };
		}

		const config: ParsedShadowsocks = {
			type: 'ss',
			name,
			server,
			port,
			method,
			password,
		};

		if (plugin) config.plugin = plugin;
		if (pluginOpts) config.pluginOpts = pluginOpts;

		return { success: true, config };
	} catch (err) {
		return { success: false, error: `Failed to parse Shadowsocks URI: ${err}` };
	}
}

/**
 * Parse a naive+https:// or naive+quic:// URI
 * Format: naive+https://[user:pass@]server:port[?params]#name
 */
export function parseNaive(uri: string): ParseResult {
	try {
		let content: string;
		let quic = false;
		if (uri.startsWith('naive+https://')) {
			content = uri.slice(14);
		} else if (uri.startsWith('naive+quic://')) {
			content = uri.slice(13);
			quic = true;
		} else {
			return { success: false, error: 'Invalid Naive URI: must start with naive+https:// or naive+quic://' };
		}

		// Split by # to get name
		const [mainPart, ...nameParts] = content.split('#');
		const name = nameParts.length > 0 ? decodeURIComponent(nameParts.join('#')) : 'NaiveProxy';

		// Strip query params (none are used by sing-box)
		const [authority] = mainPart.split('?');

		// Optional userinfo
		let username: string | undefined;
		let password: string | undefined;
		let hostPort = authority;
		const atIndex = authority.lastIndexOf('@');
		if (atIndex !== -1) {
			const userinfo = authority.slice(0, atIndex);
			hostPort = authority.slice(atIndex + 1);
			const colonIdx = userinfo.indexOf(':');
			if (colonIdx === -1) {
				username = decodeURIComponent(userinfo);
				password = '';
			} else {
				username = decodeURIComponent(userinfo.slice(0, colonIdx));
				password = decodeURIComponent(userinfo.slice(colonIdx + 1));
			}
		}

		// Parse host:port
		const colonIndex = hostPort.lastIndexOf(':');
		if (colonIndex === -1) {
			return { success: false, error: 'Invalid Naive URI: missing port' };
		}
		const server = hostPort.slice(0, colonIndex);
		const port = parseInt(hostPort.slice(colonIndex + 1), 10);
		if (!server) {
			return { success: false, error: 'Invalid Naive URI: missing server' };
		}
		if (isNaN(port) || port < 1 || port > 65535) {
			return { success: false, error: 'Invalid Naive URI: invalid port' };
		}

		const config: ParsedNaive = { type: 'naive', name, server, port };
		if (username !== undefined) {
			config.username = username;
			config.password = password;
		}
		if (quic) config.quic = true;

		return { success: true, config };
	} catch (err) {
		return { success: false, error: `Failed to parse Naive URI: ${err}` };
	}
}

/**
 * Parse an AmneziaWG/WireGuard configuration
 * INI-like format with [Interface] and [Peer] sections
 */
export function parseAWG(configText: string): ParseResult {
	try {
		const lines = configText.split('\n').map(l => l.trim());

		let currentSection = '';
		const interfaceData: Record<string, string> = {};
		const peerData: Record<string, string> = {};

		for (const line of lines) {
			// Skip empty lines and comments
			if (!line || line.startsWith('#')) continue;

			// Section headers
			if (line.startsWith('[') && line.endsWith(']')) {
				currentSection = line.slice(1, -1).toLowerCase();
				continue;
			}

			// Key = Value
			const eqIndex = line.indexOf('=');
			if (eqIndex === -1) continue;

			const key = line.slice(0, eqIndex).trim().toLowerCase();
			const value = line.slice(eqIndex + 1).trim();

			if (currentSection === 'interface') {
				interfaceData[key] = value;
			} else if (currentSection === 'peer') {
				peerData[key] = value;
			}
		}

		// Validate required fields
		if (!interfaceData['privatekey']) {
			return { success: false, error: 'Missing PrivateKey in [Interface]' };
		}
		if (!interfaceData['address']) {
			return { success: false, error: 'Missing Address in [Interface]' };
		}
		if (!peerData['publickey']) {
			return { success: false, error: 'Missing PublicKey in [Peer]' };
		}
		if (!peerData['endpoint']) {
			return { success: false, error: 'Missing Endpoint in [Peer]' };
		}

		// Parse endpoint
		const endpoint = peerData['endpoint'];
		const colonIndex = endpoint.lastIndexOf(':');
		if (colonIndex === -1) {
			return { success: false, error: 'Invalid Endpoint format (missing port)' };
		}

		const peerEndpoint = endpoint.slice(0, colonIndex);
		const peerPort = parseInt(endpoint.slice(colonIndex + 1), 10);

		if (isNaN(peerPort) || peerPort < 1 || peerPort > 65535) {
			return { success: false, error: 'Invalid Endpoint port' };
		}

		// Parse addresses (can be comma-separated)
		// Ensure CIDR notation - sing-box requires it
		const address = interfaceData['address'].split(',').map(a => {
			const addr = a.trim();
			// If already has CIDR, return as-is
			if (addr.includes('/')) return addr;
			// Add default CIDR: /32 for IPv4, /128 for IPv6
			return addr.includes(':') ? `${addr}/128` : `${addr}/32`;
		});

		// Parse DNS if present
		const dns = interfaceData['dns']
			? interfaceData['dns'].split(',').map(d => d.trim())
			: undefined;

		// Parse AllowedIPs
		const allowedIps = peerData['allowedips']
			? peerData['allowedips'].split(',').map(a => a.trim())
			: ['0.0.0.0/0', '::/0'];

		const config: ParsedAWG = {
			type: 'awg',
			name: 'AmneziaWG',
			privateKey: interfaceData['privatekey'],
			address,
			dns,
			mtu: interfaceData['mtu'] ? parseInt(interfaceData['mtu'], 10) : undefined,
			peerPublicKey: peerData['publickey'],
			peerEndpoint,
			peerPort,
			presharedKey: peerData['presharedkey'],
			allowedIps,
			persistentKeepalive: peerData['persistentkeepalive']
				? parseInt(peerData['persistentkeepalive'], 10)
				: undefined,
		};

		// Amnezia-specific obfuscation params
		if (interfaceData['jc']) config.jc = parseInt(interfaceData['jc'], 10);
		if (interfaceData['jmin']) config.jmin = parseInt(interfaceData['jmin'], 10);
		if (interfaceData['jmax']) config.jmax = parseInt(interfaceData['jmax'], 10);
		if (interfaceData['s1']) config.s1 = parseInt(interfaceData['s1'], 10);
		if (interfaceData['s2']) config.s2 = parseInt(interfaceData['s2'], 10);
		if (interfaceData['s3']) config.s3 = parseInt(interfaceData['s3'], 10);
		if (interfaceData['s4']) config.s4 = parseInt(interfaceData['s4'], 10);
		// H1-H4 are strings
		if (interfaceData['h1']) config.h1 = interfaceData['h1'];
		if (interfaceData['h2']) config.h2 = interfaceData['h2'];
		if (interfaceData['h3']) config.h3 = interfaceData['h3'];
		if (interfaceData['h4']) config.h4 = interfaceData['h4'];

		// Init parameters I1-I5 (stored as strings)
		if (interfaceData['i1']) config.i1 = interfaceData['i1'];
		if (interfaceData['i2']) config.i2 = interfaceData['i2'];
		if (interfaceData['i3']) config.i3 = interfaceData['i3'];
		if (interfaceData['i4']) config.i4 = interfaceData['i4'];
		if (interfaceData['i5']) config.i5 = interfaceData['i5'];

		return { success: true, config };
	} catch (err) {
		return { success: false, error: `Failed to parse AWG config: ${err}` };
	}
}

/**
 * Auto-detect and parse a VPN configuration
 * Supports: vless://, hy2://, hysteria2://, and AWG config text
 */
export function parseConfig(input: string): ParseResult {
	const trimmed = input.trim();

	if (trimmed.startsWith('vless://')) {
		return parseVless(trimmed);
	}

	if (trimmed.startsWith('hy2://') || trimmed.startsWith('hysteria2://')) {
		return parseHysteria2(trimmed);
	}

	if (trimmed.startsWith('ss://')) {
		return parseShadowsocks(trimmed);
	}

	if (trimmed.startsWith('naive+https://') || trimmed.startsWith('naive+quic://')) {
		return parseNaive(trimmed);
	}

	// Try AWG config (starts with [Interface])
	if (trimmed.includes('[Interface]') || trimmed.includes('[interface]')) {
		return parseAWG(trimmed);
	}

	return {
		success: false,
		error: 'Unknown configuration format. Supported: vless://, hy2://, hysteria2://, ss://, naive+https://, naive+quic://, or AmneziaWG config'
	};
}

/**
 * Convert parsed config to sing-box endpoint/outbound format
 */
export function toSingboxConfig(parsed: ParsedConfig): { endpoint?: Endpoint; outbound?: OutboundTyped; outboundTag: string } {
	switch (parsed.type) {
		case 'vless': {
			const outbound: Record<string, unknown> = {
				type: 'vless',
				tag: parsed.name.replace(/[^a-zA-Z0-9-_]/g, '-'),
				server: parsed.server,
				server_port: parsed.port,
				uuid: parsed.uuid,
			};

			if (parsed.flow) outbound.flow = parsed.flow;

			// TLS
			if (parsed.security === 'tls' || parsed.security === 'reality') {
				outbound.tls = {
					enabled: true,
					server_name: parsed.sni || parsed.server,
				};

				if (parsed.fingerprint) {
					(outbound.tls as Record<string, unknown>).utls = {
						enabled: true,
						fingerprint: parsed.fingerprint,
					};
				}

				if (parsed.alpn) {
					(outbound.tls as Record<string, unknown>).alpn = parsed.alpn;
				}

				// Reality
				if (parsed.security === 'reality' && parsed.pbk) {
					(outbound.tls as Record<string, unknown>).reality = {
						enabled: true,
						public_key: parsed.pbk,
						short_id: parsed.sid || '',
					};
				}
			}

			// Transport
			if (parsed.transport === 'ws') {
				outbound.transport = {
					type: 'ws',
					path: parsed.path || '/',
					headers: parsed.host ? { Host: parsed.host } : undefined,
				};
			} else if (parsed.transport === 'grpc') {
				outbound.transport = {
					type: 'grpc',
					service_name: parsed.serviceName || '',
				};
			} else if (parsed.transport === 'http') {
				outbound.transport = {
					type: 'http',
					path: parsed.path || '/',
					host: parsed.host ? [parsed.host] : undefined,
				};
			}

			return { outbound: outbound as unknown as OutboundTyped, outboundTag: outbound.tag as string };
		}

		case 'hy2': {
			const outbound: Record<string, unknown> = {
				type: 'hysteria2',
				tag: parsed.name.replace(/[^a-zA-Z0-9-_]/g, '-'),
				server: parsed.server,
				server_port: parsed.port,
				password: parsed.password,
			};

			// TLS
			outbound.tls = {
				enabled: true,
				server_name: parsed.sni || parsed.server,
				insecure: parsed.insecure || false,
			};

			// Obfuscation
			if (parsed.obfs) {
				outbound.obfs = {
					type: parsed.obfs,
					password: parsed.obfsPassword || '',
				};
			}

			return { outbound: outbound as unknown as OutboundTyped, outboundTag: outbound.tag as string };
		}

		case 'ss': {
			const outbound: Record<string, unknown> = {
				type: 'shadowsocks',
				tag: parsed.name.replace(/[^a-zA-Z0-9-_]/g, '-'),
				server: parsed.server,
				server_port: parsed.port,
				method: parsed.method,
				password: parsed.password,
			};

			if (parsed.plugin) outbound.plugin = parsed.plugin;
			if (parsed.pluginOpts) outbound.plugin_opts = parsed.pluginOpts;

			return { outbound: outbound as unknown as OutboundTyped, outboundTag: outbound.tag as string };
		}

		case 'naive': {
			const outbound: Record<string, unknown> = {
				type: 'naive',
				tag: parsed.name.replace(/[^a-zA-Z0-9-_]/g, '-'),
				server: parsed.server,
				server_port: parsed.port,
				tls: { enabled: true, server_name: parsed.server },
			};

			if (parsed.username) {
				outbound.username = parsed.username;
				outbound.password = parsed.password ?? '';
			}
			if (parsed.quic) outbound.quic = true;

			return { outbound: outbound as unknown as OutboundTyped, outboundTag: outbound.tag as string };
		}

		case 'awg': {
			const tag = parsed.name.replace(/[^a-zA-Z0-9-_]/g, '-');

			// Build peer object, only including defined values
			const peer: Record<string, unknown> = {
				address: parsed.peerEndpoint,
				port: parsed.peerPort,
				public_key: parsed.peerPublicKey,
				allowed_ips: parsed.allowedIps,
			};
			// Optional peer fields - only add if defined
			if (parsed.presharedKey) peer.preshared_key = parsed.presharedKey;
			if (parsed.persistentKeepalive) peer.persistent_keepalive_interval = parsed.persistentKeepalive;

			const endpoint: Record<string, unknown> = {
				type: 'awg',
				tag, // Endpoint tag can be used directly in selector/urltest
				private_key: parsed.privateKey,
				address: parsed.address,
				peers: [peer],
			};

			if (parsed.mtu) endpoint.mtu = parsed.mtu;

			// Amnezia obfuscation
			if (parsed.jc !== undefined) endpoint.jc = parsed.jc;
			if (parsed.jmin !== undefined) endpoint.jmin = parsed.jmin;
			if (parsed.jmax !== undefined) endpoint.jmax = parsed.jmax;
			if (parsed.s1 !== undefined) endpoint.s1 = parsed.s1;
			if (parsed.s2 !== undefined) endpoint.s2 = parsed.s2;
			if (parsed.s3 !== undefined) endpoint.s3 = parsed.s3;
			if (parsed.s4 !== undefined) endpoint.s4 = parsed.s4;
			if (parsed.h1 !== undefined) endpoint.h1 = parsed.h1;
			if (parsed.h2 !== undefined) endpoint.h2 = parsed.h2;
			if (parsed.h3 !== undefined) endpoint.h3 = parsed.h3;
			if (parsed.h4 !== undefined) endpoint.h4 = parsed.h4;
			// Init parameters (base64 encoded binary data)
			if (parsed.i1) endpoint.i1 = parsed.i1;
			if (parsed.i2) endpoint.i2 = parsed.i2;
			if (parsed.i3) endpoint.i3 = parsed.i3;
			if (parsed.i4) endpoint.i4 = parsed.i4;
			if (parsed.i5) endpoint.i5 = parsed.i5;

			// AWG endpoint can be used directly by tag in selector/urltest
			// No separate outbound needed - return endpoint tag as outboundTag for reference
			return { endpoint: endpoint as unknown as Endpoint, outboundTag: tag };
		}
	}
}

// ============================================================================
// Generic text/form parsing utilities
// ============================================================================

/**
 * Parse multiline text into array of trimmed non-empty strings
 */
export function parseLines(text: string | undefined | null): string[] {
	if (!text) return [];
	return text
		.split('\n')
		.map((s) => s.trim())
		.filter((s) => s.length > 0);
}

/**
 * Parse CSV (comma-separated) into array of trimmed non-empty strings
 */
export function parseCSV(text: string | undefined | null): string[] {
	if (!text) return [];
	return text
		.split(',')
		.map((s) => s.trim())
		.filter((s) => s.length > 0);
}

/**
 * Parse ports from CSV (e.g., "80, 443, 8080")
 * Returns only valid port numbers (1-65535)
 */
export function parsePorts(text: string | undefined | null): number[] {
	if (!text) return [];
	return text
		.split(',')
		.map((s) => parseInt(s.trim(), 10))
		.filter((n) => !isNaN(n) && n >= 1 && n <= 65535);
}

/**
 * Parse integers from CSV (e.g., "1, 2, 3")
 */
export function parseIntArray(text: string | undefined | null): number[] {
	if (!text) return [];
	return text
		.split(',')
		.map((s) => parseInt(s.trim(), 10))
		.filter((n) => !isNaN(n));
}

/**
 * Parse addresses from text (supports comma and newline separators)
 * Used for IP addresses, CIDR blocks, etc.
 */
export function parseAddresses(text: string | undefined | null): string[] {
	if (!text) return [];
	return text
		.split(/[,\n]/)
		.map((s) => s.trim())
		.filter((s) => s.length > 0);
}

/**
 * Format array to multiline text
 */
export function formatLines(arr: string[] | undefined | null): string {
	if (!arr || arr.length === 0) return '';
	return arr.join('\n');
}

/**
 * Format array to CSV
 */
export function formatCSV(arr: (string | number)[] | undefined | null): string {
	if (!arr || arr.length === 0) return '';
	return arr.join(', ');
}

/**
 * Format array to CSV (compact, no spaces)
 */
export function formatCSVCompact(arr: (string | number)[] | undefined | null): string {
	if (!arr || arr.length === 0) return '';
	return arr.join(',');
}

/**
 * Parse duration string (e.g., "30s", "5m", "1h") to display format
 * Returns the string as-is if valid, or empty string
 */
export function parseDuration(text: string | undefined | null): string {
	if (!text) return '';
	const trimmed = text.trim();
	// Basic validation: number followed by s/m/h/d
	if (/^\d+[smhd]$/.test(trimmed)) {
		return trimmed;
	}
	return trimmed; // Return as-is, let backend validate
}

/**
 * Parse key=value pairs from multiline text
 * Returns Record<string, string>
 */
export function parseKeyValuePairs(text: string | undefined | null): Record<string, string> {
	if (!text) return {};
	const result: Record<string, string> = {};
	const lines = text.split('\n');
	for (const line of lines) {
		const trimmed = line.trim();
		if (!trimmed || !trimmed.includes('=')) continue;
		const [key, ...valueParts] = trimmed.split('=');
		const value = valueParts.join('='); // Handle values with = in them
		if (key.trim()) {
			result[key.trim()] = value.trim();
		}
	}
	return result;
}

/**
 * Format key=value pairs to multiline text
 */
export function formatKeyValuePairs(obj: Record<string, string> | undefined | null): string {
	if (!obj) return '';
	return Object.entries(obj)
		.map(([k, v]) => `${k}=${v}`)
		.join('\n');
}

/**
 * Parse port ranges from CSV (e.g., "1000-2000, 3000-4000" or "1000:2000")
 * Returns array of range strings normalized to "start:end" format
 */
export function parsePortRanges(text: string | undefined | null): string[] {
	if (!text) return [];
	return text
		.split(',')
		.map((s) => s.trim().replace('-', ':'))
		.filter((s) => /^\d+:\d+$/.test(s));
}

/**
 * Try to extract domain from URL or return as-is
 */
export function extractDomain(input: string | undefined | null): string {
	if (!input) return '';
	const trimmed = input.trim();

	// Try to parse as URL
	try {
		const url = new URL(trimmed.startsWith('http') ? trimmed : `https://${trimmed}`);
		return url.hostname;
	} catch {
		// Not a URL, return trimmed input (might be just domain)
		return trimmed.replace(/^(https?:\/\/)?(www\.)?/, '').split('/')[0];
	}
}

/**
 * Parse reserved bytes for WireGuard (e.g., "0, 0, 0" or "[0, 0, 0]")
 */
export function parseReservedBytes(text: string | undefined | null): number[] {
	if (!text) return [];
	// Remove brackets if present
	const cleaned = text.replace(/[\[\]]/g, '');
	return parseIntArray(cleaned).slice(0, 3); // Max 3 bytes
}

/**
 * Format reserved bytes to display string
 */
export function formatReservedBytes(arr: number[] | undefined | null): string {
	if (!arr || arr.length === 0) return '';
	return arr.join(', ');
}
