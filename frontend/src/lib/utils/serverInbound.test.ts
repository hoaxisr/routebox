import { describe, it, expect } from 'vitest';
import { buildServerInbound, parseServerInbound, type ServerFormState } from './serverInbound';

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
	obfsType: '',
	obfsPassword: ''
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
