import type { AwgPeerTraffic, MtprotoClientTraffic, UserTrafficPoint } from '$lib/types';

/** One line of the per-user monitor: a panel user, an AWG peer (#40), or a
 *  Telegram proxy client. */
export type MonitorRow = {
	id: string;
	name: string;
	upload: number;
	download: number;
	total: number;
	series: UserTrafficPoint[];
	active: boolean;
	// Same presentation, three accounting sources: panel users come from
	// sing-box per-user stats, peers from their tunnel IP, and Telegram clients
	// from the proxy's own relay.
	kind: 'user' | 'peer' | 'mtproto';
};

/**
 * AWG peers as monitor rows. Ids are namespaced so a peer can never collide with
 * a panel user in the row key or the expand map, and liveness is whatever the
 * server computed — a real handshake either way, off the kernel interface on
 * that backend and off sing-box's WireGuard device via its UAPI on the other.
 * A peer that was never named falls back to its tunnel IP.
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

/**
 * Telegram proxy clients as monitor rows. Ids are namespaced the same way peers
 * are, so a client can never collide with a panel user of the same name — which
 * is the same reason the backend stores their bytes under an mtproto: prefix.
 *
 * Liveness is inferred from recent buckets rather than read from a flag: this
 * endpoint reports traffic, and whether a client is connected right now is the
 * Telegram page's business.
 */
export function mtprotoClientsToRows(clients: MtprotoClientTraffic[]): MonitorRow[] {
	const cutoff = Math.floor(Date.now() / 1000) - 600;

	return clients.map((c) => {
		const series = c.history ?? [];

		return {
			id: `mtproto:${c.name}`,
			name: c.name,
			upload: c.upload,
			download: c.download,
			total: c.upload + c.download,
			series,
			active: series.some((p) => p.ts >= cutoff && p.upload + p.download > 0),
			kind: 'mtproto' as const
		};
	});
}

/** Every source in one list, heaviest first — what the page renders. */
export function mergeMonitorRows(
	users: MonitorRow[],
	peers: MonitorRow[],
	mtproto: MonitorRow[] = []
): MonitorRow[] {
	return [...users, ...peers, ...mtproto].sort((a, b) => b.total - a.total);
}
