import { describe, it, expect } from 'vitest';
import {
	buildServerInbound,
	parseServerInbound,
	validateServerInbound,
	hy2CongestionSummary,
	type ServerFormState
} from './serverInbound';

// Mock translator: returns the i18n key so tests can assert on error KEYS while
// still exercising the exact translation call sites the component uses.
const tr = (k: string) => k;

const base: ServerFormState = {
	type: 'vless',
	tag: 'vless-in',
	listen: '::',
	listenPort: 443,
	tlsMode: 'reality',
	tls: {
		server_name: 'www.microsoft.com',
		acme: { domain: '', email: '' },
		reality: { enabled: true, private_key: 'PRIV', short_id: '0123abcd' },
		certificate_path: '',
		key_path: ''
	},
	handshakeServer: '',
	handshakePort: 443,
	users: [{ name: 'phone', uuid: 'U1', flow: 'xtls-rprx-vision' }],
	upMbps: 0,
	downMbps: 0,
	bbrProfile: '',
	ignoreClientBandwidth: false,
	obfsType: '',
	obfsPassword: '',
	obfsMinPacketSize: 0,
	obfsMaxPacketSize: 0,
	transport: { type: 'raw' },
	mieruTransport: 'TCP',
	mieruListenPorts: '',
	trafficPattern: '',
	userHintIsMandatory: false
};

describe('buildServerInbound', () => {
	it('builds a vless reality inbound', () => {
		const ib = buildServerInbound(base);
		expect(ib.type).toBe('vless');
		expect(ib.listen_port).toBe(443);
		expect(ib.tls?.reality?.private_key).toBe('PRIV');
		expect(ib.tls?.reality?.short_id).toBe('0123abcd');
		expect(ib.tls?.acme).toBeUndefined();
		expect(ib.tls?.certificate_path).toBeUndefined();
		expect(ib.users?.[0]).toEqual({ name: 'phone', uuid: 'U1', flow: 'xtls-rprx-vision' });
	});

	it('builds an ACME naive inbound and drops reality/manual fields', () => {
		const ib = buildServerInbound({
			...base,
			type: 'naive',
			tlsMode: 'acme',
			tls: { ...base.tls, acme: { domain: 'vpn.example.com', email: 'a@b.c' } },
			users: [{ username: 'alice', password: 'pw' }]
		});
		expect(ib.type).toBe('naive');
		expect(ib.tls?.acme).toEqual({ domain: 'vpn.example.com', email: 'a@b.c' });
		expect(ib.tls?.reality).toBeUndefined();
		expect(ib.users?.[0]).toEqual({ username: 'alice', password: 'pw' });
	});

	it('builds a manual-tls hysteria2 inbound with obfs and bandwidth', () => {
		const ib = buildServerInbound({
			...base,
			type: 'hysteria2',
			tlsMode: 'manual',
			tls: { ...base.tls, certificate_path: '/c.pem', key_path: '/k.pem' },
			users: [{ name: 'phone', password: 'pw' }],
			upMbps: 100,
			downMbps: 200,
			obfsType: 'salamander',
			obfsPassword: 'o'
		});
		expect(ib.tls?.certificate_path).toBe('/c.pem');
		expect(ib.tls?.reality).toBeUndefined();
		expect(ib.up_mbps).toBe(100);
		expect(ib.down_mbps).toBe(200);
		expect(ib.obfs).toEqual({ type: 'salamander', password: 'o' });
	});

	it('round-trips through parseServerInbound', () => {
		const ib = buildServerInbound(base);
		const state = parseServerInbound(ib);
		expect(state.tlsMode).toBe('reality');
		expect(state.tls.reality.private_key).toBe('PRIV');
		expect(state.users[0].uuid).toBe('U1');
	});

	it('builds and round-trips a vless xhttp transport (top-level host, flow stripped)', () => {
		const ib = buildServerInbound({
			...base,
			transport: { type: 'xhttp', path: '/dl', host: 'cdn.example.com' }
		});
		// x_padding_bytes is not decoration: without it the fork refuses to load
		// the whole config ("x_padding_bytes cannot be disabled").
		expect(ib.transport).toEqual({
			type: 'xhttp',
			path: '/dl',
			host: 'cdn.example.com',
			x_padding_bytes: '100-1000'
		});
		// non-raw transport strips xtls-rprx-vision flow from users
		expect(ib.users?.[0]).toEqual({ name: 'phone', uuid: 'U1' });
		// The form state itself has no padding field — it is emitted, not edited.
		const state = parseServerInbound(ib);
		expect(state.transport).toEqual({ type: 'xhttp', path: '/dl', host: 'cdn.example.com' });
	});
});

