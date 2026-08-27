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
	publicHost: string,
	frontPort = 0
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
	// An inbound bound to the loopback while a front is configured is not local:
	// it is one standing behind the front, which is how the panel recognises it
	// (server.front_port, serverlinks). Its own port is unreachable by
	// definition, so showing it tells the operator nothing they can dial or hand
	// to anyone — clients reach it at the front's address.
	if (frontPort > 0 && publicHost && listen !== undefined && isLoopbackListen(listen)) {
		const host = publicHost.includes(':') && !publicHost.startsWith('[')
			? `[${publicHost}]`
			: publicHost;
		return `${host}:${frontPort}`;
	}
	return `${listen ?? '127.0.0.1'}:${port}`;
}

// Loopback binds, mirroring the backend's listensOnLoopback (serverlinks).
function isLoopbackListen(listen: string): boolean {
	const v = listen.trim().replace(/^\[|\]$/g, '');
	return v === '127.0.0.1' || v === '::1' || v.startsWith('127.');
}
