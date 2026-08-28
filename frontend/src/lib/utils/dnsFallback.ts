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
 *   {action: 'evaluate', server}                                                   -- the primary
 *   {match_response: true, response_rcode, action: 'evaluate', server}  × codes    -- per fallback but the last
 *   {match_response: true, response_rcode, action: 'route',    server}  × codes    -- the last fallback
 *   {match_response: true, action: 'respond'}
 *
 * A tail written before #70 has no evaluate hops at all; it is the one-fallback
 * case of the same shape.
 */
export const FALLBACK_RCODES = ['NXDOMAIN', 'SERVFAIL', 'REFUSED', 'NOTIMP', 'FORMERR'];

const keyCount = (r: DnsRule) => Object.keys(r).filter((k) => r[k as keyof DnsRule] !== undefined).length;

export function fallbackStart(rules: DnsRule[]): number {
	const none = rules.length;
	let i = rules.length - 1;
	if (i < 1) return none;

	const last = rules[i];
	if (keyCount(last) !== 2 || last.match_response !== true || last.action !== 'respond') return none;
	i--;

	// One hop: the run of per-rcode rules with `action`, all naming one server.
	// Returns null when the rule at i does not end such a run, and 'bad' when it
	// ends one this panel could not have written.
	const hop = (action: string): { server: string; codes: string[] } | null | 'bad' => {
		let server = '';
		const codes: string[] = [];
		for (; i >= 0; i--) {
			const r = rules[i];
			if (
				keyCount(r) !== 4 ||
				r.match_response !== true ||
				r.action !== action ||
				!r.response_rcode ||
				!r.server
			) {
				break;
			}
			if (!FALLBACK_RCODES.includes(r.response_rcode)) return 'bad';
			if (server && r.server !== server) break; // previous hop; leave i on it
			server = r.server;
			codes.unshift(r.response_rcode);
		}
		return codes.length > 0 ? { server, codes } : null;
	};

	const terminal = hop('route');
	if (terminal === null || terminal === 'bad') return none;

	const servers = [terminal.server];
	for (;;) {
		const step = hop('evaluate');
		if (step === 'bad') return none;
		if (step === null) break;
		if (step.codes.join() !== terminal.codes.join()) return none;
		servers.unshift(step.server);
	}
	if (i < 0) return none;

	const head = rules[i];
	if (keyCount(head) !== 2 || head.action !== 'evaluate' || !head.server) return none;
	// A server twice in the chain is a loop the panel never writes.
	const seen = new Set([head.server]);
	for (const s of servers) {
		if (seen.has(s)) return none;
		seen.add(s);
	}
	return i;
}
