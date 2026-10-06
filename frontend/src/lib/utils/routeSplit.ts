import type { ClashConnection, LeafHistoryPoint } from '$lib/types';
import { seriesRates } from './trafficSeries';

// The dashboard's route graph (#110): download that left through a direct
// outbound against everything that went through a proxy or an endpoint. A
// connection is sorted by its final outbound, which sing-box lists first in
// the chain.

// directTags returns the tags whose traffic counts as direct: every outbound
// of type direct, plus sing-box's implicit "direct" when no outbound claims
// that tag. PURE.
export function directTags(outbounds: { type: string; tag: string }[] | undefined): Set<string> {
	const out = new Set<string>();
	let taken = false;
	for (const ob of outbounds ?? []) {
		if (ob.type === 'direct') out.add(ob.tag);
		if (ob.tag === 'direct') taken = true;
	}
	if (!taken) out.add('direct');
	return out;
}

// leafRates is seriesRates for /traffic/history's `leaves`: two dense B/s
// arrays, direct and proxied. Rows without a chain ("-") are neither. PURE.
export function leafRates(
	rows: LeafHistoryPoint[] | undefined,
	direct: Set<string>,
	startTs: number,
	endTs: number,
	step: number,
	maxPoints: number
): { direct: number[]; proxy: number[] } {
	const pick = (want: boolean) =>
		seriesRates(
			(rows ?? [])
				.filter((r) => r.leaf !== '-' && direct.has(r.leaf) === want)
				.map((r) => ({ ts: r.ts, upload: 0, download: r.download })),
			startTs,
			endTs,
			step,
			maxPoints
		).down;
	return { direct: pick(true), proxy: pick(false) };
}

// liveSplit turns one connections-stream tick into the bytes each side
// downloaded since the previous tick. `seen` holds every connection's counter
// from that tick and is replaced in place; a connection new to it counts in
// full. A connection that closed between ticks loses its last delta — the same
// blind spot as the history sampler.
export function liveSplit(
	conns: ClashConnection[],
	direct: Set<string>,
	seen: Map<string, number>
): { direct: number; proxy: number } {
	let d = 0;
	let p = 0;
	const next = new Map<string, number>();
	for (const c of conns) {
		next.set(c.id, c.download);
		const leaf = c.chains?.[0];
		if (!leaf) continue;
		const delta = Math.max(0, c.download - (seen.get(c.id) ?? 0));
		if (direct.has(leaf)) d += delta;
		else p += delta;
	}
	seen.clear();
	for (const [k, v] of next) seen.set(k, v);
	return { direct: d, proxy: p };
}
