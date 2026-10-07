import { describe, it, expect } from 'vitest';
import { directTags, leafRates, liveSplit, leafOf, bucketFlows, localFlows, rankLeaves, rankClients } from './routeSplit';
import type { ClashConnection } from '$lib/types';

const conn = (id: string, download: number, chains: string[], sourceIP = '') =>
	({ id, download, upload: 0, chains, metadata: { sourceIP } }) as unknown as ClashConnection;

describe('directTags', () => {
	it('collects direct outbounds and the implicit direct', () => {
		expect(directTags([{ type: 'direct', tag: 'lan-out' }, { type: 'vless', tag: 'nl' }])).toEqual(new Set(['lan-out', 'direct']));
	});
	it('drops the implicit direct when a proxy owns that tag', () => {
		expect(directTags([{ type: 'vless', tag: 'direct' }])).toEqual(new Set());
	});
});

describe('leafRates', () => {
	it('splits leaves into direct and proxied B/s', () => {
		const rows = [
			{ ts: 0, leaf: 'direct', download: 600 },
			{ ts: 0, leaf: 'nl', download: 120 },
			{ ts: 60, leaf: 'awg', download: 60 },
			{ ts: 60, leaf: '-', download: 999 }
		];
		expect(leafRates(rows, new Set(['direct']), 0, 120, 60, 10)).toEqual({ direct: [10, 0], proxy: [2, 1] });
	});
});

describe('liveSplit', () => {
	it('counts deltas by the first chain hop and remembers counters', () => {
		const seen = new Map<string, number>();
		const direct = new Set(['direct']);
		const a = liveSplit([conn('a', 100, ['direct']), conn('b', 50, ['nl', 'proxy'])], direct, seen);
		expect({ direct: a.direct, proxy: a.proxy }).toEqual({ direct: 100, proxy: 50 });
		const b = liveSplit([conn('a', 130, ['direct']), conn('b', 40, ['nl', 'proxy']), conn('c', 5, [])], direct, seen);
		expect({ direct: b.direct, proxy: b.proxy }).toEqual({ direct: 30, proxy: 0 });
		expect([...seen.keys()]).toEqual(['a', 'b', 'c']);
	});

	it('groups the tick deltas by source and final outbound', () => {
		const seen = new Map<string, number>();
		const direct = new Set(['direct']);
		liveSplit([conn('a', 100, ['direct'], '192.168.1.5'), conn('b', 10, ['nl'], '192.168.1.5')], direct, seen);
		const r = liveSplit(
			[
				conn('a', 160, ['direct'], '192.168.1.5'),
				conn('b', 30, ['nl'], '192.168.1.5'),
				conn('c', 7, ['nl', 'auto'], '192.168.1.5'),
				conn('d', 9, ['direct'], '192.168.1.9')
			],
			direct,
			seen
		);
		const sorted = [...r.flows].sort((x, y) => (x.source + x.leaf).localeCompare(y.source + y.leaf));
		expect(sorted).toEqual([
			{ source: '192.168.1.5', leaf: 'direct', download: 60 },
			{ source: '192.168.1.5', leaf: 'nl', download: 27 },
			{ source: '192.168.1.9', leaf: 'direct', download: 9 }
		]);
	});

	it('never yields negative bytes when a counter goes backwards', () => {
		const seen = new Map<string, number>([['a', 500]]);
		const r = liveSplit([conn('a', 20, ['nl'], '10.0.0.2')], new Set(['direct']), seen);
		expect(r.proxy).toBe(0);
		expect(r.flows).toEqual([]);
	});

	it('leaves chainless connections out of the flows', () => {
		const r = liveSplit([conn('a', 50, [], '10.0.0.2')], new Set(['direct']), new Map());
		expect(r.flows).toEqual([]);
	});
});

describe('leafOf / bucketFlows', () => {
	it('takes the first hop of a stored chain', () => {
		expect(leafOf('awg-2-fi → awg-auto')).toBe('awg-2-fi');
		expect(leafOf('direct')).toBe('direct');
		expect(leafOf('')).toBe('');
	});
	it('maps history buckets to flows', () => {
		expect(bucketFlows([{ source: '10.0.0.2', chain: 'nl → auto', download: 40 }])).toEqual([
			{ source: '10.0.0.2', leaf: 'nl', download: 40 }
		]);
	});
});

describe('localFlows', () => {
	it('keeps LAN (plain and IPv4-mapped) and sourceless flows, drops public sources', () => {
		const flows = [
			{ source: '192.168.1.5', leaf: 'nl', download: 1 },
			{ source: '', leaf: 'nl', download: 2 },
			{ source: '203.0.113.7', leaf: 'nl', download: 3 },
			{ source: '::ffff:192.168.1.7', leaf: 'nl', download: 4 }
		];
		expect(localFlows(flows).map((f) => f.download)).toEqual([1, 2, 4]);
	});
});

describe('rankLeaves', () => {
	const direct = new Set(['direct']);
	it('sums by outbound, sorts by bytes, cuts to top N with the rest', () => {
		const r = rankLeaves(
			[
				{ source: 'x', leaf: 'nl', download: 30 },
				{ source: 'y', leaf: 'direct', download: 100 },
				{ source: 'y', leaf: 'nl', download: 20 },
				{ source: 'x', leaf: 'de', download: 5 },
				{ source: 'x', leaf: 'fi', download: 4 },
				{ source: 'x', leaf: '-', download: 999 },
				{ source: 'x', leaf: '', download: 999 }
			],
			direct,
			2
		);
		expect(r.items).toEqual([
			{ leaf: 'direct', bytes: 100, direct: true },
			{ leaf: 'nl', bytes: 50, direct: false }
		]);
		expect(r.rest).toEqual({ count: 2, bytes: 9 });
	});
	it('is empty for a silent period', () => {
		expect(rankLeaves([{ source: 'x', leaf: 'nl', download: 0 }], direct, 5)).toEqual({ items: [], rest: { count: 0, bytes: 0 } });
	});
});

describe('rankClients', () => {
	const direct = new Set(['direct']);
	it('splits each client into direct and proxied download', () => {
		const r = rankClients(
			[
				{ source: 'a', leaf: 'direct', download: 10 },
				{ source: 'a', leaf: 'nl', download: 90 },
				{ source: 'b', leaf: 'direct', download: 40 },
				{ source: 'c', leaf: 'nl', download: 5 },
				{ source: 'c', leaf: '-', download: 500 }
			],
			direct,
			2
		);
		expect(r.items).toEqual([
			{ source: 'a', direct: 10, proxy: 90 },
			{ source: 'b', direct: 40, proxy: 0 }
		]);
		expect(r.rest).toEqual({ count: 1, bytes: 5 });
	});
	it('leaves out clients with no download', () => {
		expect(rankClients([{ source: 'a', leaf: 'nl', download: 0 }], direct, 5).items).toEqual([]);
	});
});
