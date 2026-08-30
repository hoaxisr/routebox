import type { RouteRule } from '$lib/types';

// The ONLY fields a plain mapping may carry. An allowlist, deliberately: the
// denylist this replaced had to enumerate every match condition, and the ones it
// missed were drawn as friendly rule-set rows that hid what the rule really did
// — `invert` most dangerously, since the row then states the exact opposite
// ("ads -> REJECT" for a rule that rejects everything EXCEPT ads). A logical
// rule with nested `rules` hid the same way. With an allowlist, any field the
// list does not know — including one added to sing-box tomorrow — disqualifies
// the rule and it renders in full.
const MAPPING_FIELDS = new Set(['rule_set', 'action', 'outbound']);

// Values that mean "this field is not set". The API round-trips conditions as
// empty arrays and JSON nulls, and treating those as present would strip the
// friendly row off a mapping that has no extra conditions at all.
function isAbsent(v: unknown): boolean {
	if (v === undefined || v === null || v === '' || v === false) return true;
	return Array.isArray(v) && v.length === 0;
}

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
	const tags = rule?.rule_set;
	if (!Array.isArray(tags) || tags.length !== 1) return null;
	const tag = tags[0];
	if (typeof tag !== 'string' || tag === '') return null;
	const action = rule.action ?? 'route';
	if (action !== 'route' && action !== 'reject') return null;
	for (const [key, value] of Object.entries(rule)) {
		if (!MAPPING_FIELDS.has(key) && !isAbsent(value)) return null;
	}
	return tag;
}

/**
 * The array with the item at `from` moved to index `to`, where `to` is the
 * index it ENDS UP at — the same contract the backend's ReorderRules follows.
 * The two used to disagree for downward moves, so the list on screen and the
 * config on disk drifted apart and every later index-addressed edit hit the
 * wrong rule. Out-of-range input returns the array unchanged.
 */
export function reorderArray<T>(items: T[], from: number, to: number): T[] {
	if (from < 0 || from >= items.length || to < 0 || to >= items.length || from === to) {
		return items.slice();
	}
	const out = items.slice();
	const [moved] = out.splice(from, 1);
	out.splice(to, 0, moved);
	return out;
}

/**
 * The rule-set a row should be drawn as, or null for an ordinary rule row. A
 * mapping whose tag names no known rule-set gets no picker — there would be
 * nothing to show in it — and neither does anything when the caller wired up no
 * change handler.
 */
export function ruleSetRowTag(
	rule: RouteRule,
	knownTags: Set<string>,
	hasChangeHandler: boolean
): string | null {
	if (!hasChangeHandler) return null;
	const tag = simpleRuleSetTag(rule);
	return tag && knownTags.has(tag) ? tag : null;
}

/**
 * Every rule-set tag ANY rule mentions, however the rule is shaped.
 *
 * "Has a route" is a question about the config, not about how the panel chose to
 * draw a row. A rule-set named by a rule with a second condition on it — or by
 * one of two, or from inside a logical rule's nested `rules` — is not a plain
 * mapping, so simpleRuleSetTag says nothing about it; listing it as having no
 * route put a live rule-set under a heading that says the opposite, and offered
 * a delete the backend refuses ("referenced by route rule[N]").
 */
export function referencedRuleSetTags(rules: RouteRule[]): Set<string> {
	const out = new Set<string>();
	const walk = (list: RouteRule[] | undefined) => {
		if (!Array.isArray(list)) return;
		for (const rule of list) {
			if (!rule || typeof rule !== 'object') continue;
			if (Array.isArray(rule.rule_set)) {
				for (const tag of rule.rule_set) {
					if (typeof tag === 'string' && tag !== '') out.add(tag);
				}
			}
			// Logical rules carry their operands in `rules`, and a rule-set named
			// only in there is just as referenced.
			walk((rule as { rules?: RouteRule[] }).rules);
		}
	};
	walk(rules);
	return out;
}

/** The outbound a plain mapping points at, for the inline picker's value. */
export function mappingOutboundValue(rule: RouteRule): string {
	return (rule.action ?? 'route') === 'reject' ? '__reject__' : (rule.outbound ?? '');
}

/** The picker's sentinel for "block this instead of routing it". */
export const REJECT_VALUE = '__reject__';

/**
 * A rule with the picker's chosen destination applied. Kept pure and shared by
 * the inline switch and by assigning a rule-set that had no route, so both
 * produce the same shape — and so the invariant that matters can be tested: a
 * plain mapping must STAY one, or the row would lose its picker the moment it
 * was used.
 */
export function applyMappingOutbound(rule: RouteRule, value: string): RouteRule {
	const next: RouteRule = { ...rule };
	if (value === REJECT_VALUE) {
		next.action = 'reject';
		delete next.outbound;
	} else {
		next.action = 'route';
		next.outbound = value;
	}
	return next;
}
