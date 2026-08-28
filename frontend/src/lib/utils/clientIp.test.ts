import { describe, it, expect } from 'vitest';
import type { ConnectionsResponse } from '$lib/types';
import { canonicalClientIp, canonicalizeConnections } from './clientIp';

describe('canonicalClientIp', () => {
	it('unmaps what a dual-stack inbound reports for an IPv4 client', () => {
		expect(canonicalClientIp('::ffff:203.0.113.7')).toBe('203.0.113.7');
		expect(canonicalClientIp('::FFFF:192.168.1.4')).toBe('192.168.1.4');
	});

	it('leaves a real address alone', () => {
		for (const ip of ['203.0.113.7', '2001:db8::1', '::1', 'fd00::ffff:1']) {
			expect(canonicalClientIp(ip)).toBe(ip);
		}
	});

	// A display and grouping key, not a validator: anything it cannot read is
	// passed through rather than turned into something wrong.
	it('passes through what it cannot read', () => {
		for (const s of ['', 'unknown', '::ffff:999.1.1.1', '::ffff:1.2.3']) {
			expect(canonicalClientIp(s)).toBe(s);
		}
	});
});

describe('canonicalizeConnections', () => {
	const resp = (sourceIP: string): ConnectionsResponse =>
		({
			downloadTotal: 1,
			uploadTotal: 2,
			connections: [{ id: 'a', metadata: { sourceIP, host: 'example.com' } }]
		}) as unknown as ConnectionsResponse;

	it('rewrites sourceIP and keeps the rest of the payload', () => {
		const out = canonicalizeConnections(resp('::ffff:10.0.0.5'));
		expect(out.connections[0].metadata.sourceIP).toBe('10.0.0.5');
		expect(out.connections[0].metadata.host).toBe('example.com');
		expect(out.downloadTotal).toBe(1);
		expect(out.uploadTotal).toBe(2);
	});

	it('survives a payload with no connections at all', () => {
		expect(() => canonicalizeConnections({} as ConnectionsResponse)).not.toThrow();
		const empty = { connections: [], downloadTotal: 0, uploadTotal: 0 };
		expect(canonicalizeConnections(empty).connections).toEqual([]);
	});
});
