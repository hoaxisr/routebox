import { render, fireEvent, waitFor } from '@testing-library/svelte';
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { addMessages, init } from 'svelte-i18n';

vi.mock('$lib/api/client', () => ({ api: { listDir: vi.fn() } }));

import { api } from '$lib/api/client';
import LocalFilePicker from './LocalFilePicker.svelte';
import en from '$lib/i18n/locales/en.json';

beforeAll(() => {
	addMessages('en', en as never);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

const listing = { path: '/opt/srs', parent: '/opt', entries: [{ name: 'old', dir: true }, { name: 'yt.srs', dir: false, size: 5 }] };

describe('LocalFilePicker', () => {
	beforeEach(() => vi.mocked(api.listDir).mockReset());

	it('opens in the folder of the current file, preselected', async () => {
		vi.mocked(api.listDir).mockResolvedValue(listing);
		const onPick = vi.fn();
		const { getByText } = render(LocalFilePicker, { props: { start: '/opt/srs/yt.srs', onPick, onCancel: () => {} } });
		await waitFor(() => expect((getByText('Select') as HTMLButtonElement).disabled).toBe(false));
		expect(api.listDir).toHaveBeenCalledWith('/opt/srs');
		await fireEvent.click(getByText('Select'));
		expect(onPick).toHaveBeenCalledWith('/opt/srs/yt.srs');
	});

	it('a folder that is gone falls back to the config dir', async () => {
		vi.mocked(api.listDir).mockRejectedValueOnce(new Error('404')).mockResolvedValue(listing);
		render(LocalFilePicker, { props: { start: '/gone/yt.srs', onPick: () => {}, onCancel: () => {} } });
		await waitFor(() => expect(api.listDir).toHaveBeenLastCalledWith(''));
	});

	it('double-click on a file picks it; a folder click opens it', async () => {
		vi.mocked(api.listDir).mockResolvedValue(listing);
		const onPick = vi.fn();
		const { findByText } = render(LocalFilePicker, { props: { start: '', onPick, onCancel: () => {} } });
		await fireEvent.dblClick(await findByText(/yt\.srs/));
		expect(onPick).toHaveBeenCalledWith('/opt/srs/yt.srs');
		await fireEvent.click(await findByText('📁 old'));
		expect(api.listDir).toHaveBeenLastCalledWith('/opt/srs/old');
	});
});
