import { render, fireEvent, screen } from '@testing-library/svelte';
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { addMessages, init } from 'svelte-i18n';

// Mocked before the component imports them: the share dialog fetches the
// links per click, and jsdom has no canvas for the QR code.
vi.mock('$lib/api/client', () => ({
	api: { getMtprotoClientLink: vi.fn() }
}));
vi.mock('qrcode', () => ({
	default: { toDataURL: vi.fn(async () => 'data:image/png;base64,qr') }
}));

import QRCode from 'qrcode';
import { api } from '$lib/api/client';
import ClientRoster from './ClientRoster.svelte';
import en from '$lib/i18n/locales/en.json';
import type { MtprotoClient, MtprotoLink } from '$lib/types';

const getLink = api.getMtprotoClientLink as unknown as ReturnType<typeof vi.fn>;
const toDataURL = QRCode.toDataURL as unknown as ReturnType<typeof vi.fn>;

beforeAll(() => {
	addMessages('en', en as never);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

beforeEach(() => {
	getLink.mockReset();
	toDataURL.mockClear();
});

const alice: MtprotoClient = { name: 'alice', enabled: true, created_at: 0, expires_at: 0, online: false };

const fakeTls = { tg: 'tg://proxy?server=h&port=443&secret=ee', web: 'https://t.me/proxy?server=h&port=443&secret=ee' };
const webproxy = { tg: 'tg://webproxy?server=media.example.com&secret=dd', web: 'https://t.me/webproxy?server=media.example.com&secret=dd' };

function textareas(): string[] {
	return [...document.body.querySelectorAll('textarea.link-text')].map((t) => (t as HTMLTextAreaElement).value);
}

async function openShare(link: MtprotoLink, props: Record<string, unknown>, button: 'Link' | 'QR') {
	getLink.mockResolvedValue(link);
	render(ClientRoster, { props: { clients: [alice], canShare: true, onChange: () => {}, ...props } });
	const btn = screen.getAllByRole('button').find((b) => b.textContent?.trim() === button)!;
	await fireEvent.click(btn);
	await vi.waitFor(() => expect(document.body.textContent).toContain(en.telegram.transportMtproto));
}

// Spec §7.6: the share dialog carries the WEB proxy link when the server
// issues one, and only the Fake TLS links when it does not.
describe('ClientRoster: share dialog', () => {
	it('with a WEB proxy link: both sections, the webproxy link first', async () => {
		await openShare({ ...fakeTls, webproxy }, { canShareWeb: true }, 'Link');
		expect(document.body.textContent).toContain(en.telegram.transportWeb);
		expect(textareas()).toEqual([webproxy.tg, fakeTls.tg, fakeTls.web]);
	});

	it('without one: no WEB proxy section', async () => {
		await openShare({ ...fakeTls, webproxy: null }, {}, 'Link');
		expect(document.body.textContent).not.toContain(en.telegram.transportWeb);
		expect(textareas()).toEqual([fakeTls.tg, fakeTls.web]);
	});

	it('all-in-one: the QR encodes the WEB proxy link and the Fake TLS one is flagged as unpublished', async () => {
		await openShare({ ...fakeTls, webproxy }, { canShareWeb: true, webManaged: true, fakeTlsPort: '9447' }, 'QR');
		expect(toDataURL).toHaveBeenCalledWith(webproxy.tg, expect.anything());
		expect(document.body.textContent).toContain('Fake TLS port 9447 is not published');
	});
});
