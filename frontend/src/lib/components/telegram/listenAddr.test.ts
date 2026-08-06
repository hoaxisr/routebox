import { describe, it, expect } from 'vitest';
import { splitListen, joinListen } from './listenAddr';

describe('splitListen', () => {
	it('splits a plain IPv4 host:port', () => {
		expect(splitListen('0.0.0.0:9443')).toEqual({ host: '0.0.0.0', port: 9443 });
	});

	it('splits a bracketed IPv6 host:port', () => {
		expect(splitListen('[::]:9443')).toEqual({ host: '::', port: 9443 });
		expect(splitListen('[2001:db8::1]:443')).toEqual({ host: '2001:db8::1', port: 443 });
	});

	it('keeps a hostname', () => {
		expect(splitListen('example.com:9443')).toEqual({ host: 'example.com', port: 9443 });
	});

	// A bare port is what "listen on everything" looks like in a lot of configs,
	// and the panel has to render it as an empty host rather than eat the port.
	it('accepts a bare :port', () => {
		expect(splitListen(':9443')).toEqual({ host: '', port: 9443 });
	});

	// An UNBRACKETED IPv6 has more than one colon and no port. Splitting on the
	// last colon would turn "::" into host ":" port NaN and silently move the
	// listener, so it has to be recognised as a host with no port.
	it('treats an unbracketed IPv6 as a host with no port', () => {
		expect(splitListen('::')).toEqual({ host: '::', port: null });
		expect(splitListen('2001:db8::1')).toEqual({ host: '2001:db8::1', port: null });
	});

	it('handles a host with no port', () => {
		expect(splitListen('0.0.0.0')).toEqual({ host: '0.0.0.0', port: null });
	});

	it('handles empty and garbage without throwing', () => {
		expect(splitListen('')).toEqual({ host: '', port: null });
		expect(splitListen('0.0.0.0:notaport')).toEqual({ host: '0.0.0.0', port: null });
	});
});

describe('joinListen', () => {
	it('joins IPv4', () => {
		expect(joinListen('0.0.0.0', 9443)).toBe('0.0.0.0:9443');
	});

	// Round-tripping must not lose the brackets, or sing-box and Go's net.Listen
	// both read "::" + ":9443" as a malformed address.
	it('brackets a bare IPv6 host', () => {
		expect(joinListen('::', 9443)).toBe('[::]:9443');
		expect(joinListen('2001:db8::1', 443)).toBe('[2001:db8::1]:443');
	});

	it('does not double-bracket an already bracketed host', () => {
		expect(joinListen('[::]', 9443)).toBe('[::]:9443');
	});

	it('emits a bare :port for an empty host', () => {
		expect(joinListen('', 9443)).toBe(':9443');
	});

	it('trims incidental whitespace', () => {
		expect(joinListen('  0.0.0.0  ', 9443)).toBe('0.0.0.0:9443');
	});

	// A cleared port field is null. Writing "host:null" would be saved verbatim
	// and the proxy would fail to bind, so the host alone is the honest result
	// and the server rejects it with a real message.
	it('omits a missing port', () => {
		expect(joinListen('0.0.0.0', null)).toBe('0.0.0.0');
		expect(joinListen('', null)).toBe('');
	});
});

describe('round trip', () => {
	for (const addr of ['0.0.0.0:9443', '[::]:9443', '[2001:db8::1]:443', ':9443', '127.0.0.1:1080']) {
		it(`preserves ${addr}`, () => {
			const { host, port } = splitListen(addr);
			expect(joinListen(host, port)).toBe(addr);
		});
	}
});
