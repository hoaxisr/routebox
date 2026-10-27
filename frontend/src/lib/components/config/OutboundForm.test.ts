import { render, fireEvent } from '@testing-library/svelte';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { addMessages, init } from 'svelte-i18n';

import OutboundForm from './OutboundForm.svelte';
import en from '$lib/i18n/locales/en.json';
import type { Outbound } from '$lib/types';

beforeAll(() => {
	addMessages('en', en as never);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

const members = [{ tag: 'a', type: 'direct' }, { tag: 'b', type: 'direct' }] as Outbound[];

async function save(outbound: Outbound) {
	const onSave = vi.fn();
	const { container } = render(OutboundForm, { props: { outbound, endpoints: [], outbounds: members, onSave, onCancel: () => {} } });
	await fireEvent.submit(container.querySelector('form')!);
	expect(onSave).toHaveBeenCalledTimes(1);
	return onSave.mock.calls[0][0] as Outbound;
}

// sing-box 1.15 decodes strictly: a fallback group carrying urltest's
// tolerance is refused at check.
describe('OutboundForm: fallback group', () => {
	it('keeps members in order with url/interval, never tolerance', async () => {
		const saved = await save({ type: 'fallback', tag: 'fb', outbounds: ['b', 'a'], url: 'https://cp.cloudflare.com', interval: '1m', tolerance: 200 } as Outbound);
		expect(saved).toMatchObject({ type: 'fallback', tag: 'fb', outbounds: ['b', 'a'], url: 'https://cp.cloudflare.com', interval: '1m' });
		expect(saved).not.toHaveProperty('tolerance');
		expect(saved).not.toHaveProperty('default');
	});
});
