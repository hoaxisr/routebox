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

// sharePaths draws the route graph as a share of the whole (#110): direct
// fills from the bottom up to its share at each point, proxied fills the rest
// up to the top. A point without traffic has no share, so it is a gap, not a
// fake 0 % or 100 %; a lone point between gaps is widened to one step. PURE.
export function sharePaths(direct: number[], proxy: number[], w: number, h: number): { direct: string; proxy: string; line: string } {
	const n = direct.length;
	if (n === 0) return { direct: '', proxy: '', line: '' };
	const step = n > 1 ? w / (n - 1) : w;
	const xAt = (i: number) => (n > 1 ? i * step : w / 2);
	const f = (v: number) => +v.toFixed(2);
	let d = '';
	let p = '';
	let line = '';
	let i = 0;
	while (i < n) {
		if (!(direct[i] + proxy[i] > 0)) {
			i++;
			continue;
		}
		const pts: [number, number][] = [];
		for (; i < n && direct[i] + proxy[i] > 0; i++) pts.push([xAt(i), h - (direct[i] / (direct[i] + proxy[i])) * h]);
		if (pts.length === 1) {
			const [x, y] = pts[0];
			pts.splice(0, 1, [Math.max(0, x - step / 2), y], [Math.min(w, x + step / 2), y]);
		}
		const edge = pts.map(([x, y], k) => `${k ? 'L' : 'M'}${f(x)} ${f(y)}`).join('');
		const x0 = f(pts[0][0]);
		const x1 = f(pts[pts.length - 1][0]);
		d += `${edge}L${x1} ${h}L${x0} ${h}Z`;
		p += `${edge}L${x1} 0L${x0} 0Z`;
		line += edge;
	}
	return { direct: d, proxy: p, line };
}
