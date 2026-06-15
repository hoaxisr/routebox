import type { Inbound, ServerInboundUser, ServerTlsConfig } from '$lib/types';

export type TlsMode = 'acme' | 'reality' | 'manual';
export type ServerInboundType = 'vless' | 'trojan' | 'naive' | 'hysteria2';
export type TransportType = 'raw' | 'ws' | 'grpc' | 'httpupgrade';

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
		const hsServer = s.handshakeServer.trim() || s.tls.server_name.trim();
		tls.reality = {
			enabled: true,
			private_key: s.tls.reality.private_key.trim(),
			short_id: s.tls.reality.short_id.trim(),
			handshake: { server: hsServer, server_port: s.handshakePort || 443 }
		};
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
		} else if (tr.type === 'grpc') {
			ib.transport = { type: 'grpc', service_name: tr.service_name?.trim() || '' } as unknown as Inbound['transport'];
		}
	}

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

	const serverTypes = ['vless', 'trojan', 'naive', 'hysteria2'] as const;
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
	} else if (trType === 'grpc') {
		transport = { type: 'grpc', service_name: (rawTr!.service_name as string) ?? '' };
	}

	return {
		type,
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
		handshakeServer: tls.reality?.handshake?.server ?? '',
		handshakePort: tls.reality?.handshake?.server_port ?? 443,
		users: (ib.users ?? []).map((u) => ({ ...u })),
		transport,
		upMbps: ib.up_mbps ?? 0,
		downMbps: ib.down_mbps ?? 0,
		obfsType: ib.obfs?.type ?? '',
		obfsPassword: ib.obfs?.password ?? ''
	};
}
