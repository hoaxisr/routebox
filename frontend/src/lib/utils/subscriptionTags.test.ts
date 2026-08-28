import { describe, it, expect } from 'vitest';
import type { Subscription } from '$lib/types';
import { owningSubscription, sanitizeSubscriptionName } from './subscriptionTags';

const sub = (name: string): Subscription =>
	({ id: name.toLowerCase(), name, url: 'http://x', interval_hrs: 12, node_count: 2 }) as Subscription;

describe('sanitizeSubscriptionName', () => {
	// Mirrors Sanitize() in the Go fetcher: ASCII letters, digits, - _ and space.
	it('keeps what the backend keeps and drops what it drops', () => {
		expect(sanitizeSubscriptionName('MySub')).toBe('MySub');
		expect(sanitizeSubscriptionName('Подписка NL')).toBe('NL');
		expect(sanitizeSubscriptionName('  spaced  ')).toBe('spaced');
		expect(sanitizeSubscriptionName('a-b_c 1')).toBe('a-b_c 1');
		expect(sanitizeSubscriptionName('плюс+знаки!')).toBe('');
	});
});

describe('owningSubscription', () => {
	const subs = [sub('MySub'), sub('Подписка NL')];

	it('claims the group tag and every node under it', () => {
		expect(owningSubscription('MySub', subs)?.name).toBe('MySub');
		expect(owningSubscription('MySub · NL-1', subs)?.name).toBe('MySub');
		// the tag comes from the sanitized name, not the name the operator sees
		expect(owningSubscription('NL', subs)?.name).toBe('Подписка NL');
		expect(owningSubscription('NL · DE-2', subs)?.name).toBe('Подписка NL');
	});

	it('leaves the operator’s own outbounds alone', () => {
		for (const tag of ['direct', 'MySubOther', 'my-vless', 'NL-manual', '']) {
			expect(owningSubscription(tag, subs)).toBeNull();
		}
	});

	// A name with nothing left after sanitising owns no tag at all — claiming ''
	// would make it the owner of every outbound on the page.
	it('claims nothing when the name sanitises to empty', () => {
		expect(owningSubscription('direct', [sub('Подписка')])).toBeNull();
		expect(owningSubscription('', [sub('Подписка')])).toBeNull();
	});
});
