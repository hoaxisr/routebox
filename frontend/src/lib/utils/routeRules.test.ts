import { describe, it, expect } from 'vitest';
import {
	referencedRuleSetTags,
	simpleRuleSetTag,
	mappingOutboundValue,
	applyMappingOutbound,
	reorderArray,
	ruleSetRowTag,
	REJECT_VALUE
} from './routeRules';
import type { RouteRule } from '$lib/types';

// The routes page used to draw plain rule-set mappings and full rules as two
// separate sections, each numbered from 1 and each draggable only within itself
// — so a full rule could never be put above a rule-set one even though both live
// in the same route.rules array. Classification now only decides how a row is
// drawn; order is global.
describe('simpleRuleSetTag', () => {
	it('recognises a bare rule_set -> outbound mapping', () => {
		expect(simpleRuleSetTag({ rule_set: ['ads'], outbound: 'direct' })).toBe('ads');
	});

	it('recognises a bare rule_set -> reject mapping', () => {
		expect(simpleRuleSetTag({ rule_set: ['ads'], action: 'reject' })).toBe('ads');
	});

	it('rejects a rule with no rule_set', () => {
		expect(simpleRuleSetTag({ domain_suffix: ['corp.local'], outbound: 'vpn' })).toBeNull();
	});

	// rule_set present but useless: an empty array once returned undefined, and a
	// JSON null once threw — both break the string|null contract callers rely on.
	it.each([[[]], [null], [undefined], [['']], ['not-an-array']])(
		'rejects a malformed rule_set %p',
		(rule_set) => {
			expect(simpleRuleSetTag({ rule_set, outbound: 'direct' } as never)).toBeNull();
		}
	);

	// Any field outside {rule_set, action, outbound} disqualifies — including
	// ones sing-box may add later, which a denylist would silently let through.
	it.each([
		['invert', { invert: true }],
		['a logical rule with nested rules', { type: 'logical', rules: [{ domain: ['x'] }] }],
		['clash_mode', { clash_mode: 'Direct' }],
		['ip_version', { ip_version: 4 }],
		['source_ip_is_private', { source_ip_is_private: true }],
		['source_port_range', { source_port_range: ['1000:2000'] }],
		['process_path_regex', { process_path_regex: ['^/usr'] }],
		['auth_user', { auth_user: ['alice'] }],
		['user', { user: ['alice'] }],
		['a field nobody has thought of yet', { some_future_condition: 'x' }]
	])('rejects a rule that also carries %s', (_name, extra) => {
		expect(simpleRuleSetTag({ rule_set: ['ads'], outbound: 'direct', ...extra } as never)).toBeNull();
	});

	it('rejects a rule referencing several rule-sets', () => {
		expect(simpleRuleSetTag({ rule_set: ['ads', 'trackers'], action: 'reject' })).toBeNull();
	});

	it.each([
		['domain', { domain: ['example.com'] }],
		['domain_suffix', { domain_suffix: ['example.com'] }],
		['domain_keyword', { domain_keyword: ['ads'] }],
		['domain_regex', { domain_regex: ['^ad'] }],
		['ip_cidr', { ip_cidr: ['10.0.0.0/8'] }],
		['source_ip_cidr', { source_ip_cidr: ['192.168.1.0/24'] }],
		['ip_is_private', { ip_is_private: true }],
		['protocol', { protocol: ['quic'] }],
		['port', { port: [443] }],
		['port_range', { port_range: ['1000:2000'] }],
		['source_port', { source_port: [53] }],
		['network', { network: 'udp' }],
		['inbound', { inbound: ['mixed-in'] }],
		['process_name', { process_name: ['curl'] }],
		['process_path', { process_path: ['/usr/bin/curl'] }]
	])('rejects a rule that also matches on %s', (_name, extra) => {
		expect(simpleRuleSetTag({ rule_set: ['ads'], outbound: 'direct', ...extra } as RouteRule)).toBeNull();
	});

	it('rejects rule-set rules whose action is not a destination', () => {
		expect(simpleRuleSetTag({ rule_set: ['ads'], action: 'sniff' })).toBeNull();
		expect(simpleRuleSetTag({ rule_set: ['ads'], action: 'hijack-dns' })).toBeNull();
	});

	// Empty arrays, nulls and false are how the API spells "condition absent";
	// treating any of them as present would hide a mapping from its rule-set row.
	it('ignores absent conditions in every spelling the API uses', () => {
		expect(simpleRuleSetTag({ rule_set: ['ads'], outbound: 'direct', domain: [], port: [] })).toBe('ads');
		expect(
			simpleRuleSetTag({
				rule_set: ['ads'],
				outbound: 'direct',
				ip_is_private: false,
				network: '',
				domain: null
			} as never)
		).toBe('ads');
	});
});

