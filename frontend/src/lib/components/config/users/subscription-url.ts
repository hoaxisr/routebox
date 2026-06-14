// buildSubUrl constructs the public subscription URL. When host carries no
// scheme, https is forced (clients fetch the subscription over TLS).
export function buildSubUrl(host: string, token: string): string {
	host = host.trim().replace(/\s+/g, '');
	if (!host || !token) return '';
	const base = /^https?:\/\//i.test(host) ? host : `https://${host}`;
	return `${base.replace(/\/+$/, '')}/sub/${encodeURIComponent(token)}`;
}

// effectiveSubUrl is the central sticky-revoke gate: it returns '' when the
// user is revoked (token_disabled) or has no token; otherwise the sub URL.
export function effectiveSubUrl(
	user: { token?: string; token_disabled?: boolean },
	host: string
): string {
	if (user.token_disabled) return '';
	if (!user.token) return '';
	return buildSubUrl(host, user.token);
}
