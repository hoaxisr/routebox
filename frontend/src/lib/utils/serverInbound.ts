import type { Inbound, ServerInboundUser, ServerTlsConfig } from '$lib/types';
import { parseMieruListenPorts, formatMieruListenPorts } from './mieruPorts';
import { XHTTP_DEFAULT_PADDING } from './xhttp';

export type TlsMode = 'acme' | 'reality' | 'manual' | 'panel';
export type ServerInboundType = 'vless' | 'trojan' | 'naive' | 'hysteria2' | 'mieru';

export const PANEL_CERT_PATH = '/etc/routebox/panel-cert/fullchain.pem';
export const PANEL_KEY_PATH = '/etc/routebox/panel-cert/key.pem';
// xhttp is deliberately absent: it is an OUTBOUND-only transport in the fork.
// Existing configs may still carry one, so parseServerInbound keeps reading it.
export type TransportType = 'raw' | 'ws' | 'grpc' | 'httpupgrade' | 'xhttp';

export interface ServerTransportState {
	type: TransportType;
	path?: string;
	host?: string;
	service_name?: string;
}

export interface ServerFormState {
	type: ServerInboundType;
	tag: string;
	listen: string;
	listenPort: number;
	tlsMode: TlsMode;
	tls: {
		server_name: string;
		acme: { domain: string; email: string };
		reality: { enabled: boolean; private_key: string; short_id: string };
		certificate_path: string;
		key_path: string;
	};
	handshakeServer: string;
	handshakePort: number;
	users: ServerInboundUser[];
	transport: ServerTransportState;
	upMbps: number;
	downMbps: number;
	/** hysteria2: '' leaves the fork's default (standard). */
	bbrProfile: string;
	/** hysteria2: server decides the congestion control instead of the client. */
	ignoreClientBandwidth: boolean;
	obfsType: string;
	obfsPassword: string;
	/** gecko only; 0 = leave hysteria's default (512 / 1200). */
	obfsMinPacketSize: number;
	obfsMaxPacketSize: number;
	mieruTransport: 'TCP' | 'UDP';
	/** Free-text "lo-hi" ranges the mieru server binds on top of listenPort (#37). */
	mieruListenPorts: string;
	trafficPattern: string;
	userHintIsMandatory: boolean;
}

