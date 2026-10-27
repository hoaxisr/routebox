import { render, fireEvent } from '@testing-library/svelte';
import { beforeAll, describe, expect, it } from 'vitest';
import { addMessages, init } from 'svelte-i18n';

import SelectorForm from './SelectorForm.svelte';
import en from '$lib/i18n/locales/en.json';

beforeAll(() => {
	addMessages('en', en as never);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

const props = (selected: string[]) => ({
	type: 'fallback' as const, availableOutbounds: ['a', 'b', 'c'], selectedOutbounds: selected,
	defaultOutbound: '', interruptExistConnections: false
});
// The queue block: one row per member, "#N tag" in order.
const queue = (c: HTMLElement) =>
	[...c.querySelectorAll('.divide-y > div')].map((r) => r.querySelector('.truncate')?.textContent?.trim());

// Fallback tries members in array order: the arrows are the only way to set it.
describe('SelectorForm: fallback queue', () => {
	it('down on #1 swaps it with #2; the ends are disabled', async () => {
		const { container, getAllByLabelText } = render(SelectorForm, { props: props(['a', 'b', 'c']) });
		const down = getAllByLabelText('Move Down') as HTMLButtonElement[];
		const up = getAllByLabelText('Move Up') as HTMLButtonElement[];
		expect(up[0].disabled).toBe(true);
		expect(down[2].disabled).toBe(true);
		await fireEvent.click(down[0]);
		expect(queue(container)).toEqual(['b', 'a', 'c']);
		await fireEvent.click((getAllByLabelText('Move Up') as HTMLButtonElement[])[2]);
		expect(queue(container)).toEqual(['b', 'c', 'a']);
	});

	it('the arrows hint shows only with a queue to arrange', () => {
		const hint = en.outbounds.fallbackOrderHint;
		expect(render(SelectorForm, { props: props(['a']) }).queryByText(hint)).toBeNull();
		expect(render(SelectorForm, { props: props(['a', 'b']) }).queryByText(hint)).not.toBeNull();
	});
});
