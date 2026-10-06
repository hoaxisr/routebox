import { describe, it, expect } from 'vitest';
import { directTags, leafRates, liveSplit, sharePaths } from './routeSplit';
import type { ClashConnection } from '$lib/types';

const conn = (id: string, download: number, chains: string[]) =>
	({ id, download, upload: 0, chains, metadata: {} }) as unknown as ClashConnection;

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
		expect(liveSplit([conn('a', 100, ['direct']), conn('b', 50, ['nl', 'proxy'])], direct, seen)).toEqual({ direct: 100, proxy: 50 });
		expect(liveSplit([conn('a', 130, ['direct']), conn('b', 40, ['nl', 'proxy']), conn('c', 5, [])], direct, seen)).toEqual({ direct: 30, proxy: 0 });
		expect([...seen.keys()]).toEqual(['a', 'b', 'c']);
	});
});

describe('sharePaths', () => {
	it('fills direct up to its share and leaves idle points as gaps', () => {
		// 3 points over w=100: 25 % direct, idle, 100 % direct (lone point → widened)
		const r = sharePaths([1, 0, 4], [3, 0, 0], 100, 80);
		expect(r.direct).toBe('M0 60L25 60L25 80L0 80ZM75 0L100 0L100 80L75 80Z');
		expect(r.proxy).toBe('M0 60L25 60L25 0L0 0ZM75 0L100 0L100 0L75 0Z');
	});
	it('is empty without data', () => {
		expect(sharePaths([], [], 100, 80)).toEqual({ direct: '', proxy: '', line: '' });
		expect(sharePaths([0, 0], [0, 0], 100, 80).direct).toBe('');
	});
});
