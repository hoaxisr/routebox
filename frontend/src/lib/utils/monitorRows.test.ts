import { describe, it, expect } from 'vitest';
import { peersToRows, mtprotoClientsToRows, mergeMonitorRows, type MonitorRow } from './monitorRows';
import type { MtprotoClientTraffic } from '$lib/types';
import type { AwgPeerTraffic } from '$lib/types';

const peer = (over: Partial<AwgPeerTraffic> = {}): AwgPeerTraffic => ({
	public_key: 'abc=',
	name: 'laptop',
	address: '10.10.0.2/32',
	source: '10.10.0.2',
	last_handshake: 1753700000,
	online: true,
	upload: 10,
	download: 20,
	history: [{ ts: 1753699940, upload: 10, download: 20 }],
	...over
});

const user = (over: Partial<MonitorRow> = {}): MonitorRow => ({
	id: 'a1b2c3',
	name: 'alice',
	upload: 1,
	download: 1,
	total: 2,
	series: [],
	active: false,
	kind: 'user',
	...over
});

// Issue #40: AWG peers join the per-user monitor. They are not sing-box inbound
// users, so they arrive from a different endpoint and must slot into the same
// row list without colliding with panel users.
describe('peersToRows', () => {
	it('namespaces the id so a peer cannot collide with a panel user', () => {
		expect(peersToRows([peer({ public_key: 'a1b2c3' })])[0].id).toBe('peer:a1b2c3');
	});

	it('carries totals, series and the peer kind', () => {
		const [row] = peersToRows([peer()]);
		expect(row).toMatchObject({ name: 'laptop', upload: 10, download: 20, total: 30, kind: 'peer' });
		expect(row.series).toHaveLength(1);
	});

	// The dot must reflect the tunnel handshake, not bucket recency: a connected
	// peer that is merely idle is still online.
	it('takes liveness from the handshake, not from the series', () => {
		expect(peersToRows([peer({ online: true, history: [] })])[0].active).toBe(true);
		expect(peersToRows([peer({ online: false })])[0].active).toBe(false);
	});

	it('falls back to the tunnel IP when the peer has no name', () => {
		expect(peersToRows([peer({ name: '' })])[0].name).toBe('10.10.0.2');
	});

	it('tolerates a missing history array', () => {
		expect(peersToRows([peer({ history: undefined as never })])[0].series).toEqual([]);
	});

	it('maps an empty list to an empty list', () => {
		expect(peersToRows([])).toEqual([]);
	});
});

describe('mergeMonitorRows', () => {
	it('keeps both kinds and orders by total, heaviest first', () => {
		const merged = mergeMonitorRows(
			[user({ id: 'u1', name: 'alice', total: 5 }), user({ id: 'u2', name: 'bob', total: 50 })],
			peersToRows([peer({ public_key: 'p1', name: 'laptop', upload: 10, download: 10 })])
		);
		expect(merged.map((r) => r.name)).toEqual(['bob', 'laptop', 'alice']);
		expect(merged.filter((r) => r.kind === 'peer')).toHaveLength(1);
	});

	it('is a plain pass-through when there are no peers', () => {
		const users = [user()];
		expect(mergeMonitorRows(users, [])).toEqual(users);
	});
});

// MTProto clients join the same list (their traffic is per-client, same as a
// panel user's), but they are counted by the proxy's own relay rather than by
// sing-box, so they carry their own kind.
describe('mtprotoClientsToRows', () => {
	const client = (over: Partial<MtprotoClientTraffic> = {}): MtprotoClientTraffic => ({
		name: 'phone',
		upload: 10,
		download: 20,
		history: [],
		...over
	});

	it('namespaces the id so a client cannot collide with a panel user of the same name', () => {
		expect(mtprotoClientsToRows([client({ name: 'alice' })])[0].id).toBe('mtproto:alice');
	});

	it('carries the totals through', () => {
		const [row] = mtprotoClientsToRows([client({ upload: 3, download: 4 })]);
		expect(row.upload).toBe(3);
		expect(row.download).toBe(4);
		expect(row.total).toBe(7);
	});

	it('marks the row as an mtproto client', () => {
		expect(mtprotoClientsToRows([client()])[0].kind).toBe('mtproto');
	});

	it('tolerates a missing history', () => {
		const [row] = mtprotoClientsToRows([{ name: 'x', upload: 0, download: 0 } as MtprotoClientTraffic]);
		expect(row.series).toEqual([]);
	});

	it('infers liveness from recent buckets, since the endpoint reports no online flag', () => {
		const now = Math.floor(Date.now() / 1000);
		const live = mtprotoClientsToRows([client({ history: [{ ts: now - 30, upload: 1, download: 1 }] })]);
		const stale = mtprotoClientsToRows([client({ history: [{ ts: now - 7200, upload: 1, download: 1 }] })]);
		expect(live[0].active).toBe(true);
		expect(stale[0].active).toBe(false);
	});
});

describe('mergeMonitorRows with three sources', () => {
	const row = (id: string, total: number): MonitorRow => ({
		id,
		name: id,
		upload: total,
		download: 0,
		total,
		series: [],
		active: false,
		kind: 'user'
	});

	it('orders every source together, heaviest first', () => {
		const merged = mergeMonitorRows([row('u', 5)], [row('p', 50)], [row('m', 500)]);
		expect(merged.map((r) => r.id)).toEqual(['m', 'p', 'u']);
	});

	it('still works when the third source is absent', () => {
		expect(mergeMonitorRows([row('u', 1)], [row('p', 2)]).map((r) => r.id)).toEqual(['p', 'u']);
	});
});
