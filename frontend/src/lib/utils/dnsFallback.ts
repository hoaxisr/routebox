import type { DnsRule } from '$lib/types';

/**
 * Where the generated DNS-fallback tail starts, or rules.length when there is none.
 *
 * This MUST agree with findFallbackBlock in backend/internal/config/dns_fallback.go:
 * the page hides everything from this index on, and mirrors the backend's insert
 * position for a newly created rule. A looser match here means the page splices a
 * rule where the backend appends it, the two lists drift apart, and every later
 * edit — addressed by position — lands on somebody else's rule.
 *
 * The shape, at the very end of the list and nowhere else:
 *   {action: 'evaluate', server}
 *   {match_response: true, response_rcode, action: 'route', server}   × 1..n, one server
 *   {match_response: true, action: 'respond'}
 */
export const FALLBACK_RCODES = ['NXDOMAIN', 'SERVFAIL', 'REFUSED', 'NOTIMP', 'FORMERR'];

const keyCount = (r: DnsRule) => Object.keys(r).filter((k) => r[k as keyof DnsRule] !== undefined).length;

export function fallbackStart(rules: DnsRule[]): number {
	const none = rules.length;
	let i = rules.length - 1;
	if (i < 1) return none;

	const last = rules[i];
	if (keyCount(last) !== 2 || last.match_response !== true || last.action !== 'respond') return none;

	let fallback = '';
	const codes: string[] = [];
	for (i--; i >= 0; i--) {
		const r = rules[i];
		if (
			keyCount(r) !== 4 ||
			r.match_response !== true ||
			r.action !== 'route' ||
			!r.response_rcode ||
			!r.server
		) {
			break;
		}
		if (!FALLBACK_RCODES.includes(r.response_rcode)) return none;
		if (fallback && r.server !== fallback) return none;
		fallback = r.server;
		codes.unshift(r.response_rcode);
	}
	if (codes.length === 0 || i < 0) return none;

	const head = rules[i];
	if (keyCount(head) !== 2 || head.action !== 'evaluate' || !head.server || head.server === fallback) {
		return none;
	}
	return i;
}
