import { render } from '@testing-library/svelte';
import { beforeAll, describe, expect, it } from 'vitest';
import { addMessages, init } from 'svelte-i18n';

import ServerSettingsForm from './ServerSettingsForm.svelte';
import en from '$lib/i18n/locales/en.json';
import type { MtprotoSettings, MtprotoWeb } from '$lib/types';

beforeAll(() => {
	addMessages('en', en as never);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

const form = (): MtprotoSettings => ({
	enabled: true,
	listen: '0.0.0.0:9443',
	masking_domain: 'storage.googleapis.com',
	public_host: '',
	public_port: 0,
	concurrency: 8192,
	idle_timeout_sec: 60,
	prefer_ip: '',
	domain_fronting_port: 443,
	outbound: '',
	socks_port: 1080,
	web_enabled: true,
	web_hostname: 'proxy.example.com',
	web_listen: '127.0.0.1:9446',
	web_public_dir: ''
});

const web = (managed: boolean): MtprotoWeb => ({
	enabled: true,
	managed,
	hostname: managed ? 'media.example.com' : 'proxy.example.com',
	listen: '127.0.0.1:9446',
	public_dir: '',
	can_issue_link: true,
	sessions: 0,
	streams: 0
});

function renderForm(managed: boolean) {
	// $state, as the page holds it: the form binds into it.
	const state = $state(form());
	return render(ServerSettingsForm, {
		props: { form: state, savedDomain: 'storage.googleapis.com', web: web(managed), onSave: () => {}, onReset: () => {} }
	}).container;
}

// Spec §7.6: on all-in-one the panel owns the WEB front, so the operator gets
// a note naming the domain and nothing to wire; elsewhere the hostname,
// listen and public-dir fields and the reverse-proxy snippets.
describe('ServerSettingsForm: WEB group', () => {
	it('managed: a note naming the domain, no fields and no snippets', () => {
		const c = renderForm(true);
		expect(c.querySelector('#mt-web-enabled')).not.toBeNull();
		expect(c.textContent).toContain('media.example.com');
		for (const id of ['#mt-web-host', '#mt-web-listen', '#mt-web-dir', '#mt-web-nginx', '#mt-web-caddy']) {
			expect(c.querySelector(id), id).toBeNull();
		}
	});

	it('unmanaged: the fields and both snippets built from the typed values', () => {
		const c = renderForm(false);
		expect((c.querySelector('#mt-web-host') as HTMLInputElement).value).toBe('proxy.example.com');
		expect((c.querySelector('#mt-web-listen') as HTMLInputElement).value).toBe('127.0.0.1:9446');
		expect(c.querySelector('#mt-web-dir')).not.toBeNull();
		const nginx = (c.querySelector('#mt-web-nginx') as HTMLTextAreaElement).value;
		expect(nginx).toContain('server_name proxy.example.com;');
		expect(nginx).toContain('proxy_pass http://127.0.0.1:9446;');
		expect((c.querySelector('#mt-web-caddy') as HTMLTextAreaElement).value).toBe(
			'proxy.example.com {\n\treverse_proxy 127.0.0.1:9446\n}'
		);
	});
});
