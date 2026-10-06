import type {
	Consumer, ConsumerKind, ConsumerLiveRow, ConsumersLive, TrafficRange, TrafficSeriesPoint
} from '$lib/types';

// Pure helpers for /monitor/consumers, kept out of the page so they can be tested.

export type Period = 'live' | '1h' | '24h' | '7d' | '30d';
export type SortKey = 'now' | 'period';
export const PERIODS: Period[] = ['live', '1h', '24h', '7d', '30d'];
export const KINDS: ConsumerKind[] = ['user', 'awg', 'mtproto', 'lan'];
// Seconds between live points; the backend samples at this interval.
export const LIVE_INTERVAL_S = 2;

const RANGE: Record<Exclude<Period, 'live'>, TrafficRange> = { '1h': '1h', '24h': '24h', '7d': 'week', '30d': 'month' };

// The row list behind Live still needs a range for its totals; the last hour is
// the cheapest one that is never empty for someone active right now.
export function periodRange(p: Period): TrafficRange {
	return p === 'live' ? '1h' : RANGE[p];
}

export const consumerKey = (c: { kind: string; id: string }) => `${c.kind}:${c.id}`;

export function liveIndex(live: ConsumersLive | null): Map<string, ConsumerLiveRow> {
	return new Map((live?.rows ?? []).map((r) => [consumerKey(r), r]));
}

export function kindCounts(rows: Consumer[]): { kind: ConsumerKind; count: number }[] {
	return KINDS.map((kind) => ({ kind, count: rows.filter((r) => r.kind === kind).length })).filter((k) => k.count > 0);
}

export function filterRows(rows: Consumer[], kind: ConsumerKind | 'all'): Consumer[] {
	return kind === 'all' ? rows : rows.filter((r) => r.kind === kind);
}

const total = (r: Consumer) => r.upload + r.download;

// 'now': current rate, consumers with no live reading after those at 0; ties by
// period total, then name. 'period': period total, then name.
export function sortRows(rows: Consumer[], live: Map<string, ConsumerLiveRow>, key: SortKey): Consumer[] {
	const rate = (r: Consumer) => {
		const l = live.get(consumerKey(r));
		return l ? l.down_bps + l.up_bps : -1;
	};
	return [...rows].sort(
		(a, b) => (key === 'now' ? rate(b) - rate(a) : 0) || total(b) - total(a) || a.name.localeCompare(b.name)
	);
}

export function isActive(c: Consumer, l?: ConsumerLiveRow): boolean {
	if (c.state !== 'active') return false;
	return (l ? l.down_bps + l.up_bps > 0 : false) || !!c.online;
}

export function sumHistory(rows: Consumer[]): TrafficSeriesPoint[] {
	const m = new Map<number, TrafficSeriesPoint>();
	for (const r of rows)
		for (const p of r.history) {
			const a = m.get(p.ts) ?? { ts: p.ts, upload: 0, download: 0 };
			a.upload += p.upload;
			a.download += p.download;
			m.set(p.ts, a);
		}
	return [...m.values()].sort((a, b) => a.ts - b.ts);
}

// Live trends share tick timestamps (one sampler), so summing by ts lines them up.
export function sumTrend(rows: ConsumerLiveRow[]): { down: number[]; up: number[] } {
	const m = new Map<number, { d: number; u: number }>();
	for (const r of rows)
		for (const p of r.trend) {
			const a = m.get(p.ts) ?? { d: 0, u: 0 };
			a.d += p.down_bps;
			a.u += p.up_bps;
			m.set(p.ts, a);
		}
	const pts = [...m.entries()].sort((a, b) => a[0] - b[0]).map(([, v]) => v);
	return { down: pts.map((p) => p.d), up: pts.map((p) => p.u) };
}

// Bytes moved over the live window: each point is a rate held since the
// previous point. The first one has no predecessor and counts for one sampler
// interval; a gap wider than that (missed tick, sampler woke up) is weighed as
// what it is instead of being squeezed into a nominal 2 s.
export function trendBytes(l?: ConsumerLiveRow): { down: number; up: number } {
	let down = 0,
		up = 0,
		prev: number | null = null;
	for (const p of l?.trend ?? []) {
		const dt = prev === null ? LIVE_INTERVAL_S : Math.max(0, p.ts - prev);
		down += p.down_bps * dt;
		up += p.up_bps * dt;
		prev = p.ts;
	}
	return { down, up };
}

export function peakOf(h: TrafficSeriesPoint[]): TrafficSeriesPoint | null {
	let best: TrafficSeriesPoint | null = null;
	for (const p of h) if (!best || p.upload + p.download > best.upload + best.download) best = p;
	return best;
}

const MANAGE: Record<ConsumerKind, string> = {
	user: '/config/users',
	awg: '/config/awg',
	mtproto: '/config/telegram',
	lan: '/config/clients'
};
export const manageHref = (c: Consumer) => MANAGE[c.kind];

export function monitorHref(c: { kind: ConsumerKind; id: string }): string {
	return `/monitor/consumers?kind=${c.kind}&focus=${encodeURIComponent(consumerKey(c))}`;
}