describe('parseServerInbound', () => {
	it('parses a manual-tls inbound', () => {
		const state = parseServerInbound({
			type: 'naive', tag: 'n', listen: '::', listen_port: 443,
			tls: { enabled: true, certificate_path: '/c.pem', key_path: '/k.pem' },
			users: [{ username: 'u', password: 'p' }]
		});
		expect(state.tlsMode).toBe('manual');
		expect(state.tls.certificate_path).toBe('/c.pem');
		expect(state.tls.key_path).toBe('/k.pem');
	});

	it('parses an inbound with no tls (defaults to manual)', () => {
		const state = parseServerInbound({ type: 'vless', tag: 'v', listen_port: 443, users: [] });
		expect(state.tlsMode).toBe('manual');
		expect(state.listenPort).toBe(443);
	});

	// #59: the congestion-control keys are absent unless asked for — an empty
	// bbr_profile is not the same as "standard", the fork rejects unknown values
	// and an unasked-for ignore_client_bandwidth would silently turn BBR clients
	// away on every server the panel ever wrote.
	it('omits the congestion-control keys when they are left alone', () => {
		const ib = buildServerInbound({
			...base, type: 'hysteria2', tlsMode: 'manual',
			tls: { ...base.tls, certificate_path: '/c.pem', key_path: '/k.pem' },
			users: [{ name: 'p', password: 'pw' }]
		});
		expect(ib.bbr_profile).toBeUndefined();
		expect(ib.ignore_client_bandwidth).toBeUndefined();
	});

	// The keys live behind `type === 'hysteria2'` and that guard is load-bearing:
	// InboundForm keeps one state object across protocol switches, so a vless save
	// after touching these toggles would carry them — and the fork rejects an
	// unknown field at decode, before the VPN comes back up.
	it('never emits the congestion-control keys on another protocol', () => {
		const ib = buildServerInbound({
			...base, type: 'vless', bbrProfile: 'aggressive', ignoreClientBandwidth: true
		});
		expect(ib.bbr_profile).toBeUndefined();
		expect(ib.ignore_client_bandwidth).toBeUndefined();
	});

	it('round-trips the congestion-control keys', () => {
		const ib = buildServerInbound({
			...base, type: 'hysteria2', tlsMode: 'manual',
			tls: { ...base.tls, certificate_path: '/c.pem', key_path: '/k.pem' },
			users: [{ name: 'p', password: 'pw' }],
			bbrProfile: 'aggressive', ignoreClientBandwidth: true
		});
		expect(ib.bbr_profile).toBe('aggressive');
		expect(ib.ignore_client_bandwidth).toBe(true);
		const state = parseServerInbound(ib);
		expect(state.bbrProfile).toBe('aggressive');
		expect(state.ignoreClientBandwidth).toBe(true);
	});

	it('round-trips hysteria2 obfs and bandwidth', () => {
		const ib = buildServerInbound({
			...base, type: 'hysteria2', tlsMode: 'acme',
			tls: { ...base.tls, acme: { domain: 'd', email: 'e' } },
			users: [{ name: 'p', password: 'pw' }],
			upMbps: 50, downMbps: 80, obfsType: 'salamander', obfsPassword: 'x'
		});
		const state = parseServerInbound(ib);
		expect(state.upMbps).toBe(50);
		expect(state.downMbps).toBe(80);
		expect(state.obfsType).toBe('salamander');
		expect(state.obfsPassword).toBe('x');
	});

	it('emits a valid reality handshake derived from server_name', () => {
		const ib = buildServerInbound(base);
		expect(ib.tls?.reality?.handshake).toEqual({ server: 'www.microsoft.com', server_port: 443 });
		const state = parseServerInbound(ib);
		expect(state.handshakeServer).toBe('www.microsoft.com');
		expect(state.handshakePort).toBe(443);
	});
});

