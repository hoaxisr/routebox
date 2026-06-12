import type { Inbound, ServerInboundUser, ServerTlsConfig } from '$lib/types';

export type TlsMode = 'acme' | 'reality' | 'manual';
export type ServerInboundType = 'vless' | 'naive' | 'hysteria2';

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
	users: ServerInboundUser[];
	upMbps: number;
	downMbps: number;
	obfsType: string;
	obfsPassword: string;
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

	const tls: ServerTlsConfig = { enabled: true };
	if (s.tlsMode === 'acme') {
		tls.acme = { domain: s.tls.acme.domain.trim(), email: s.tls.acme.email.trim() };
	} else if (s.tlsMode === 'reality') {
		tls.server_name = s.tls.server_name.trim();
		tls.reality = {
			enabled: true,
			private_key: s.tls.reality.private_key,
			short_id: s.tls.reality.short_id.trim()
		};
	} else {
		tls.certificate_path = s.tls.certificate_path.trim();
		tls.key_path = s.tls.key_path.trim();
		if (s.tls.server_name.trim()) tls.server_name = s.tls.server_name.trim();
	}
	ib.tls = tls;

	ib.users = s.users.map((u) => ({ ...u }));

	if (s.type === 'hysteria2') {
		if (s.upMbps > 0) ib.up_mbps = s.upMbps;
		if (s.downMbps > 0) ib.down_mbps = s.downMbps;
		if (s.obfsType) ib.obfs = { type: s.obfsType, password: s.obfsPassword };
	}

	return ib;
}

// parseServerInbound is the inverse used to populate the form when editing.
export function parseServerInbound(ib: Inbound): ServerFormState {
	const tls = ib.tls ?? {};
	let tlsMode: TlsMode = 'manual';
	if (tls.acme) tlsMode = 'acme';
	else if (tls.reality) tlsMode = 'reality';

	return {
		type: (ib.type as ServerInboundType) ?? 'vless',
		tag: ib.tag ?? '',
		listen: ib.listen ?? '::',
		listenPort: ib.listen_port ?? 443,
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
		users: (ib.users ?? []).map((u) => ({ ...u })),
		upMbps: ib.up_mbps ?? 0,
		downMbps: ib.down_mbps ?? 0,
		obfsType: ib.obfs?.type ?? '',
		obfsPassword: ib.obfs?.password ?? ''
	};
}
