import { describe, it, expect } from 'vitest';
import { inboundDisplayAddress, isWildcardListen } from './inboundAddress';

// Exported for the server-inbound forms: a wildcard bind gets a "clients
// connect to <public_host>:<port>" hint under the Listen Address field (#37).
describe('isWildcardListen', () => {
	it.each(['', '::', '[::]', '::0', '0.0.0.0', '*', ' :: '])(
		'treats %p as a wildcard',
		(v) => expect(isWildcardListen(v)).toBe(true)
	);

	it.each(['127.0.0.1', '10.0.0.5', '2001:db8::1', 'fe80::1'])(
		'treats a specific bind %p as non-wildcard',
		(v) => expect(isWildcardListen(v)).toBe(false)
	);
});

// Issue #37: server inbounds bind the wildcard, so the inbound list rendered
// ":::443" — the one address no client can ever dial — for mieru, naive,
// hysteria2 and friends alike. Show what clients actually connect to.
describe('inboundDisplayAddress', () => {
	const host = 'vpn.example.com';

	it.each(['::', '[::]', '::0', '0.0.0.0', '*', ''])(
		'substitutes the public host for wildcard listen %p',
		(listen) => {
			expect(inboundDisplayAddress({ listen, listen_port: 443 }, host)).toBe('vpn.example.com:443');
		}
	);

	it('keeps a specific bind address — it is deliberate and not the public host', () => {
		expect(inboundDisplayAddress({ listen: '127.0.0.1', listen_port: 1080 }, host)).toBe(
			'127.0.0.1:1080'
		);
		expect(inboundDisplayAddress({ listen: '10.0.0.5', listen_port: 8080 }, host)).toBe(
			'10.0.0.5:8080'
		);
	});

	// An absent listen is a local inbound (mixed/socks): sing-box builds the bind
	// address with a 127.0.0.1 fallback, so it is NOT reachable from outside and
	// substituting a public host would overstate exposure.
	it('leaves an absent listen on the loopback default', () => {
		expect(inboundDisplayAddress({ listen: undefined, listen_port: undefined }, host)).toBe(
			'127.0.0.1:1080'
		);
	});

	// #46: a mieru inbound bound to ranges only has no listen_port — the old
	// `?? 1080` claimed it listened on 1080.
	it('shows the ranges when a mieru inbound binds only listen_ports', () => {
		expect(
			inboundDisplayAddress({ listen: '::', listen_ports: ['25010-25012'] }, host)
		).toBe('vpn.example.com:25010-25012');
	});

	it('lists the single port and the ranges together', () => {
		expect(
			inboundDisplayAddress(
				{ listen: '::', listen_port: 2020, listen_ports: ['25010-25012', '26000-26100'] },
				host
			)
		).toBe('vpn.example.com:2020,25010-25012,26000-26100');
	});

	it('shows a bare wildcard when no public host is configured', () => {
		expect(inboundDisplayAddress({ listen: '::', listen_port: 443 }, '')).toBe('*:443');
	});

	it('brackets an IPv6 public host so host:port stays parseable', () => {
		expect(inboundDisplayAddress({ listen: '::', listen_port: 443 }, '2001:db8::1')).toBe(
			'[2001:db8::1]:443'
		);
	});
});

// An inbound bound to the loopback in an out-of-the-box install is not a local
// inbound — it is one standing behind the front, which is exactly how the panel
// knows it is behind one. Its listen port is an implementation detail: clients
// reach it at the front's address, and showing 127.0.0.1:9444 tells the operator
// nothing they can use or hand to anyone.
describe('inboundDisplayAddress behind a front', () => {
	const host = 'vpn.example.com';

	it('shows the front address for a loopback-bound inbound', () => {
		expect(
			inboundDisplayAddress({ listen: '127.0.0.1', listen_port: 9444 }, host, 443)
		).toBe('vpn.example.com:443');
	});

	it('shows the loopback as-is when nothing fronts it', () => {
		expect(inboundDisplayAddress({ listen: '127.0.0.1', listen_port: 9444 }, host, 0)).toBe(
			'127.0.0.1:9444'
		);
	});

	// Without a public host there is nothing to substitute, and inventing one
	// would be worse than showing the bind address.
	it('shows the loopback as-is with no public host', () => {
		expect(inboundDisplayAddress({ listen: '127.0.0.1', listen_port: 9444 }, '', 443)).toBe(
			'127.0.0.1:9444'
		);
	});

	// A wildcard bind keeps its own port: the front does not carry it.
	it('leaves a wildcard-bound inbound on its own port', () => {
		expect(inboundDisplayAddress({ listen: '::', listen_port: 443 }, host, 443)).toBe(
			'vpn.example.com:443'
		);
	});
});