describe('buildServerInbound transport host-matrix', () => {
	it('raw omits the transport block entirely', () => {
		const ib = buildServerInbound({ ...base, transport: { type: 'raw' } });
		expect(ib.transport).toBeUndefined();
	});
	it('ws emits headers.Host and NO top-level host key', () => {
		const ib = buildServerInbound({ ...base, transport: { type: 'ws', path: '/p', host: 'cdn.example.com' } });
		expect(ib.transport).toEqual({ type: 'ws', path: '/p', headers: { Host: 'cdn.example.com' } });
		expect((ib.transport as unknown as Record<string, unknown>).host).toBeUndefined();
	});
	it('ws without host omits headers entirely', () => {
		const ib = buildServerInbound({ ...base, transport: { type: 'ws', path: '/p', host: '' } });
		expect(ib.transport).toEqual({ type: 'ws', path: '/p' });
	});
	it('httpupgrade emits top-level host string', () => {
		const ib = buildServerInbound({ ...base, transport: { type: 'httpupgrade', path: '/h', host: 'h.example.com' } });
		expect(ib.transport).toEqual({ type: 'httpupgrade', path: '/h', host: 'h.example.com' });
	});
	it('grpc emits service_name only', () => {
		const ib = buildServerInbound({ ...base, transport: { type: 'grpc', service_name: 'gsvc' } });
		expect(ib.transport).toEqual({ type: 'grpc', service_name: 'gsvc' });
	});
	it('inbound tls never carries utls (outbound-only)', () => {
		const ib = buildServerInbound({ ...base, transport: { type: 'ws', path: '/', host: '' } });
		expect((ib.tls as Record<string, unknown>).utls).toBeUndefined();
	});
});

describe('buildServerInbound trojan users', () => {
	it('trojan inbound carries password users', () => {
		const ib = buildServerInbound({
			...base, type: 'trojan', transport: { type: 'raw' },
			users: [{ name: 'dave', password: 'pw' }]
		});
		expect(ib.type).toBe('trojan');
		expect(ib.tls?.reality?.private_key).toBe('PRIV');
		expect((ib.users?.[0] as Record<string, unknown>).password).toBe('pw');
	});
});

describe('parseServerInbound transport host-matrix', () => {
	it('reads ws host from headers.Host', () => {
		const s = parseServerInbound({ type: 'vless', tag: 't', listen_port: 443,
			transport: { type: 'ws', path: '/p', headers: { Host: 'cdn.example.com' } } } as never);
		expect(s.transport).toEqual({ type: 'ws', path: '/p', host: 'cdn.example.com' });
	});
	it('reads httpupgrade host from top-level host', () => {
		const s = parseServerInbound({ type: 'vless', tag: 't', listen_port: 443,
			transport: { type: 'httpupgrade', path: '/h', host: 'h.example.com' } } as never);
		expect(s.transport).toEqual({ type: 'httpupgrade', path: '/h', host: 'h.example.com' });
	});
	it('grpc reads service_name', () => {
		const s = parseServerInbound({ type: 'trojan', tag: 't', listen_port: 443,
			transport: { type: 'grpc', service_name: 'gsvc' } } as never);
		expect(s.transport).toEqual({ type: 'grpc', service_name: 'gsvc' });
	});
	it('defaults to raw when no transport block', () => {
		const s = parseServerInbound({ type: 'vless', tag: 't', listen_port: 443 } as never);
		expect(s.transport).toEqual({ type: 'raw' });
	});
});

describe('parse∘build round-trip on transport', () => {
	const transports = [
		{ type: 'raw' as const },
		{ type: 'ws' as const, path: '/p', host: 'cdn.com' },
		{ type: 'ws' as const, path: '/p', host: '' },
		{ type: 'httpupgrade' as const, path: '/u', host: 'h.com' },
		{ type: 'grpc' as const, service_name: 'g' }
	];
	for (const tr of transports) {
		it(`round-trips ${tr.type}`, () => {
			const built = buildServerInbound({ ...base, transport: tr });
			const parsed = parseServerInbound(built);
			expect(parsed.transport.type).toBe(tr.type);
			if (tr.type === 'ws' || tr.type === 'httpupgrade') {
				expect(parsed.transport.path).toBe(tr.path);
				expect(parsed.transport.host ?? '').toBe(tr.host ?? '');
			}
			if (tr.type === 'grpc') expect(parsed.transport.service_name).toBe('g');
		});
	}
});

