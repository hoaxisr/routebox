import { describe, it, expect } from 'vitest';
import type { Consumer, ConsumerLiveRow, ConsumersLive } from '$lib/types';
import {
	consumerKey, liveIndex, kindCounts, filterRows, sortRows, isActive, sumHistory,
	sumTrend, trendBytes, peakOf, manageHref, monitorHref, periodRange
} from './consumers';

const c = (over: Partial<Consumer>): Consumer => ({
	kind: 'awg', id: 'x', name: 'x', state: 'active', upload: 0, download: 0, history: [], ...over
});
const l = (over: Partial<ConsumerLiveRow>): ConsumerLiveRow => ({
	kind: 'awg', id: 'x', down_bps: 0, up_bps: 0, trend: [], ...over
});

describe('consumers helpers', () => {
	it('keys and indexes live rows by kind:id', () => {
		const live: ConsumersLive = { ts: 1, rows: [l({ kind: 'lan', id: '192.168.1.5', down_bps: 7 })], unavailable: {} };
		expect(consumerKey({ kind: 'lan', id: '192.168.1.5' })).toBe('lan:192.168.1.5');
		expect(liveIndex(live).get('lan:192.168.1.5')?.down_bps).toBe(7);
		expect(liveIndex(null).size).toBe(0);
	});

	it('counts only present kinds, in display order', () => {
		const rows = [c({ kind: 'lan' }), c({ kind: 'awg' }), c({ kind: 'lan', id: 'y' })];
		expect(kindCounts(rows)).toEqual([{ kind: 'awg', count: 1 }, { kind: 'lan', count: 2 }]);
		expect(filterRows(rows, 'lan')).toHaveLength(2);
		expect(filterRows(rows, 'all')).toHaveLength(3);
	});

	it('sorts by current rate, unknown rate last, then by period total', () => {
		const rows = [c({ id: 'a', download: 100 }), c({ id: 'b', download: 5 }), c({ id: 'c', download: 50 })];
		const live = new Map([['awg:b', l({ id: 'b', down_bps: 900 })], ['awg:c', l({ id: 'c', down_bps: 0 })]]);
		expect(sortRows(rows, live, 'now').map((r) => r.id)).toEqual(['b', 'c', 'a']);
		expect(sortRows(rows, live, 'period').map((r) => r.id)).toEqual(['a', 'c', 'b']);
	});

	it('is active when moving bytes or handshaking, never when suspended', () => {
		expect(isActive(c({}), l({ down_bps: 1 }))).toBe(true);
		expect(isActive(c({ online: true }))).toBe(true);
		expect(isActive(c({}))).toBe(false);
		expect(isActive(c({ state: 'suspended', online: true }), l({ down_bps: 1 }))).toBe(false);
	});

	it('sums histories by timestamp', () => {
		const rows = [
			c({ history: [{ ts: 60, upload: 1, download: 10 }, { ts: 120, upload: 2, download: 20 }] }),
			c({ history: [{ ts: 120, upload: 3, download: 30 }] })
		];
		expect(sumHistory(rows)).toEqual([{ ts: 60, upload: 1, download: 10 }, { ts: 120, upload: 5, download: 50 }]);
	});

	it('sums live trends point by point and turns a trend into bytes', () => {
		const a = l({ trend: [{ ts: 2, down_bps: 10, up_bps: 1 }, { ts: 4, down_bps: 20, up_bps: 2 }] });
		const b = l({ trend: [{ ts: 4, down_bps: 5, up_bps: 5 }] });
		expect(sumTrend([a, b])).toEqual({ down: [10, 25], up: [1, 7] });
		expect(trendBytes(a)).toEqual({ down: 60, up: 6 }); // 2 s per point
		expect(trendBytes(undefined)).toEqual({ down: 0, up: 0 });
	});

	it('weighs each live point by the gap since the previous one', () => {
		// First point: no predecessor, so one sampler interval (2 s). Then 2 s, then 6 s
		// (a tick the page missed or the sampler slept through).
		const r = l({ trend: [{ ts: 10, down_bps: 10, up_bps: 1 }, { ts: 12, down_bps: 20, up_bps: 2 }, { ts: 18, down_bps: 30, up_bps: 3 }] });
		expect(trendBytes(r)).toEqual({ down: 10 * 2 + 20 * 2 + 30 * 6, up: 1 * 2 + 2 * 2 + 3 * 6 });
	});

	it('finds the peak bucket', () => {
		expect(peakOf([{ ts: 1, upload: 1, download: 1 }, { ts: 2, upload: 0, download: 9 }])?.ts).toBe(2);
		expect(peakOf([])).toBeNull();
	});

	it('links to the right config page and back to the monitor', () => {
		expect(manageHref(c({ kind: 'awg' }))).toBe('/config/awg');
		expect(manageHref(c({ kind: 'user' }))).toBe('/config/users');
		expect(manageHref(c({ kind: 'mtproto' }))).toBe('/config/telegram');
		expect(manageHref(c({ kind: 'lan' }))).toBe('/config/clients');
		expect(monitorHref({ kind: 'awg', id: 'a+b/c=' })).toBe('/monitor/consumers?kind=awg&focus=awg%3Aa%2Bb%2Fc%3D');
		expect(periodRange('live')).toBe('1h');
		expect(periodRange('7d')).toBe('week');
	});
});
