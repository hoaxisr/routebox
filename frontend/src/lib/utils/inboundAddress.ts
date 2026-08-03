// Wildcard bind addresses, mirroring the backend's normalizeListenAddr
// (backend/internal/config/inbounds.go). An ABSENT listen is deliberately not
// here: sing-box defaults it to 127.0.0.1 (listener_tcp.go builds the bind addr
// with a 127.0.0.1 fallback), so it is a local inbound, not a server one, and
// claiming it is reachable from outside would be the dangerous direction to be
// wrong in. An explicitly empty string is a wildcard spelling, not an absence.
const WILDCARDS = new Set(['', '::', '[::]', '::0', '0.0.0.0', '*']);

/**
 * Whether a listen VALUE (present, possibly padded — e.g. a form field) is a
 * wildcard bind. An absent listen is a different thing entirely (see above);
 * callers with `listen?: string` must handle undefined themselves.
 */
export function isWildcardListen(listen: string): boolean {
	return WILDCARDS.has(listen.trim());
}

/**
 * The address to SHOW for an inbound. A server inbound binds the wildcard, so
 * its listen value ("::") is the one address no client can dial — substitute the
 * public host, which is what clients actually connect to (#37). A specific bind
 * address is deliberate and shown as-is.
 */
export function inboundDisplayAddress(
	inbound: { listen?: string; listen_port?: number; listen_ports?: string[] },
	publicHost: string
): string {
	// A mieru inbound may bind ONLY ranges (listen_ports, no listen_port); showing
	// the 1080 fallback there invented a port nothing listens on (#46). Ranges are
	// listed alongside the single port when both are set.
	const ranges = inbound.listen_ports?.filter(Boolean) ?? [];
	const specs = [
		...(inbound.listen_port !== undefined ? [String(inbound.listen_port)] : []),
		...ranges
	];
	const port = specs.length > 0 ? specs.join(',') : 1080;
	const listen = inbound.listen;
	if (listen !== undefined && isWildcardListen(listen)) {
		if (!publicHost) return `*:${port}`;
		// Bracket a bare IPv6 literal so the result stays an unambiguous host:port.
		const host = publicHost.includes(':') && !publicHost.startsWith('[')
			? `[${publicHost}]`
			: publicHost;
		return `${host}:${port}`;
	}
	return `${listen ?? '127.0.0.1'}:${port}`;
}
