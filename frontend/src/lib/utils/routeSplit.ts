import type { ClashConnection, LeafHistoryPoint } from '$lib/types';
import { seriesRates } from './trafficSeries';
import { isLocalClientIp } from './clientIp';

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

// One client's download through one final outbound. The live minute and the
// 1h/24h history are both reduced to these, so the rankings beside the graph
// count the same bytes whichever period is shown.
export interface RouteFlow {
	source: string;
	leaf: string;
	download: number;
}

// liveSplit turns one connections-stream tick into the bytes each side
// downloaded since the previous tick, and the same bytes per (source, final
// outbound). `seen` holds every connection's counter from that tick and is
// replaced in place; a connection new to it counts in full. A connection that
// closed between ticks loses its last delta — the same blind spot as the
// history sampler.
export function liveSplit(
	conns: ClashConnection[],
	direct: Set<string>,
	seen: Map<string, number>
): { direct: number; proxy: number; flows: RouteFlow[] } {
	let d = 0;
	let p = 0;
	const next = new Map<string, number>();
	const flows = new Map<string, RouteFlow>();
	for (const c of conns) {
		next.set(c.id, c.download);
		const leaf = c.chains?.[0];
		if (!leaf) continue;
		const delta = Math.max(0, c.download - (seen.get(c.id) ?? 0));
		if (delta === 0) continue;
		if (direct.has(leaf)) d += delta;
		else p += delta;
		const source = c.metadata?.sourceIP ?? '';
		const key = `${source}\n${leaf}`;
		const f = flows.get(key);
		if (f) f.download += delta;
		else flows.set(key, { source, leaf, download: delta });
	}
	seen.clear();
	for (const [k, v] of next) seen.set(k, v);
	return { direct: d, proxy: p, flows: [...flows.values()] };
}

// leafOf is the final outbound of a stored chain: sing-box lists it first,
// and history joins the hops with " → ". PURE.
export function leafOf(chain: string): string {
	const i = chain.indexOf(' → ');
	return i < 0 ? chain : chain.slice(0, i);
}

// bucketFlows reads /traffic/history buckets as flows. PURE.
export function bucketFlows(buckets: { source: string; chain: string; download: number }[]): RouteFlow[] {
	return buckets.map((b) => ({ source: b.source, leaf: leafOf(b.chain), download: b.download }));
}

// localFlows keeps the flows of this box's devices (and its own dials, which
// have no source), as localSourceConnections does for the live list (#102).
// PURE.
export function localFlows(flows: RouteFlow[]): RouteFlow[] {
	return flows.filter((f) => !f.source || isLocalClientIp(f.source));
}

export interface Ranked<T> {
	items: T[];
	rest: { count: number; bytes: number };
}

const counted = (f: RouteFlow) => f.download > 0 && f.leaf !== '' && f.leaf !== '-';

function cut<T>(all: T[], top: number, size: (x: T) => number): Ranked<T> {
	const rest = all.slice(top);
	return { items: all.slice(0, top), rest: { count: rest.length, bytes: rest.reduce((s, x) => s + size(x), 0) } };
}

// rankLeaves: download per final outbound, largest first, top N plus the
// rest summed. PURE.
export function rankLeaves(
	flows: RouteFlow[],
	direct: Set<string>,
	top: number
): Ranked<{ leaf: string; bytes: number; direct: boolean }> {
	const by = new Map<string, number>();
	for (const f of flows) if (counted(f)) by.set(f.leaf, (by.get(f.leaf) ?? 0) + f.download);
	const all = [...by]
		.map(([leaf, bytes]) => ({ leaf, bytes, direct: direct.has(leaf) }))
		.sort((a, b) => b.bytes - a.bytes || a.leaf.localeCompare(b.leaf));
	return cut(all, top, (x) => x.bytes);
}

// rankClients: each client's download split into direct and proxied, largest
// total first, top N plus the rest summed. PURE.
export function rankClients(
	flows: RouteFlow[],
	direct: Set<string>,
	top: number
): Ranked<{ source: string; direct: number; proxy: number }> {
	const by = new Map<string, { source: string; direct: number; proxy: number }>();
	for (const f of flows) {
		if (!counted(f)) continue;
		const r = by.get(f.source) ?? { source: f.source, direct: 0, proxy: 0 };
		if (direct.has(f.leaf)) r.direct += f.download;
		else r.proxy += f.download;
		by.set(f.source, r);
	}
	const total = (x: { direct: number; proxy: number }) => x.direct + x.proxy;
	const all = [...by.values()].sort((a, b) => total(b) - total(a) || a.source.localeCompare(b.source));
	return cut(all, top, total);
}
