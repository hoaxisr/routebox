import { render, fireEvent } from '@testing-library/svelte';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { addMessages, init } from 'svelte-i18n';

import DnsRuleForm from './DnsRuleForm.svelte';
import en from '$lib/i18n/locales/en.json';
import type { DnsRule, DnsServer } from '$lib/types';

beforeAll(() => {
	addMessages('en', en as never);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

const dnsServers = [{ tag: 'd', type: 'udp', server: '1.1.1.1' }] as DnsServer[];

async function submit(rule: DnsRule) {
	const onSave = vi.fn();
	const { container } = render(DnsRuleForm, { props: { rule, dnsServers, ruleSets: [], onSave, onCancel: () => {} } });
	await fireEvent.submit(container.querySelector('form')!);
	expect(onSave).toHaveBeenCalledTimes(1);
	return onSave.mock.calls[0][0] as DnsRule;
}

describe('DnsRuleForm: sing-box 1.14+ shapes', () => {
	// ip_cidr matches the RESPONSE now; without match_response check fails.
	it('ip_cidr emits match_response', async () => {
		const r = await submit({ ip_cidr: ['1.1.1.1/32'], server: 'd' });
		expect(r.match_response).toBe(true);
	});

	it('disable_cache only with the route action', async () => {
		expect(await submit({ domain: ['x'], action: 'reject', disable_cache: true })).not.toHaveProperty('disable_cache');
		expect((await submit({ domain: ['x'], server: 'd', disable_cache: true })).disable_cache).toBe(true);
	});
});