describe('reorderArray', () => {
	const abcd = ['A', 'B', 'C', 'D'];

	// `to` is where the item ENDS UP. The backend used to decrement it for
	// downward moves, so the saved order differed from the one on screen.
	it.each([
		[0, 1, 'BACD'],
		[0, 2, 'BCAD'],
		[0, 3, 'BCDA'],
		[1, 0, 'BACD'],
		[3, 0, 'DABC'],
		[1, 2, 'ACBD'],
		[2, 2, 'ABCD']
	])('moves %i to %i giving %s', (from, to, want) => {
		expect(reorderArray(abcd, from, to).join('')).toBe(want);
	});

	it('does not mutate the input', () => {
		const input = [...abcd];
		reorderArray(input, 0, 3);
		expect(input).toEqual(abcd);
	});

	it.each([
		[-1, 0],
		[0, -1],
		[4, 0],
		[0, 4]
	])('leaves the order alone for out-of-range (%i, %i)', (from, to) => {
		expect(reorderArray(abcd, from, to)).toEqual(abcd);
	});
});

describe('applyMappingOutbound', () => {
	it('switches a mapping to an outbound and back to reject', () => {
		const routed = applyMappingOutbound({ rule_set: ['ads'], action: 'reject' }, 'vpn');
		expect(routed).toEqual({ rule_set: ['ads'], action: 'route', outbound: 'vpn' });
		const rejected = applyMappingOutbound(routed, REJECT_VALUE);
		expect(rejected).toEqual({ rule_set: ['ads'], action: 'reject' });
	});

	// The invariant that matters: using the picker must not turn the row into an
	// ordinary rule, or the picker would vanish the moment it was touched.
	it.each(['direct', 'vpn-de', REJECT_VALUE])('keeps the row a plain mapping (%s)', (value) => {
		const rule: RouteRule = { rule_set: ['ads'], outbound: 'direct' };
		expect(simpleRuleSetTag(applyMappingOutbound(rule, value))).toBe('ads');
	});

	it('does not mutate the rule it is given', () => {
		const rule: RouteRule = { rule_set: ['ads'], outbound: 'direct' };
		applyMappingOutbound(rule, REJECT_VALUE);
		expect(rule).toEqual({ rule_set: ['ads'], outbound: 'direct' });
	});
});

describe('ruleSetRowTag', () => {
	const known = new Set(['ads']);

	it('draws a rule-set row for a known, plainly mapped tag', () => {
		expect(ruleSetRowTag({ rule_set: ['ads'], outbound: 'direct' }, known, true)).toBe('ads');
	});

	it('falls back to an ordinary row when the tag names no known rule-set', () => {
		expect(ruleSetRowTag({ rule_set: ['gone'], outbound: 'direct' }, known, true)).toBeNull();
	});

	it('falls back to an ordinary row when there is nothing to change it with', () => {
		expect(ruleSetRowTag({ rule_set: ['ads'], outbound: 'direct' }, known, false)).toBeNull();
	});

	it('falls back to an ordinary row for a rule that is not a plain mapping', () => {
		expect(ruleSetRowTag({ rule_set: ['ads'], invert: true } as never, known, true)).toBeNull();
	});
});

// "Has a route" is a question about the config, not about how the panel draws a
// row. The plain-mapping test above decides the row; this decides whether the
// rule-set belongs under "Rule sets with no route" — and getting that wrong put
// a live rule-set under a heading saying the opposite, with a delete button the
// backend refuses ("referenced by route rule[N]"). Issue #86.
describe('referencedRuleSetTags', () => {
	it('collects every tag any rule mentions, plain mapping or not', () => {
		const rules: RouteRule[] = [
			{ rule_set: ['ads'], action: 'reject' },
			{ rule_set: ['ru'], domain_suffix: ['ru'], outbound: 'direct' }, // not plain, still routed
			{ rule_set: ['a', 'b'], outbound: 'vpn' }, // two sets: not plain either
			{ domain: ['x.com'], outbound: 'vpn' },
			{ rule_set: ['streaming'], outbound: 'vpn-de' }
		];
		expect([...referencedRuleSetTags(rules)].sort()).toEqual([
			'a',
			'ads',
			'b',
			'ru',
			'streaming'
		]);
	});

	it('descends into a logical rule\'s nested rules', () => {
		const rules: RouteRule[] = [
			{
				type: 'logical',
				mode: 'and',
				rules: [{ rule_set: ['ads'] }, { rules: [{ rule_set: ['deep'] }] }],
				outbound: 'direct'
			} as RouteRule
		];
		expect([...referencedRuleSetTags(rules)].sort()).toEqual(['ads', 'deep']);
	});

	it('ignores empty and non-string entries', () => {
		const rules = [{ rule_set: ['', 7, 'ok'] }] as unknown as RouteRule[];
		expect([...referencedRuleSetTags(rules)]).toEqual(['ok']);
	});

	it('is empty for an empty rules array', () => {
		expect(referencedRuleSetTags([]).size).toBe(0);
	});
});

describe('mappingOutboundValue', () => {
	it('maps reject to the picker sentinel and route to the outbound tag', () => {
		expect(mappingOutboundValue({ rule_set: ['a'], action: 'reject' })).toBe('__reject__');
		expect(mappingOutboundValue({ rule_set: ['a'], outbound: 'vpn' })).toBe('vpn');
		expect(mappingOutboundValue({ rule_set: ['a'] })).toBe('');
	});
});
