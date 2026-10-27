import { render } from '@testing-library/svelte';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { addMessages, init } from 'svelte-i18n';

vi.mock('$lib/api/client', () => ({ api: { switchProxy: vi.fn() } }));

import ProxyCard from './ProxyCard.svelte';
import en from '$lib/i18n/locales/en.json';

beforeAll(() => {
	addMessages('en', en as never);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

const member = (type: string, name: string) => {
	const { getByRole } = render(ProxyCard, { props: { proxy: { name: 'g', type, now: 'a', all: ['a', 'b'] } } });
	return getByRole('button', { name }) as HTMLButtonElement;
};

// The Clash API switches only a Selector ("Must be a Selector" otherwise):
// URLTest and Fallback members are shown, not clickable.
describe('ProxyCard: group members', () => {
	it('a Selector switches by hand', () => {
		expect(member('Selector', 'b').disabled).toBe(false);
	});

	it('URLTest picks its own member', () => {
		expect(member('URLTest', 'b').disabled).toBe(true);
	});

	it('Fallback picks its own member', () => {
		expect(member('Fallback', 'b').disabled).toBe(true);
	});
});
