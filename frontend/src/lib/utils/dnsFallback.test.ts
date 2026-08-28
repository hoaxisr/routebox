import { describe, it, expect } from 'vitest';
import { fallbackStart } from './dnsFallback';
import type { DnsRule } from '$lib/types';

const tail: DnsRule[] = [
	{ action: 'evaluate', server: 'primary' },
	{ match_response: true, response_rcode: 'NXDOMAIN', action: 'route', server: 'backup' },
	{ match_response: true, response_rcode: 'SERVFAIL', action: 'route', server: 'backup' },
	{ match_response: true, action: 'respond' }
];
const plain: DnsRule = { domain: ['example.com'], server: 'primary' };

describe('fallbackStart', () => {
	it('finds the generated tail after the operator rules', () => {
		expect(fallbackStart([plain, plain, ...tail])).toBe(2);
	});

	it('reports no tail for an empty or plain rule list', () => {
		expect(fallbackStart([])).toBe(0);
		expect(fallbackStart([plain, plain])).toBe(2);
	});

	// Each of these makes the BACKEND report no block, so it appends a new rule at
	// the very end. Matching them here would splice the rule somewhere else, and
	// from then on every position-addressed edit would hit the wrong rule.
	it.each([
		['respond alone', [{ match_response: true, action: 'respond' } as DnsRule]],
		['no evaluate ahead of the route rules', tail.slice(1)],
		['evaluate in the middle, not a tail', [{ action: 'evaluate', server: 'primary' } as DnsRule, plain]],
		['an extra key on the respond', [...tail.slice(0, -1), { match_response: true, action: 'respond', server: 'backup' } as DnsRule]],
		[
			'route rules split across two servers',
			[
				tail[0],
				tail[1],
				{ match_response: true, response_rcode: 'SERVFAIL', action: 'route', server: 'other' } as DnsRule,
				tail[3]
			]
		],
		[
			'an rcode the panel cannot write back',
			[tail[0], { match_response: true, response_rcode: 'NOERROR', action: 'route', server: 'backup' } as DnsRule, tail[3]]
		],
		[
			'primary and fallback are the same server',
			[{ action: 'evaluate', server: 'backup' } as DnsRule, tail[1], tail[3]]
		]
	])('does not claim a tail: %s', (_name, rules) => {
		expect(fallbackStart(rules as DnsRule[])).toBe((rules as DnsRule[]).length);
	});

	// #70: a chain longer than one hop — the extra fallbacks are `evaluate` rules
	// between the head and the terminal `route`. The whole block must still hide as
	// one unit, or the page splices the operator's next rule into the middle of it.
	it('finds the start of a multi-hop chain', () => {
		const rules: DnsRule[] = [
			{ domain: ['example.com'], server: 'primary' },
			{ action: 'evaluate', server: 'primary' },
			{ match_response: true, response_rcode: 'NXDOMAIN', action: 'evaluate', server: 'fb1' },
			{ match_response: true, response_rcode: 'SERVFAIL', action: 'evaluate', server: 'fb1' },
			{ match_response: true, response_rcode: 'NXDOMAIN', action: 'route', server: 'fb2' },
			{ match_response: true, response_rcode: 'SERVFAIL', action: 'route', server: 'fb2' },
			{ match_response: true, action: 'respond' }
		];
		expect(fallbackStart(rules)).toBe(1);
	});

	it('refuses a chain that asks one server twice', () => {
		const rules: DnsRule[] = [
			{ action: 'evaluate', server: 'primary' },
			{ match_response: true, response_rcode: 'NXDOMAIN', action: 'evaluate', server: 'fb1' },
			{ match_response: true, response_rcode: 'NXDOMAIN', action: 'route', server: 'fb1' },
			{ match_response: true, action: 'respond' }
		];
		expect(fallbackStart(rules)).toBe(rules.length);
	});

	it('refuses a hop whose codes differ from the terminal hop', () => {
		const rules: DnsRule[] = [
			{ action: 'evaluate', server: 'primary' },
			{ match_response: true, response_rcode: 'REFUSED', action: 'evaluate', server: 'fb1' },
			{ match_response: true, response_rcode: 'NXDOMAIN', action: 'route', server: 'fb2' },
			{ match_response: true, action: 'respond' }
		];
		expect(fallbackStart(rules)).toBe(rules.length);
	});

	// dns.rules can carry a literal null. The backend reads one as "not a rule I
	// own"; throwing here instead would take the whole DNS page down over a rule
	// this parser does not even want.
	it('survives a null element in the rule list', () => {
		expect(() => fallbackStart([null as unknown as DnsRule])).not.toThrow();
		expect(fallbackStart([null as unknown as DnsRule])).toBe(1);
		const rules = [
			null as unknown as DnsRule,
			{ action: 'evaluate', server: 'primary' },
			{ match_response: true, response_rcode: 'NXDOMAIN', action: 'route', server: 'fb' },
			{ match_response: true, action: 'respond' }
		] as DnsRule[];
		expect(fallbackStart(rules)).toBe(1);
	});
});