// buildServerInbound converts form state into a sing-box inbound object,
// emitting only the TLS fields relevant to the selected mode.
export function buildServerInbound(s: ServerFormState): Inbound {
	const ib: Inbound = {
		type: s.type,
		tag: s.tag.trim(),
		listen: s.listen.trim() || '::',
		listen_port: s.listenPort
	};

	if (s.type === 'mieru') {
		ib.transport = s.mieruTransport;               // STRING "TCP"|"UDP"
		// Ranges are optional; with them set, listen_port may be left empty and
		// is then omitted entirely rather than emitted as a meaningless 0.
		const { ranges } = parseMieruListenPorts(s.mieruListenPorts);
		if (ranges.length > 0) ib.listen_ports = ranges;
		if (!s.listenPort) delete ib.listen_port;
		ib.users = s.users.map((u) => ({ ...u }));
		if (s.trafficPattern.trim()) ib.traffic_pattern = s.trafficPattern.trim();
		if (s.userHintIsMandatory) ib.user_hint_is_mandatory = true;
		return ib;
	}

	const tls: ServerTlsConfig = { enabled: true };
	if (s.tlsMode === 'acme') {
		tls.acme = { domain: s.tls.acme.domain.trim(), email: s.tls.acme.email.trim() };
	} else if (s.tlsMode === 'reality') {
		tls.server_name = s.tls.server_name.trim();
		const hsServer = s.handshakeServer.trim() || s.tls.server_name.trim();
		tls.reality = {
			enabled: true,
			private_key: s.tls.reality.private_key.trim(),
			short_id: s.tls.reality.short_id.trim(),
			handshake: { server: hsServer, server_port: s.handshakePort || 443 }
		};
	} else if (s.tlsMode === 'panel') {
		tls.certificate_path = PANEL_CERT_PATH;
		tls.key_path = PANEL_KEY_PATH;
		if (s.tls.server_name.trim()) tls.server_name = s.tls.server_name.trim();
	} else {
		tls.certificate_path = s.tls.certificate_path.trim();
		tls.key_path = s.tls.key_path.trim();
		if (s.tls.server_name.trim()) tls.server_name = s.tls.server_name.trim();
	}
	ib.tls = tls;

	ib.users = s.users.map((u) => ({ ...u }));

	// Flow strip: xtls-rprx-vision requires raw TCP. ServerUsers sets flow
	// unconditionally on new vless users, so this build-time delete is the real
	// enforcement point (the binary will NOT catch the mismatch at check time).
	if (s.type === 'vless' && s.transport && s.transport.type !== 'raw') {
		ib.users = ib.users.map((u) => {
			const { flow, ...rest } = u;
			return rest;
		});
	}

	// Transport (vless/trojan only). Host matrix: ws→headers.Host (NEVER a
	// top-level host key — the binary rejects `unknown field "host"`),
	// httpupgrade→top-level host string, grpc→service_name, raw→omit the block.
	if ((s.type === 'vless' || s.type === 'trojan') && s.transport && s.transport.type !== 'raw') {
		const tr = s.transport;
		if (tr.type === 'ws') {
			const ws: Record<string, unknown> = { type: 'ws', path: tr.path?.trim() || '/' };
			if (tr.host?.trim()) ws.headers = { Host: tr.host.trim() };
			ib.transport = ws as unknown as Inbound['transport'];
		} else if (tr.type === 'httpupgrade') {
			const hu: Record<string, unknown> = { type: 'httpupgrade', path: tr.path?.trim() || '/' };
			if (tr.host?.trim()) hu.host = tr.host.trim();
			ib.transport = hu as unknown as Inbound['transport'];
		} else if (tr.type === 'xhttp') {
			// xHTTP (awg3-xhttp fork): top-level host string like httpupgrade; mode defaults to auto.
			// x_padding_bytes is mandatory — see XHTTP_DEFAULT_PADDING.
			const xh: Record<string, unknown> = {
				type: 'xhttp',
				path: tr.path?.trim() || '/',
				x_padding_bytes: XHTTP_DEFAULT_PADDING
			};
			if (tr.host?.trim()) xh.host = tr.host.trim();
			ib.transport = xh as unknown as Inbound['transport'];
		} else if (tr.type === 'grpc') {
			ib.transport = { type: 'grpc', service_name: tr.service_name?.trim() || '' } as unknown as Inbound['transport'];
		}
	}

	if (s.type === 'hysteria2') {
		if (s.upMbps > 0) ib.up_mbps = s.upMbps;
		if (s.downMbps > 0) ib.down_mbps = s.downMbps;
		// Congestion control (#59). The rates are what actually pick the algorithm:
		// a client that announced a rate gets Brutal at it, one that announced none
		// gets BBR. ignore_client_bandwidth takes that decision away from the client
		// — with no down_mbps it forces BBR on everyone, with one it turns BBR
		// clients away. bbr_profile only applies to the connections that end up on
		// BBR; '' leaves the fork's default (standard).
		if (s.ignoreClientBandwidth) ib.ignore_client_bandwidth = true;
		if (s.bbrProfile) ib.bbr_profile = s.bbrProfile;
		if (s.obfsType) {
			ib.obfs = { type: s.obfsType, password: s.obfsPassword };
			// gecko takes packet-size bounds; both ends must agree, so unset stays
			// unset (hysteria then uses 512/1200 on both sides).
			if (s.obfsType === 'gecko') {
				if (s.obfsMinPacketSize > 0) ib.obfs.min_packet_size = s.obfsMinPacketSize;
				if (s.obfsMaxPacketSize > 0) ib.obfs.max_packet_size = s.obfsMaxPacketSize;
			}
		}
	}

	return ib;
}

// Translator matches the shape of svelte-i18n's $t: a key plus optional
// interpolation values, returning the localized string. Kept structural so this
// util stays decoupled from svelte-i18n while producing byte-identical messages.
export type Translator = (
	key: string,
	options?: { values?: Record<string, string | number | boolean | Date | null | undefined> }
) => string;

