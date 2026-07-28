import { describe, it, expect } from 'vitest';
import { inboundDisplayAddress } from './inboundAddress';

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

	it('shows a bare wildcard when no public host is configured', () => {
		expect(inboundDisplayAddress({ listen: '::', listen_port: 443 }, '')).toBe('*:443');
	});

	it('brackets an IPv6 public host so host:port stays parseable', () => {
		expect(inboundDisplayAddress({ listen: '::', listen_port: 443 }, '2001:db8::1')).toBe(
			'[2001:db8::1]:443'
		);
	});
});
