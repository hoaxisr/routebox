import { render } from '@testing-library/svelte';
import { beforeAll, describe, expect, it } from 'vitest';
import { addMessages, init } from 'svelte-i18n';

import DraggableRuleList from './DraggableRuleList.svelte';
import en from '$lib/i18n/locales/en.json';
import type { RouteRule } from '$lib/types';

beforeAll(() => {
	addMessages('en', en as never);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

const summary = (rule: RouteRule) => {
	const { container } = render(DraggableRuleList, { props: { rules: [rule], onReorder: () => {}, onEdit: () => {}, onDelete: () => {} } });
	return container.querySelector('span.font-medium.truncate')?.textContent?.trim();
};

// A logical rule has no conditions of its own: summarised from the top level
// it read "All traffic", even when its nested rules pin inbound + rule set (#115).
describe('DraggableRuleList: logical rule summary', () => {
	it('AND of nested conditions, multi-part ones in parentheses', () => {
		expect(summary({
			type: 'logical', mode: 'and', outbound: 'awg',
			rules: [{ inbound: ['tun-lan'], rule_set: ['telegram'] }, { network: 'udp' }]
		})).toBe('(From tun-lan · ruleset: telegram) AND UDP');
	});

	it('OR, nested logical in parentheses, invert in front', () => {
		expect(summary({
			type: 'logical', mode: 'or', invert: true, action: 'reject',
			rules: [{ domain: ['a.com'] }, { type: 'logical', mode: 'and', rules: [{ port: [443] }, { network: 'tcp' }] }]
		})).toBe('NOT (a.com OR (port 443 AND TCP))');
	});

	it('a single nested rule needs no parentheses; an empty one is all traffic', () => {
		expect(summary({ type: 'logical', mode: 'and', rules: [{ inbound: ['tun'], rule_set: ['tg'] }] })).toBe('From tun · ruleset: tg');
		expect(summary({ type: 'logical', mode: 'or', rules: [] })).toBe('All traffic');
	});

	it('the tooltip carries the whole summary', () => {
		const { container } = render(DraggableRuleList, { props: { rules: [{ type: 'logical', mode: 'and', rules: [{ domain: ['a.com'] }, { network: 'udp' }] }], onReorder: () => {}, onEdit: () => {}, onDelete: () => {} } });
		const span = container.querySelector('span.font-medium.truncate')!;
		expect(span.getAttribute('title')).toBe(span.textContent?.trim());
	});

	it('a plain rule is unchanged', () => {
		expect(summary({ domain_suffix: ['t.me'], outbound: 'awg' })).toBe('*.t.me');
	});
});