// validateServerInbound is the pure extraction of InboundForm.svelte's
// server-inbound validation. It owns the port range check, the ≥1-user guard,
// the TLS-mode required-field block (SKIPPED for mieru — the make-or-break
// exemption: mieru renders no TLS fields, so validating them would raise
// invisible, unclearable errors that permanently block Save), and the per-user
// credential checks. The component keeps the tag check and merges this result,
// so insertion order (port → users → tls → userCred) and messages stay identical.
export function validateServerInbound(state: ServerFormState, t: Translator): Record<string, string> {
	const errors: Record<string, string> = {};
	const req = (field: string) => t('errors.fieldNamedRequired', { values: { field } });

	// mieru may bind ONLY ranges, in which case the single port is left empty;
	// every other type needs a real port. An out-of-range value is always wrong.
	const mieruRanges = state.type === 'mieru' ? parseMieruListenPorts(state.mieruListenPorts) : null;
	const portOptional = !!mieruRanges && mieruRanges.ranges.length > 0;
	if (state.listenPort > 65535 || state.listenPort < 0) {
		errors['port'] = t('form.minValue', { values: { value: 1 } });
	} else if (state.listenPort < 1 && !portOptional) {
		errors['port'] = t('form.minValue', { values: { value: 1 } });
	}
	if (mieruRanges && mieruRanges.invalid.length > 0) {
		errors['mieruListenPorts'] = t('inbounds.server.mieruListenPortsInvalid', {
			values: { list: mieruRanges.invalid.join(', ') }
		});
	}
	if (state.users.length === 0) errors['users'] = t('inbounds.server.needUser');

	// TLS mode required fields. mieru has no TLS section, so its form never
	// renders these fields — validating them would raise invisible, unclearable
	// errors that permanently block Save. Skip the whole block for mieru.
	if (state.type !== 'mieru') {
		if (state.tlsMode === 'acme') {
			if (!state.tls.acme.domain.trim()) errors['acmeDomain'] = req(t('inbounds.server.domain'));
			if (!state.tls.acme.email.trim()) errors['acmeEmail'] = req(t('inbounds.server.email'));
		} else if (state.tlsMode === 'reality') {
			if (!state.tls.server_name.trim()) errors['serverName'] = req(t('inbounds.server.handshakeServer'));
			if (!state.tls.reality.private_key.trim()) errors['realityKey'] = t('inbounds.server.realityKeyRequired');
		} else if (state.tlsMode === 'manual') {
			if (!state.tls.certificate_path.trim()) errors['certPath'] = req(t('inbounds.server.certificatePath'));
			if (!state.tls.key_path.trim()) errors['keyPath'] = req(t('inbounds.server.keyPath'));
		}
		// 'panel' mode injects the canonical cert/key paths at build time — nothing to validate.
	}

	// hysteria2 obfs: with a type selected, the password is what actually keys
	// the obfuscation — an empty one would emit `"password": ""`, a config the
	// binary accepts but no client can be pointed at intentionally (#48).
	if (state.type === 'hysteria2' && state.obfsType && !state.obfsPassword.trim()) {
		errors['obfsPassword'] = req(t('inbounds.server.obfsPassword'));
	}

	// gecko: a lone bound is the trap — the other end keeps hysteria's default
	// for the missing one, the two sides disagree and the tunnel goes quiet.
	if (state.type === 'hysteria2' && state.obfsType === 'gecko') {
		const min = state.obfsMinPacketSize;
		const max = state.obfsMaxPacketSize;
		if ((min > 0) !== (max > 0)) {
			errors['obfsPacketSize'] = t('inbounds.server.obfsPacketSizeBoth');
		} else if (min > 0 && min > max) {
			errors['obfsPacketSize'] = t('inbounds.server.obfsPacketSizeOrder');
		}
	}

	// Per-user credential required fields
	for (const u of state.users) {
		if (state.type === 'vless' && !u.uuid?.trim()) { errors['userCred'] = t('inbounds.server.needUserCred'); break; }
		if (state.type === 'naive' && (!u.username?.trim() || !u.password?.trim())) { errors['userCred'] = t('inbounds.server.needUserCred'); break; }
		if (state.type === 'hysteria2' && !u.password?.trim()) { errors['userCred'] = t('inbounds.server.needUserCred'); break; }
		if (state.type === 'trojan' && !u.password?.trim()) { errors['userCred'] = t('inbounds.server.needUserCred'); break; }
		if (state.type === 'mieru' && (!u.name?.trim() || !u.password?.trim())) { errors['userCred'] = t('inbounds.server.needUserCred'); break; }
	}

	return errors;
}