describe('buildServerInbound flow-strip (vless)', () => {
	it('strips vision flow from every vless user when transport is non-raw', () => {
		const ib = buildServerInbound({
			...base,
			users: [{ name: 'a', uuid: 'U1', flow: 'xtls-rprx-vision' }, { name: 'b', uuid: 'U2', flow: 'xtls-rprx-vision' }],
			transport: { type: 'ws', path: '/', host: '' }
		});
		for (const u of ib.users!) expect((u as Record<string, unknown>).flow).toBeUndefined();
	});
	it('keeps vision flow when transport is raw', () => {
		const ib = buildServerInbound({ ...base, transport: { type: 'raw' } });
		expect((ib.users![0] as Record<string, unknown>).flow).toBe('xtls-rprx-vision');
	});
	it('trojan users never carry flow regardless of transport', () => {
		const ib = buildServerInbound({
			...base, type: 'trojan',
			users: [{ name: 'a', password: 'pw' }],
			transport: { type: 'ws', path: '/', host: 'c' }
		});
		expect((ib.users![0] as Record<string, unknown>).flow).toBeUndefined();
		expect((ib.users![0] as Record<string, unknown>).password).toBe('pw');
	});
});

function mieruState(): ServerFormState {
	const s = parseServerInbound({ type: 'mieru', tag: 't', listen: '::', listen_port: 2020,
		transport: 'TCP', users: [{ name: 'a', password: 'p' }] });
	return s;
}

describe('serverInbound mieru', () => {
	it('builds a mieru inbound with no tls and string transport', () => {
		const s = mieruState();
		s.mieruTransport = 'UDP';
		const ib = buildServerInbound(s);
		expect(ib.type).toBe('mieru');
		expect(ib.transport).toBe('UDP');        // STRING, not an object
		expect(ib.tls).toBeUndefined();           // never emit tls for mieru
		expect(ib.users).toEqual([{ name: 'a', password: 'p' }]);
	});

	it('emits traffic_pattern and user_hint_is_mandatory only when set', () => {
		const s = mieruState();
		let ib = buildServerInbound(s);
		expect(ib.traffic_pattern).toBeUndefined();
		expect(ib.user_hint_is_mandatory).toBeUndefined();
		s.trafficPattern = 'YWJj';
		s.userHintIsMandatory = true;
		ib = buildServerInbound(s);
		expect(ib.traffic_pattern).toBe('YWJj');
		expect(ib.user_hint_is_mandatory).toBe(true);
	});

	it('round-trips transport through parse', () => {
		const ib = { type: 'mieru', tag: 't', listen: '::', listen_port: 2020,
			transport: 'UDP', users: [{ name: 'a', password: 'p' }] };
		const s = parseServerInbound(ib);
		expect(s.type).toBe('mieru');
		expect(s.mieruTransport).toBe('UDP');
	});

	// FU-3: parse robustness — a non-'TCP'/'UDP' transport (garbage object or an
	// unsupported string like 'quic') must fall back to the 'TCP' default.
	it('falls back to TCP when mieru transport is an object', () => {
		const s = parseServerInbound({ type: 'mieru', tag: 't', listen_port: 2020,
			transport: { type: 'ws', path: '/x' }, users: [{ name: 'a', password: 'p' }] } as never);
		expect(s.mieruTransport).toBe('TCP');
	});
	it('falls back to TCP when mieru transport is an unsupported string', () => {
		const s = parseServerInbound({ type: 'mieru', tag: 't', listen_port: 2020,
			transport: 'quic', users: [{ name: 'a', password: 'p' }] } as never);
		expect(s.mieruTransport).toBe('TCP');
	});
});

