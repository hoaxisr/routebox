import type { Subscription } from '$lib/types';

/**
 * The separator between a subscription's group tag and a node's own name. Must
 * match tagSeparator in backend/internal/subscriptions/fetcher.go, which is what
 * actually writes the tags.
 */
export const SUB_TAG_SEPARATOR = ' · ';

/**
 * Group tags are the SANITIZED subscription name, so "Подписка NL" owns
 * outbounds tagged "NL" and "NL · Node". Mirrors Sanitize() in
 * backend/internal/subscriptions/fetcher.go.
 */
export function sanitizeSubscriptionName(name: string): string {
	return [...name].filter((c) => /[a-zA-Z0-9\-_ ]/.test(c)).join('').trim();
}

/**
 * Which subscription owns an outbound tag, or null for one the operator wrote.
 *
 * A subscription rewrites its outbounds wholesale on every refresh, so deleting
 * one by hand only lasts until the next one — and nothing in the outbound list
 * said so, which is what made deleting them look permanent (#80). Ownership is
 * derived from the tag because that is all the config records: sing-box takes no
 * marker key, and an unknown key is a decode error.
 */
export function owningSubscription(tag: string, subs: Subscription[]): Subscription | null {
	for (const sub of subs) {
		const group = sanitizeSubscriptionName(sub.name);
		if (!group) continue;
		if (tag === group || tag.startsWith(group + SUB_TAG_SEPARATOR)) return sub;
	}
	return null;
}
