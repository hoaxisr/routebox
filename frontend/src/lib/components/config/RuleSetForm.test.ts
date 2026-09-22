import { render, fireEvent } from '@testing-library/svelte';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { addMessages, init } from 'svelte-i18n';

import RuleSetForm from './RuleSetForm.svelte';
import en from '$lib/i18n/locales/en.json';
import type { Outbound, RuleSet } from '$lib/types';

beforeAll(() => {
	addMessages('en', en as never);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

const outbounds = [{ tag: 'proxy', type: 'selector' }, { tag: 'direct', type: 'direct' }] as Outbound[];
const remote = (over: Partial<RuleSet>): RuleSet =>
	({ tag: 'ads', type: 'remote', format: 'binary', url: 'https://example.com/ads.srs', ...over });

async function submit(ruleSet: RuleSet) {
	const onSave = vi.fn();
	const { container } = render(RuleSetForm, { props: { existingTags: [], outbounds, ruleSet, onSave, onCancel: () => {} } });
	await fireEvent.submit(container.querySelector('form')!);
	expect(onSave).toHaveBeenCalledTimes(1);
	return { saved: onSave.mock.calls[0][0] as RuleSet, container };
}

// sing-box 1.15 refuses download_detour at start (check passes); the download
// client is http_client, and a detour to the empty direct outbound is invalid.
describe('RuleSetForm: http_client', () => {
	it('proxy detour → http_client {detour}, never download_detour', async () => {
		const { saved } = await submit(remote({ http_client: { detour: 'proxy' } }));
		expect(saved.http_client).toEqual({ detour: 'proxy' });
		expect(saved).not.toHaveProperty('download_detour');
	});

	it('no detour → no http_client key', async () => {
		const { saved } = await submit(remote({}));
		expect(saved).not.toHaveProperty('http_client');
	});

	it('direct outbound chosen → no http_client (empty direct detour is rejected)', async () => {
		const { saved } = await submit(remote({ http_client: { detour: 'direct' } }));
		expect(saved).not.toHaveProperty('http_client');
	});

	it('shared client tag stays a string and the select is disabled', async () => {
		const { saved, container } = await submit(remote({ http_client: 'shared' }));
		expect(saved.http_client).toBe('shared');
		expect((container.querySelector('#downloadDetour') as HTMLSelectElement).disabled).toBe(true);
	});
});
