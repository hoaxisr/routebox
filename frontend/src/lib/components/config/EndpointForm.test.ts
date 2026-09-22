import { render, fireEvent } from '@testing-library/svelte';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { addMessages, init } from 'svelte-i18n';

import EndpointForm from './EndpointForm.svelte';
import en from '$lib/i18n/locales/en.json';
import type { Endpoint } from '$lib/types';

beforeAll(() => {
	addMessages('en', en as never);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

const KEY = 'yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk=';
const base = (over: Partial<Endpoint>): Endpoint => ({
	type: 'awg', tag: 'ep', private_key: KEY, address: ['10.0.0.2/32'], mtu: 1420,
	peers: [{ address: '1.2.3.4', port: 51820, public_key: KEY, allowed_ips: ['0.0.0.0/0'] }],
	...over
});

async function submit(endpoint: Endpoint, type?: 'awg' | 'wireguard') {
	const onSave = vi.fn();
	const { container } = render(EndpointForm, { props: { endpoint, onSave, onCancel: () => {} } });
	if (type) await fireEvent.change(container.querySelector('#type')!, { target: { value: type } });
	await fireEvent.submit(container.querySelector('form')!);
	expect(onSave).toHaveBeenCalledTimes(1);
	return onSave.mock.calls[0][0] as Endpoint;
}

// The fork's awg endpoint has none of the wireguard interface options and spells
// the peer PSK "preshared_key"; wireguard spells it "pre_shared_key". The other
// spelling is an unknown field that fails `check`.
describe('EndpointForm: per-type fields', () => {
	it('awg drops system/name/udp_timeout/workers', async () => {
		const ep = await submit(base({ type: 'wireguard', system: true, name: 'wg0', udp_timeout: '5m', workers: 2 }), 'awg');
		expect(ep.type).toBe('awg');
		for (const f of ['system', 'name', 'udp_timeout', 'workers']) expect(ep).not.toHaveProperty(f);
	});

	it('wireguard keeps them', async () => {
		const ep = await submit(base({ type: 'wireguard', system: true, name: 'wg0', udp_timeout: '5m', workers: 2 }));
		expect(ep).toMatchObject({ system: true, name: 'wg0', udp_timeout: '5m', workers: 2 });
	});

	it('wireguard peer PSK → pre_shared_key only', async () => {
		const ep = await submit(base({ type: 'wireguard', peers: [{ ...base({}).peers![0], preshared_key: 'psk' }] }));
		expect(ep.peers![0].pre_shared_key).toBe('psk');
		expect(ep.peers![0]).not.toHaveProperty('preshared_key');
	});

	it('awg peer PSK → preshared_key only', async () => {
		const ep = await submit(base({ peers: [{ ...base({}).peers![0], preshared_key: 'psk' }] }));
		expect(ep.peers![0].preshared_key).toBe('psk');
		expect(ep.peers![0]).not.toHaveProperty('pre_shared_key');
	});

	it('loads an existing wireguard peer written with pre_shared_key', async () => {
		const ep = await submit(base({ type: 'wireguard', peers: [{ ...base({}).peers![0], pre_shared_key: 'psk' }] }));
		expect(ep.peers![0].pre_shared_key).toBe('psk');
	});
});
