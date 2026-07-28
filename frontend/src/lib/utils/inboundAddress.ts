// Wildcard bind addresses, mirroring the backend's normalizeListenAddr
// (backend/internal/config/inbounds.go). An ABSENT listen is deliberately not
// here: sing-box defaults it to 127.0.0.1 (listener_tcp.go builds the bind addr
// with a 127.0.0.1 fallback), so it is a local inbound, not a server one, and
// claiming it is reachable from outside would be the dangerous direction to be
// wrong in. An explicitly empty string is a wildcard spelling, not an absence.
const WILDCARDS = new Set(['', '::', '[::]', '::0', '0.0.0.0', '*']);

/**
 * The address to SHOW for an inbound. A server inbound binds the wildcard, so
 * its listen value ("::") is the one address no client can dial — substitute the
 * public host, which is what clients actually connect to (#37). A specific bind
 * address is deliberate and shown as-is.
 */
export function inboundDisplayAddress(
	inbound: { listen?: string; listen_port?: number },
	publicHost: string
): string {
	const port = inbound.listen_port ?? 1080;
	const listen = inbound.listen;
	if (listen !== undefined && WILDCARDS.has(listen)) {
		if (!publicHost) return `*:${port}`;
		// Bracket a bare IPv6 literal so the result stays an unambiguous host:port.
		const host = publicHost.includes(':') && !publicHost.startsWith('[')
			? `[${publicHost}]`
			: publicHost;
		return `${host}:${port}`;
	}
	return `${listen ?? '127.0.0.1'}:${port}`;
}
