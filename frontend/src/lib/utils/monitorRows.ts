import type { AwgPeerTraffic, UserTrafficPoint } from '$lib/types';

/** One line of the per-user monitor: a panel user or an AWG peer (#40). */
export type MonitorRow = {
	id: string;
	name: string;
	upload: number;
	download: number;
	total: number;
	series: UserTrafficPoint[];
	active: boolean;
	// Panel users are counted by sing-box per-user stats, peers by their tunnel
	// IP — same presentation, different accounting source.
	kind: 'user' | 'peer';
};

/**
 * AWG peers as monitor rows. Ids are namespaced so a peer can never collide with
 * a panel user in the row key or the expand map, and liveness comes from the
 * peer's real handshake instead of being guessed from recent buckets. A peer
 * that was never named falls back to its tunnel IP.
 */
export function peersToRows(peers: AwgPeerTraffic[]): MonitorRow[] {
	return peers.map((p) => ({
		id: `peer:${p.public_key}`,
		name: p.name || p.source,
		upload: p.upload,
		download: p.download,
		total: p.upload + p.download,
		series: p.history ?? [],
		active: p.online,
		kind: 'peer' as const
	}));
}

/** Users and peers in one list, heaviest first — what the page renders. */
export function mergeMonitorRows(users: MonitorRow[], peers: MonitorRow[]): MonitorRow[] {
	return [...users, ...peers].sort((a, b) => b.total - a.total);
}
