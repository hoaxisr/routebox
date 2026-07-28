import type { RouteRule } from '$lib/types';

// Conditions that make a rule more than a plain "this rule-set goes there"
// mapping. Any of them present and the rule is a full routing rule, edited
// through the rule form rather than a one-click outbound switch.
const EXTRA_CONDITIONS = [
	'domain',
	'domain_suffix',
	'domain_keyword',
	'domain_regex',
	'ip_cidr',
	'source_ip_cidr',
	'ip_is_private',
	'protocol',
	'port',
	'port_range',
	'source_port',
	'network',
	'inbound',
	'process_name',
	'process_path'
] as const;

/**
 * The rule-set tag a rule is a plain mapping for, or null.
 *
 * "Plain" means exactly one rule_set, no other match conditions, and an action
 * that just picks a destination (route/reject) — those are the rules the panel
 * can render as a rule-set row with an inline outbound picker. Everything else
 * is a full rule.
 *
 * Both kinds live in the SAME route.rules array and their order across the whole
 * array is what sing-box evaluates, so this only decides how a row is DRAWN,
 * never where it sits.
 */
export function simpleRuleSetTag(rule: RouteRule): string | null {
	const tags = rule.rule_set;
	if (!tags || tags.length !== 1) return null;
	const action = rule.action ?? 'route';
	if (action !== 'route' && action !== 'reject') return null;
	for (const key of EXTRA_CONDITIONS) {
		const v = (rule as Record<string, unknown>)[key];
		if (Array.isArray(v) ? v.length > 0 : v !== undefined && v !== null && v !== false && v !== '') {
			return null;
		}
	}
	return tags[0];
}

/** Tags that already have a plain mapping somewhere in the rules array. */
export function assignedRuleSetTags(rules: RouteRule[]): Set<string> {
	const out = new Set<string>();
	for (const rule of rules) {
		const tag = simpleRuleSetTag(rule);
		if (tag) out.add(tag);
	}
	return out;
}

/** The outbound a plain mapping points at, for the inline picker's value. */
export function mappingOutboundValue(rule: RouteRule): string {
	return (rule.action ?? 'route') === 'reject' ? '__reject__' : (rule.outbound ?? '');
}
