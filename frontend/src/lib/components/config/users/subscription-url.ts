// buildSubUrl constructs the public subscription URL. When host carries no
// scheme, https is forced (clients fetch the subscription over TLS).
export function buildSubUrl(host: string, token: string): string {
	if (!host || !token) return '';
	const base = /^https?:\/\//i.test(host) ? host : `https://${host}`;
	return `${base.replace(/\/+$/, '')}/sub/${encodeURIComponent(token)}`;
}
