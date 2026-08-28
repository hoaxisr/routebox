import type { ConnectionsResponse } from '$lib/types';

/**
 * Collapses an IPv4-mapped IPv6 address ("::ffff:203.0.113.7") to the plain IPv4
 * it stands for, and leaves everything else alone.
 *
 * Inbounds bind a dual-stack socket, so sing-box reports an IPv4 client through
 * the Clash API in the mapped form. Left as-is it is a DIFFERENT string from the
 * same client's plain address: the connections monitor groups one device under
 * two headings and its saved name never matches (#71). Mirrors
 * util.CanonicalClientIP on the backend, which does the same to the client list
 * and the traffic history.
 */
export function canonicalClientIp(ip: string): string {
	const m = /^::ffff:(\d{1,3}(?:\.\d{1,3}){3})$/i.exec(ip);
	if (!m) return ip;
	return m[1].split('.').every((o) => Number(o) <= 255) ? m[1] : ip;
}

/**
 * The single place connection data is normalised on the way in, so no consumer
 * downstream — grouping key, name lookup, search, breakdown — has to remember to.
 */
export function canonicalizeConnections(resp: ConnectionsResponse): ConnectionsResponse {
	if (!resp?.connections) return resp;
	return {
		...resp,
		connections: resp.connections.map((c) =>
			c.metadata?.sourceIP
				? { ...c, metadata: { ...c.metadata, sourceIP: canonicalClientIp(c.metadata.sourceIP) } }
				: c
		)
	};
}