describe('validateServerInbound', () => {
	// FU-1: the make-or-break TLS-exemption. A mieru state with empty acme
	// domain/email and cert/key paths must NOT produce any TLS required-field
	// errors — those fields aren't rendered for mieru, so an error would be
	// invisible and permanently block Save.
	it('mieru with empty acme+cert fields produces NO tls errors', () => {
		const s = mieruState();
		s.tlsMode = 'acme';                 // even with a TLS mode selected, mieru skips the block
		s.tls.acme = { domain: '', email: '' };
		s.tls.certificate_path = '';
		s.tls.key_path = '';
		const errors = validateServerInbound(s, tr);
		expect(errors['acmeDomain']).toBeUndefined();
		expect(errors['acmeEmail']).toBeUndefined();
		expect(errors['certPath']).toBeUndefined();
		expect(errors['keyPath']).toBeUndefined();
	});

	// The exemption is type-specific, not a blanket skip: the SAME empty state as
	// a trojan with tlsMode='acme' MUST raise the acme errors.
	it('trojan with empty acme fields DOES produce acme errors (exemption is type-specific)', () => {
		const s = mieruState();
		s.type = 'trojan';
		s.tlsMode = 'acme';
		s.tls.acme = { domain: '', email: '' };
		s.users = [{ name: 'a', password: 'p' }]; // valid trojan cred, isolate the acme errors
		const errors = validateServerInbound(s, tr);
		expect(errors['acmeDomain']).toBe('errors.fieldNamedRequired');
		expect(errors['acmeEmail']).toBe('errors.fieldNamedRequired');
	});

	it('mieru with a user missing name or password produces userCred error', () => {
		const s = mieruState();
		s.users = [{ name: 'a', password: '' }];
		expect(validateServerInbound(s, tr)['userCred']).toBe('inbounds.server.needUserCred');
		s.users = [{ name: '', password: 'p' }];
		expect(validateServerInbound(s, tr)['userCred']).toBe('inbounds.server.needUserCred');
	});

	it('mieru with zero users produces needUser error', () => {
		const s = mieruState();
		s.users = [];
		const errors = validateServerInbound(s, tr);
		expect(errors['users']).toBe('inbounds.server.needUser');
		expect(errors['userCred']).toBeUndefined();
	});

	it('a valid mieru state produces no errors', () => {
		expect(validateServerInbound(mieruState(), tr)).toEqual({});
	});

	it('flags an out-of-range port', () => {
		const s = mieruState();
		s.listenPort = 70000;
		expect(validateServerInbound(s, tr)['port']).toBe('form.minValue');
	});

	// #48: with obfs enabled, an empty password would serialize `"password": ""`
	// — a config the binary accepts but no client can be pointed at.
	it('hysteria2 obfs with empty password produces obfsPassword error', () => {
		const s: ServerFormState = {
			...base, type: 'hysteria2', tlsMode: 'acme',
			tls: { ...base.tls, acme: { domain: 'd.example.com', email: 'a@b.c' } },
			users: [{ name: 'u', password: 'p' }],
			obfsType: 'salamander', obfsPassword: ''
		};
		expect(validateServerInbound(s, tr)['obfsPassword']).toBe('errors.fieldNamedRequired');
		expect(validateServerInbound({ ...s, obfsPassword: 'pw' }, tr)).toEqual({});
		// Off (empty type) needs no password.
		expect(validateServerInbound({ ...s, obfsType: '', obfsPassword: '' }, tr)).toEqual({});
	});

	it('obfs password check is hysteria2-specific', () => {
		const s: ServerFormState = { ...base, obfsType: 'salamander', obfsPassword: '' };
		expect(validateServerInbound(s, tr)['obfsPassword']).toBeUndefined();
	});
});

// Issue #37: the mieru server form gained port ranges. The shape that matters is
// "ranges only" — the config must then carry no listen_port at all, and reading
// it back must leave the port field empty rather than inventing 443.
describe('mieru listen_ports round-trip', () => {
	const mieru = (over: Partial<ServerFormState> = {}): ServerFormState => ({
		...base,
		type: 'mieru',
		tag: 'mieru-in',
		listenPort: 2020,
		users: [{ name: 'alice', password: 'pw' }],
		...over
	});

	it('emits ranges alongside a single port', () => {
		const ib = buildServerInbound(mieru({ mieruListenPorts: '25010-25012, 26000-26100' }));
		expect(ib.listen_port).toBe(2020);
		expect(ib.listen_ports).toEqual(['25010-25012', '26000-26100']);
	});

	it('omits listen_port entirely when only ranges are given', () => {
		const ib = buildServerInbound(mieru({ listenPort: 0, mieruListenPorts: '25010-25012' }));
		expect('listen_port' in ib).toBe(false);
		expect(ib.listen_ports).toEqual(['25010-25012']);
	});

	it('emits no listen_ports key when the field is empty', () => {
		const ib = buildServerInbound(mieru({ mieruListenPorts: '' }));
		expect('listen_ports' in ib).toBe(false);
	});

	it('round-trips through parseServerInbound', () => {
		const ib = buildServerInbound(mieru({ listenPort: 0, mieruListenPorts: '25010-25012' }));
		const back = parseServerInbound(ib);
		expect(back.listenPort).toBe(0);
		expect(back.mieruListenPorts).toBe('25010-25012');
	});

	it('accepts an empty port when ranges are set, rejects it otherwise', () => {
		const t = (k: string) => k;
		expect(validateServerInbound(mieru({ listenPort: 0, mieruListenPorts: '25010-25012' }), t).port).toBeUndefined();
		expect(validateServerInbound(mieru({ listenPort: 0, mieruListenPorts: '' }), t).port).toBeDefined();
	});

	it('reports a malformed range instead of dropping it', () => {
		const t = (k: string) => k;
		const errs = validateServerInbound(mieru({ mieruListenPorts: '25010-25012, nope' }), t);
		expect(errs.mieruListenPorts).toBeDefined();
	});
});

