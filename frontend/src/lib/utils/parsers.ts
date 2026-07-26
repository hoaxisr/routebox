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
	transport?: 'tcp' | 'ws' | 'grpc' | 'http' | 'httpupgrade' | 'xhttp';
	path?: string;
	host?: string;
	serviceName?: string;
	// Reality
	pbk?: string;
	sid?: string;
}

export interface ParsedTrojan {
	type: 'trojan';
	name: string;
	server: string;
	port: number;
	password: string;
	security?: string;
	// TLS options
	sni?: string;
	fingerprint?: string;
	alpn?: string[];
	// Transport
	transport?: 'tcp' | 'ws' | 'grpc' | 'http' | 'httpupgrade' | 'xhttp';
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

export interface ParsedMieru {
	type: 'mieru';
	name: string;
	server: string;
	server_port?: number;
	server_ports?: string[];
	transport: 'TCP' | 'UDP';
	username: string;
	password: string;
	multiplexing?: string;
	traffic_pattern?: string;
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

export type ParsedConfig = ParsedVless | ParsedTrojan | ParsedHysteria2 | ParsedShadowsocks | ParsedNaive | ParsedMieru | ParsedAWG;

export interface ParseResult {
	success: boolean;
	config?: ParsedConfig;
	error?: string;
	/** Set when a mierus:// link mixes TCP and UDP and no transport was chosen. */
	mieruTransports?: ('TCP' | 'UDP')[];
}

/**
 * Split a "host:port" string into host and numeric port.
 * Supports bracketed IPv6 notation: "[::1]:443" → { host: '::1', port: 443 }.
 * Returns null when the host is empty, the port is missing, or the port
 * is not an integer in 1-65535.
 */
export function splitHostPort(hostPort: string): { host: string; port: number } | null {
	let host: string;
	let portStr: string;
	if (hostPort.startsWith('[')) {
		const closeIdx = hostPort.indexOf(']');
		if (closeIdx === -1 || hostPort[closeIdx + 1] !== ':') return null;
		host = hostPort.slice(1, closeIdx);
		portStr = hostPort.slice(closeIdx + 2);
	} else {
		const colonIndex = hostPort.lastIndexOf(':');
		if (colonIndex === -1) return null;
		host = hostPort.slice(0, colonIndex);
		portStr = hostPort.slice(colonIndex + 1);
	}
	if (!host || !/^\d+$/.test(portStr)) return null;
	const port = parseInt(portStr, 10);
	if (port < 1 || port > 65535) return null;
	return { host, port };
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

		// Parse host:port (supports [IPv6]:port)
		const hp = splitHostPort(hostPort);
		if (!hp) {
			return { success: false, error: 'Invalid VLESS URI: invalid host:port' };
		}
		const server = hp.host;
		const port = hp.port;

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
 * Parse a trojan:// URI
 * Format: trojan://password@server:port?params#name
 */
export function parseTrojan(uri: string): ParseResult {
	try {
		if (!uri.startsWith('trojan://')) {
			return { success: false, error: 'Invalid Trojan URI: must start with trojan://' };
		}
		const content = uri.slice(9);
		const [mainPart, ...nameParts] = content.split('#');
		const name = nameParts.length > 0 ? decodeURIComponent(nameParts.join('#')) : 'Trojan';

		const atIndex = mainPart.indexOf('@');
		if (atIndex === -1) {
			return { success: false, error: 'Invalid Trojan URI: missing @' };
		}
		const password = decodeURIComponent(mainPart.slice(0, atIndex));
		const serverPart = mainPart.slice(atIndex + 1);
		const [hostPort, queryString] = serverPart.split('?');
		const hp = splitHostPort(hostPort);
		if (!hp) {
			return { success: false, error: 'Invalid Trojan URI: invalid host:port' };
		}
		const params = new URLSearchParams(queryString || '');
		const config: ParsedTrojan = { type: 'trojan', name, server: hp.host, port: hp.port, password };
		if (params.get('security')) config.security = params.get('security')!;
		if (params.get('sni')) config.sni = params.get('sni')!;
		if (params.get('fp')) config.fingerprint = params.get('fp')!;
		if (params.get('alpn')) config.alpn = params.get('alpn')!.split(',');
		if (params.get('type')) config.transport = params.get('type') as ParsedTrojan['transport'];
		if (params.get('path')) config.path = decodeURIComponent(params.get('path')!);
		if (params.get('host')) config.host = params.get('host')!;
		if (params.get('serviceName')) config.serviceName = params.get('serviceName')!;
		if (params.get('pbk')) config.pbk = params.get('pbk')!;
		if (params.get('sid')) config.sid = params.get('sid')!;
		return { success: true, config };
	} catch (err) {
		return { success: false, error: `Failed to parse Trojan URI: ${err}` };
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

		// Parse host:port (supports [IPv6]:port)
		const hp = splitHostPort(hostPort);
		if (!hp) {
			return { success: false, error: 'Invalid Hysteria2 URI: invalid host:port' };
		}
		const server = hp.host;
		const port = hp.port;

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
				const hp = splitHostPort(hostPart);
				if (!hp) {
					return { success: false, error: 'Invalid Shadowsocks URI: invalid host:port' };
				}
				server = hp.host;
				port = hp.port;
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
			const hp = splitHostPort(cleanHostPort);
			if (!hp) {
				return { success: false, error: 'Invalid Shadowsocks URI: invalid host:port' };
			}
			server = hp.host;
			port = hp.port;

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
			return { success: false, error: 'Invalid NaiveProxy URI: must start with naive+https:// or naive+quic://' };
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

		// Parse host:port (supports [IPv6]:port)
		const hp = splitHostPort(hostPort);
		if (!hp) {
			return { success: false, error: 'Invalid NaiveProxy URI: invalid host:port' };
		}
		const server = hp.host;
		const port = hp.port;

		const config: ParsedNaive = { type: 'naive', name, server, port };
		if (username !== undefined) {
			config.username = username;
			config.password = password;
		}
		if (quic) config.quic = true;

		return { success: true, config };
	} catch (err) {
		return { success: false, error: `Failed to parse NaiveProxy URI: ${err}` };
	}
}

const MIERU_MUX = new Set(['MULTIPLEXING_DEFAULT', 'MULTIPLEXING_OFF', 'MULTIPLEXING_LOW', 'MULTIPLEXING_MIDDLE', 'MULTIPLEXING_HIGH']);
const MIERU_TP_MAX = 64 * 1024;
const MIERU_MAX_PORTS = 64;

// Matches Go base64.StdEncoding exactly: strict alphabet [A-Za-z0-9+/],
// length % 4 === 0, correct '=' padding. A link that imports here must never
// fail Go-side at apply.
function isStdBase64(s: string): boolean {
	if (s.length % 4 !== 0) return false;
	return /^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(s);
}

// normalizeMieruPort: "443" | "9000-9010" (dash) → canonical, else null.
export function normalizeMieruPort(spec: string): string | null {
	const parts = spec.split('-');
	if (parts.length > 2) return null;
	const nums = parts.map((p) => (/^\d+$/.test(p) ? parseInt(p, 10) : NaN));
	if (nums.some((n) => !Number.isInteger(n) || n < 1 || n > 65535)) return null;
	if (nums.length === 2 && nums[0] > nums[1]) return null;
	return nums.length === 1 ? String(nums[0]) : `${nums[0]}-${nums[1]}`;
}

// parseMieruLink parses a plain-URL mierus:// link into ONE mieru outbound
// (single transport). For a mixed TCP+UDP link with no `transport` given it
// returns {success:false, mieruTransports:[...]} so the caller can offer a choice.
export function parseMieruLink(uri: string, transport?: 'TCP' | 'UDP'): ParseResult {
	try {
		if (!uri.startsWith('mierus://')) return { success: false, error: 'Invalid mieru link: must start with mierus://' };
		const u = new URL(uri);
		const username = decodeURIComponent(u.username);
		const password = decodeURIComponent(u.password);
		if (!username || !password) return { success: false, error: 'mieru link: username and password are required' };
		if (u.port) return { success: false, error: 'mieru link: port must be in the query (?port=…), not the host' };
		// WHATWG URL keeps IPv6 brackets in hostname ("[2001:db8::1]") — strip them.
		const host = u.hostname.startsWith('[') && u.hostname.endsWith(']')
			? u.hostname.slice(1, -1)
			: u.hostname;
		if (!host) return { success: false, error: 'mieru link: missing host' };
		const q = u.searchParams;
		const profile = q.get('profile');
		if (!profile) return { success: false, error: 'mieru link: profile is required' };

		const ports = q.getAll('port');
		if (ports.length === 0) return { success: false, error: 'mieru link: at least one port is required' };
		// Cap BEFORE any per-element work (hostile-link guard).
		if (ports.length > MIERU_MAX_PORTS) return { success: false, error: 'mieru link: too many ports' };
		let protocols = q.getAll('protocol').map((p) => p.toUpperCase());
		if (protocols.length === 0) return { success: false, error: 'mieru link: protocol is required (TCP or UDP)' };
		if (protocols.length === 1) protocols = ports.map(() => protocols[0]); // broadcast
		if (protocols.length !== ports.length) return { success: false, error: 'mieru link: port/protocol count mismatch' };
		if (protocols.some((p) => p !== 'TCP' && p !== 'UDP')) return { success: false, error: 'mieru link: protocol must be TCP or UDP' };

		const norm = ports.map(normalizeMieruPort);
		if (norm.some((n) => n === null)) return { success: false, error: 'mieru link: invalid port (want N or N-N, 1–65535)' };

		const available = Array.from(new Set(protocols)) as ('TCP' | 'UDP')[];
		if (!transport) {
			if (available.length > 1) return { success: false, error: 'mieru link mixes TCP and UDP — choose one', mieruTransports: available };
			transport = available[0];
		}
		// keep only ports of the chosen transport; de-dup identical specs
		// (element ordering in server_ports is not spec-significant)
		const chosen = Array.from(new Set(norm.filter((_, i) => protocols[i] === transport) as string[]));
		if (chosen.length === 0) return { success: false, error: `mieru link has no ${transport} ports` };

		const config: ParsedMieru = { type: 'mieru', name: profile, server: host, transport, username, password };
		const singles = chosen.filter((s) => !s.includes('-'));
		const ranges = chosen.filter((s) => s.includes('-'));
		const extraSingles: string[] = [];
		if (singles.length > 0) {
			config.server_port = parseInt(singles[0], 10);
			for (const s of singles.slice(1)) extraSingles.push(`${s}-${s}`); // degenerate range, never bare
		}
		const sp = [...ranges, ...extraSingles];
		if (sp.length > 0) config.server_ports = sp;

		const mux = q.get('multiplexing');
		if (mux) {
			if (!MIERU_MUX.has(mux)) return { success: false, error: `mieru link: invalid multiplexing ${mux}` };
			config.multiplexing = mux;
		}
		const tpRaw = q.get('traffic-pattern');
		if (tpRaw) {
			const tp = tpRaw.replace(/ /g, '+'); // URLSearchParams turned + into space
			// Size cap BEFORE the alphabet check (never regex a 90 KB hostile blob).
			if (tp.length > MIERU_TP_MAX) return { success: false, error: 'mieru link: traffic-pattern too large' };
			if (!isStdBase64(tp)) return { success: false, error: 'mieru link: traffic-pattern is not valid base64' };
			config.traffic_pattern = tp;
		}
		return { success: true, config };
	} catch {
		// FIXED message: the raw input always carries a password — never echo it.
		return { success: false, error: 'Failed to parse mieru link' };
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
 * Supports: vless://, trojan://, hy2://, hysteria2://, and AWG config text
 */
export function parseConfig(input: string): ParseResult {
	const trimmed = input.trim();

	if (trimmed.startsWith('vless://')) {
		return parseVless(trimmed);
	}

	if (trimmed.startsWith('trojan://')) {
		return parseTrojan(trimmed);
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
		error: 'Unknown configuration format. Supported: vless://, trojan://, hy2://, hysteria2://, ss://, naive+https://, naive+quic://, or AmneziaWG config'
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
			} else if (parsed.transport === 'httpupgrade') {
				outbound.transport = {
					type: 'httpupgrade',
					path: parsed.path || '/',
					host: parsed.host || undefined,
				};
			} else if (parsed.transport === 'xhttp') {
				outbound.transport = {
					type: 'xhttp',
					path: parsed.path || '/',
					host: parsed.host || undefined,
				};
			}

			return { outbound: outbound as unknown as OutboundTyped, outboundTag: outbound.tag as string };
		}

		case 'trojan': {
			const outbound: Record<string, unknown> = {
				type: 'trojan',
				tag: parsed.name.replace(/[^a-zA-Z0-9-_]/g, '-'),
				server: parsed.server,
				server_port: parsed.port,
				password: parsed.password,
			};
			if (parsed.security === 'tls' || parsed.security === 'reality') {
				const tls: Record<string, unknown> = { enabled: true, server_name: parsed.sni || parsed.server };
				if (parsed.fingerprint) tls.utls = { enabled: true, fingerprint: parsed.fingerprint };
				if (parsed.alpn) tls.alpn = parsed.alpn;
				if (parsed.security === 'reality' && parsed.pbk) {
					tls.reality = { enabled: true, public_key: parsed.pbk, short_id: parsed.sid || '' };
				}
				outbound.tls = tls;
			}
			// Host matrix: ws→headers.Host, httpupgrade→top-level host, grpc→service_name.
			if (parsed.transport === 'ws') {
				outbound.transport = { type: 'ws', path: parsed.path || '/', headers: parsed.host ? { Host: parsed.host } : undefined };
			} else if (parsed.transport === 'httpupgrade') {
				outbound.transport = { type: 'httpupgrade', path: parsed.path || '/', host: parsed.host || undefined };
			} else if (parsed.transport === 'grpc') {
				outbound.transport = { type: 'grpc', service_name: parsed.serviceName || '' };
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

		case 'mieru':
			// mieru import goes through OutboundForm.applyMieruConfig, never this
			// (test-only) converter. Explicit throw keeps the switch exhaustive.
			throw new Error('mieru is not supported by toSingboxConfig — use the OutboundForm import path');

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
 * Note: both keys and values are trimmed of surrounding whitespace;
 * values containing '=' are preserved (split on first '=' only).
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
