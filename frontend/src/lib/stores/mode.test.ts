import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

// Mock the API client BEFORE importing the store (store imports `api`).
vi.mock('$lib/api/client', () => ({
	api: { getSettings: vi.fn() }
}));
import { api } from '$lib/api/client';
import { serverMode, panelMode, routerMode, behindFront, loadMode, refreshMode, __resetMode } from './mode';

const getSettings = api.getSettings as unknown as ReturnType<typeof vi.fn>;

describe('mode store — fail-safe default', () => {
	beforeEach(() => {
		__resetMode();
		getSettings.mockReset();
	});

	it('defaults to router (full UI) before any load', () => {
		expect(get(serverMode)).toBe('router');
		expect(get(routerMode)).toBe(true);
		expect(get(panelMode)).toBe(false);
	});

	it('derives panelMode/routerMode from serverMode', () => {
		serverMode.set('vps');
		expect(get(panelMode)).toBe(true);
		expect(get(routerMode)).toBe(false);
		serverMode.set('router');
		expect(get(panelMode)).toBe(false);
		expect(get(routerMode)).toBe(true);
	});

	it('stays router when getSettings rejects (never lock the admin out)', async () => {
		getSettings.mockRejectedValueOnce(new Error('network'));
		await loadMode();
		expect(get(serverMode)).toBe('router');
		expect(get(panelMode)).toBe(false);
	});

	it('stays router when server is missing from settings', async () => {
		getSettings.mockResolvedValueOnce({ settings: {} });
		await loadMode();
		expect(get(serverMode)).toBe('router');
	});

	it('stays router when mode is missing/empty', async () => {
		getSettings.mockResolvedValueOnce({ settings: { server: { mode: '' } } });
		await loadMode();
		expect(get(serverMode)).toBe('router');
	});

	it('moves to vps ONLY on explicit mode === "vps"', async () => {
		getSettings.mockResolvedValueOnce({ settings: { server: { mode: 'vps' } } });
		await loadMode();
		expect(get(serverMode)).toBe('vps');
		expect(get(panelMode)).toBe(true);
		expect(get(routerMode)).toBe(false);
	});

	it('treats any non-vps value as router (unknown/garbage is safe)', async () => {
		getSettings.mockResolvedValueOnce({ settings: { server: { mode: 'banana' } } });
		await loadMode();
		expect(get(serverMode)).toBe('router');
	});

	it('refreshMode re-reads and can switch vps -> router live', async () => {
		getSettings.mockResolvedValueOnce({ settings: { server: { mode: 'vps' } } });
		await loadMode();
		expect(get(serverMode)).toBe('vps');
		getSettings.mockResolvedValueOnce({ settings: { server: { mode: 'router' } } });
		await refreshMode();
		expect(get(serverMode)).toBe('router');
	});

	it('refreshMode failure keeps last-good mode (never downgrades on error)', async () => {
		getSettings.mockResolvedValueOnce({ settings: { server: { mode: 'vps' } } });
		await loadMode();
		getSettings.mockRejectedValueOnce(new Error('flaky'));
		await refreshMode();
		expect(get(serverMode)).toBe('vps');
	});
});

describe('behindFront — client addresses are unobservable behind the front', () => {
	beforeEach(() => {
		__resetMode();
		getSettings.mockReset();
	});

	it('is false before any load, so every other install sees the usual monitor', () => {
		expect(get(behindFront)).toBe(false);
	});

	it('turns on only when settings say the install was bootstrapped', async () => {
		getSettings.mockResolvedValueOnce({ settings: { server: { bootstrapped: true } } });
		await loadMode();
		expect(get(behindFront)).toBe(true);
	});

	it('stays off for an install that was not bootstrapped', async () => {
		getSettings.mockResolvedValueOnce({ settings: { server: { mode: 'vps' } } });
		await loadMode();
		expect(get(behindFront)).toBe(false);
	});

	// A truthy-but-not-true value (a string "false" from a hand-edited toml, say)
	// must not hide the addresses of an install that has them.
	it('takes only a real true', async () => {
		getSettings.mockResolvedValueOnce({ settings: { server: { bootstrapped: 'false' } } });
		await loadMode();
		expect(get(behindFront)).toBe(false);
	});

	it('goes back off when a later read says so', async () => {
		getSettings.mockResolvedValueOnce({ settings: { server: { bootstrapped: true } } });
		await loadMode();
		getSettings.mockResolvedValueOnce({ settings: { server: {} } });
		await refreshMode();
		expect(get(behindFront)).toBe(false);
	});
});
