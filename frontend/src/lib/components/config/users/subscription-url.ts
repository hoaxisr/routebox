// buildSubUrl constructs the public subscription URL. When host carries no
// scheme, https is forced (clients fetch the subscription over TLS). When the
// panel runs on a non-443 port (server.public_port), that port is appended —
// public_host is a bare host (port stripped server-side), so the panel port
// must be threaded separately or every sub-URL would silently hit :443=vless.
// The port is added only when meaningful (>0, not 443) and the host does not
// already carry an explicit port.
export function buildSubUrl(host: string, token: string, port?: number): string {
	host = host.trim().replace(/\s+/g, '');
	if (!host || !token) return '';
	let base = /^https?:\/\//i.test(host) ? host : `https://${host}`;
	base = base.replace(/\/+$/, '');
	if (port && port !== 443 && !hasExplicitPort(base)) {
		base = `${base}:${port}`;
	}
	return `${base}/sub/${encodeURIComponent(token)}`;
}

// hasExplicitPort reports whether the authority of a (possibly scheme-prefixed)
// URL already carries an explicit :port. IPv6 literals are bracketed
// ([::1]:8080) so a port is the ":NNNN" after "]"; for everything else a single
// trailing :digits. A bare (unbracketed) IPv6 literal has no explicit port.
function hasExplicitPort(base: string): boolean {
	const authority = base.replace(/^https?:\/\//i, '').split('/')[0];
	if (authority.startsWith('[')) {
		return /]:\d+$/.test(authority);
	}
	return /:\d+$/.test(authority);
}

// effectiveSubUrl is the central sticky-revoke gate: it returns '' when the
// user is revoked (token_disabled) or has no token; otherwise the sub URL.
export function effectiveSubUrl(
	user: { token?: string; token_disabled?: boolean },
	host: string,
	port?: number
): string {
	if (user.token_disabled) return '';
	if (!user.token) return '';
	return buildSubUrl(host, user.token, port);
}
