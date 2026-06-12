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
