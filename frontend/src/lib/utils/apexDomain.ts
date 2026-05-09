import { getDomain, parse } from 'tldts';

// Collapse a host to its effective TLD+1 (apex domain).
// IPs, the "-" placeholder, empty strings, and unparseable inputs are
// returned unchanged so callers don't need to special-case them.
export function apexDomain(host: string): string {
	if (!host || host === '-') return host;
	const info = parse(host, { detectIp: true });
	if (info.isIp) return host;
	const apex = getDomain(host);
	return apex ?? host;
}
