import type { Outbound } from '$lib/types';

/** Outbound types that pick a member at dial time rather than dialling themselves. */
const GROUP_TYPES = ['selector', 'urltest'];

export function groupTags(outbounds: Outbound[]): Set<string> {
	return new Set(outbounds.filter((o) => GROUP_TYPES.includes(o.type)).map((o) => o.tag));
}

/**
 * Whether a connection chain stops at a group — that is, it says the traffic
 * went through one but not which member actually carried it.
 *
 * sing-box walks the chain from the matched outbound down through each group's
 * CURRENT pick and reverses it, so element 0 is the DEEPEST hop. A urltest
 * reports no pick until its first health check succeeds (`URLTest.Now()` returns
 * ""), and the walk stops there — leaving a group as the deepest element. That
 * is the whole test: a deepest hop that is a group means the walk stopped inside
 * one. Checking the chain's LENGTH instead would miss a selector over an
 * unprobed urltest, which yields ["auto", "manual"] — two hops, both groups,
 * carrier still unknown.
 *
 * The window is not rare: it opens on every start and every Apply, and lasts as
 * long as the probe takes. The chain is captured when the connection is created
 * and never revised, so those rows keep the label for good, including in the
 * traffic history (#79). Verified against amnezia-box 1.14.0-rc.1-awgm.14.
 *
 * Accepts either the array (live connections) or the " → "-joined string the
 * traffic history stores. Tags are operator-chosen and may themselves contain
 * that separator, so the string form is matched whole before it is split.
 *
 * KNOWN LIMIT: `groups` comes from /api/outbounds, which serves the DRAFT
 * config. While unapplied changes retype an existing tag, a row can be marked
 * (or missed) against a shape that is not the one the connection was dialled
 * through. Advisory marking, so this is left rather than given its own endpoint.
 */
export function isGroupOnlyChain(chain: string[] | string, groups: Set<string>): boolean {
	if (Array.isArray(chain)) {
		return chain.length > 0 && groups.has(chain[0]);
	}
	if (chain === '') return false;
	// Whole first: a tag containing " → " would not survive the split.
	if (groups.has(chain)) return true;
	return groups.has(chain.split(' → ')[0]);
}
