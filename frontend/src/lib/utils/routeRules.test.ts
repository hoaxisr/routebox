import { describe, it, expect } from 'vitest';
import { simpleRuleSetTag, assignedRuleSetTags, mappingOutboundValue } from './routeRules';
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

	// Empty arrays are how the API returns "condition absent" for some fields;
	// treating them as present would hide a mapping from its rule-set row.
	it('ignores empty condition arrays', () => {
		expect(simpleRuleSetTag({ rule_set: ['ads'], outbound: 'direct', domain: [], port: [] })).toBe('ads');
	});
});

describe('assignedRuleSetTags', () => {
	it('collects only the plainly mapped tags', () => {
		const rules: RouteRule[] = [
			{ rule_set: ['ads'], action: 'reject' },
			{ rule_set: ['ru'], domain_suffix: ['ru'], outbound: 'direct' }, // not plain
			{ domain: ['x.com'], outbound: 'vpn' },
			{ rule_set: ['streaming'], outbound: 'vpn-de' }
		];
		expect([...assignedRuleSetTags(rules)].sort()).toEqual(['ads', 'streaming']);
	});

	it('is empty for an empty rules array', () => {
		expect(assignedRuleSetTags([]).size).toBe(0);
	});
});

describe('mappingOutboundValue', () => {
	it('maps reject to the picker sentinel and route to the outbound tag', () => {
		expect(mappingOutboundValue({ rule_set: ['a'], action: 'reject' })).toBe('__reject__');
		expect(mappingOutboundValue({ rule_set: ['a'], outbound: 'vpn' })).toBe('vpn');
		expect(mappingOutboundValue({ rule_set: ['a'] })).toBe('');
	});
});
