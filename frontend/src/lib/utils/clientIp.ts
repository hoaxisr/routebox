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

/**
 * Whether an address can be a device of this box: the LAN, a tunnel, or the box
 * itself. Mirrors util.IsLocalClientIP on the backend, which decides the same
 * for the traffic history and the client roster (#102) — the live Breakdown
 * reads the Clash stream directly, so without this the page contradicted its own
 * historical ranges: a Google front-end that vanishes when you click "1h".
 *
 * IPv6 goes the other way on purpose, exactly as the backend does: a client's
 * IPv6 address is globally routable by design, so the address alone cannot tell
 * a device from a stranger and refusing one would hide a real device.
 *
 * One difference from the backend, which has the settings at hand: a tunnel
 * subnet configured outside the private ranges is not known here, so such a peer
 * is missing from the live view until the range switcher is used. The AWG
 * default is 10.10.0.0/24.
 */
export function isLocalClientIp(ip: string): boolean {
	const addr = canonicalClientIp(ip);
	const v4 = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(addr);
	if (!v4) return addr.includes(':') && !/^(ff|::$)/i.test(addr);
	const [a, b] = v4.slice(1).map(Number);
	if (v4.slice(1).some((o) => Number(o) > 255)) return false;
	return (
		a === 10 ||
		a === 127 ||
		(a === 192 && b === 168) ||
		(a === 172 && b >= 16 && b <= 31) ||
		(a === 169 && b === 254) ||
		(a === 100 && b >= 64 && b <= 127) // RFC 6598
	);
}

/**
 * Drops connections whose source is not a device of this box, keeping the ones
 * with no source at all — those are the box's own dials, shown as "unknown".
 */
export function localSourceConnections<T extends { metadata?: { sourceIP?: string } }>(
	conns: T[]
): T[] {
	return conns.filter((c) => !c.metadata?.sourceIP || isLocalClientIp(c.metadata.sourceIP));
}