// parseServerInbound is the inverse used to populate the form when editing.
export function parseServerInbound(ib: Inbound): ServerFormState {
	const tls = ib.tls ?? {};
	let tlsMode: TlsMode = 'manual';
	if (tls.acme) tlsMode = 'acme';
	else if (tls.reality) tlsMode = 'reality';
	else if (tls.certificate_path === PANEL_CERT_PATH) tlsMode = 'panel';

	const serverTypes = ['vless', 'trojan', 'naive', 'hysteria2', 'mieru'] as const;
	const type: ServerInboundType = (serverTypes as readonly string[]).includes(ib.type)
		? (ib.type as ServerInboundType)
		: 'vless';

	// Transport parse (inverse of the host matrix). ws host comes from
	// headers.Host, httpupgrade host from the top-level host string; default raw.
	const rawTr = (ib.transport ?? null) as Record<string, unknown> | null;
	let transport: ServerTransportState = { type: 'raw' };
	const trType = rawTr?.type as string | undefined;
	if (trType === 'ws') {
		const headers = (rawTr!.headers as Record<string, string> | undefined) ?? undefined;
		transport = { type: 'ws', path: (rawTr!.path as string) ?? '/', host: headers?.Host ?? '' };
	} else if (trType === 'httpupgrade') {
		transport = { type: 'httpupgrade', path: (rawTr!.path as string) ?? '/', host: (rawTr!.host as string) ?? '' };
	} else if (trType === 'xhttp') {
		transport = { type: 'xhttp', path: (rawTr!.path as string) ?? '/', host: (rawTr!.host as string) ?? '' };
	} else if (trType === 'grpc') {
		transport = { type: 'grpc', service_name: (rawTr!.service_name as string) ?? '' };
	}

	return {
		type,
		tag: ib.tag ?? '',
		listen: ib.listen ?? '::',
		// A mieru inbound that binds only ranges has no listen_port; 0 renders the
		// field empty rather than inventing 443 and silently adding a bind.
		listenPort: ib.listen_port ?? (Array.isArray(ib.listen_ports) && ib.listen_ports.length > 0 ? 0 : 443),
		tlsMode,
		tls: {
			server_name: tls.server_name ?? '',
			acme: { domain: tls.acme?.domain ?? '', email: tls.acme?.email ?? '' },
			reality: {
				enabled: true,
				private_key: tls.reality?.private_key ?? '',
				short_id: tls.reality?.short_id ?? ''
			},
			certificate_path: tls.certificate_path ?? '',
			key_path: tls.key_path ?? ''
		},
		handshakeServer: tls.reality?.handshake?.server ?? '',
		handshakePort: tls.reality?.handshake?.server_port ?? 443,
		users: (ib.users ?? []).map((u) => ({ ...u })),
		transport,
		upMbps: ib.up_mbps ?? 0,
		downMbps: ib.down_mbps ?? 0,
		bbrProfile: typeof ib.bbr_profile === 'string' ? ib.bbr_profile : '',
		ignoreClientBandwidth: ib.ignore_client_bandwidth === true,
		obfsType: ib.obfs?.type ?? '',
		obfsPassword: ib.obfs?.password ?? '',
		obfsMinPacketSize: ib.obfs?.min_packet_size ?? 0,
		obfsMaxPacketSize: ib.obfs?.max_packet_size ?? 0,
		mieruTransport: (ib.type === 'mieru' && (ib.transport === 'UDP' || ib.transport === 'TCP')) ? ib.transport : 'TCP',
		mieruListenPorts: formatMieruListenPorts(
			Array.isArray(ib.listen_ports) ? (ib.listen_ports as string[]) : undefined
		),
		trafficPattern: typeof ib.traffic_pattern === 'string' ? ib.traffic_pattern : '',
		userHintIsMandatory: ib.user_hint_is_mandatory === true
	};
}