// #48: gecko is the second obfs type the fork speaks. Its packet-size bounds
// only work when client and server carry the same numbers, so the form has to
// round-trip them and refuse a half-configured pair.
describe('hysteria2 gecko obfuscation', () => {
	const trKey = (k: string) => k;
	const geckoState = (over: Partial<ServerFormState> = {}): ServerFormState => ({
		...base,
		type: 'hysteria2',
		tlsMode: 'acme',
		tls: { ...base.tls, acme: { domain: 'd.example.com', email: 'a@b.c' } },
		users: [{ name: 'u', password: 'p' }],
		obfsType: 'gecko',
		obfsPassword: 'pw',
		...over
	});

	it('emits packet sizes only when set', () => {
		expect(buildServerInbound(geckoState()).obfs).toEqual({ type: 'gecko', password: 'pw' });
		const ib = buildServerInbound(geckoState({ obfsMinPacketSize: 700, obfsMaxPacketSize: 1300 }));
		expect(ib.obfs).toEqual({
			type: 'gecko',
			password: 'pw',
			min_packet_size: 700,
			max_packet_size: 1300
		});
	});

	it('never emits packet sizes for salamander', () => {
		const ib = buildServerInbound(
			geckoState({ obfsType: 'salamander', obfsMinPacketSize: 700, obfsMaxPacketSize: 1300 })
		);
		expect(ib.obfs).toEqual({ type: 'salamander', password: 'pw' });
	});

	it('round-trips through parseServerInbound', () => {
		const state = geckoState({ obfsMinPacketSize: 700, obfsMaxPacketSize: 1300 });
		const back = parseServerInbound(buildServerInbound(state));
		expect(back.obfsType).toBe('gecko');
		expect(back.obfsMinPacketSize).toBe(700);
		expect(back.obfsMaxPacketSize).toBe(1300);
	});

	it('rejects one bound without the other', () => {
		expect(validateServerInbound(geckoState({ obfsMinPacketSize: 700 }), trKey)['obfsPacketSize'])
			.toBe('inbounds.server.obfsPacketSizeBoth');
		expect(validateServerInbound(geckoState({ obfsMaxPacketSize: 1300 }), trKey)['obfsPacketSize'])
			.toBe('inbounds.server.obfsPacketSizeBoth');
	});

	it('rejects min above max, accepts a valid pair and no pair at all', () => {
		expect(
			validateServerInbound(geckoState({ obfsMinPacketSize: 1400, obfsMaxPacketSize: 1300 }), trKey)[
				'obfsPacketSize'
			]
		).toBe('inbounds.server.obfsPacketSizeOrder');
		expect(validateServerInbound(geckoState({ obfsMinPacketSize: 700, obfsMaxPacketSize: 1300 }), trKey)).toEqual({});
		expect(validateServerInbound(geckoState(), trKey)).toEqual({});
	});

	it('leaves salamander alone when only one size lingers in state', () => {
		const s = geckoState({ obfsType: 'salamander', obfsMinPacketSize: 700 });
		expect(validateServerInbound(s, trKey)['obfsPacketSize']).toBeUndefined();
	});
});

// The caption under the server's "decide for the client" checkbox. Inverted, the
// panel tells the operator the opposite of what their server does — and one of
// these three states refuses clients rather than shaping them. Pinned against
// sing-quic hysteria2/service.go, which branches on exactly these two inputs.
describe('hy2CongestionSummary', () => {
	it('leaves the choice to the client while the switch is off', () => {
		expect(hy2CongestionSummary(false, 0)).toBe('ccClientDecides');
		expect(hy2CongestionSummary(false, 200)).toBe('ccClientDecides');
	});

	it('forces BBR on everyone when the switch is on and Down is open', () => {
		expect(hy2CongestionSummary(true, 0)).toBe('ccBbrOnly');
	});

	it('refuses rate-less clients when the switch is on and Down is capped', () => {
		expect(hy2CongestionSummary(true, 200)).toBe('ccBrutalOnly');
	});
});
