import type { Outbound } from '$lib/types';

/** Outbound types that pick a member at dial time rather than dialling themselves. */
const GROUP_TYPES = ['selector', 'urltest'];

export function groupTags(outbounds: Outbound[]): Set<string> {
	return new Set(outbounds.filter((o) => GROUP_TYPES.includes(o.type)).map((o) => o.tag));
}

/**
 * Whether a connection chain names a group and nothing else — which says the
 * traffic went through that group but not which member carried it.
 *
 * sing-box walks the chain from the matched outbound down through each group's
 * CURRENT pick, and a urltest reports no pick until its first health check
 * succeeds (`URLTest.Now()` returns ""). The walk stops there, so every
 * connection opened in that window — after a start, after Apply, and for as long
 * as the probe takes to come back — is recorded as the bare group name. The
 * chain is captured when the connection is created and never revised, so those
 * rows keep that label for good, including in the traffic history (#79).
 *
 * Verified against amnezia-box 1.14.0-rc.1-awgm.14: with an unreachable probe URL
 * a live connection reports chains ["auto"]; once the group has picked, the same
 * request reports ["d1", "auto"].
 *
 * Accepts either the array (live connections) or the " → "-joined string the
 * traffic history stores.
 */
export function isGroupOnlyChain(chain: string[] | string, groups: Set<string>): boolean {
	const parts = Array.isArray(chain) ? chain : chain.split(' → ');
	return parts.length === 1 && parts[0] !== '' && groups.has(parts[0]);
}
