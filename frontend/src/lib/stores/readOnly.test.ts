import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

vi.mock('$lib/api/client', () => ({
	api: { getStatus: vi.fn() }
}));
import { api } from '$lib/api/client';
import {
	refreshStatus,
	configReadOnly,
	configReadOnlyPath,
	readOnlyPaths,
	anyReadOnly
} from './status';

const getStatus = api.getStatus as unknown as ReturnType<typeof vi.fn>;

describe('read-only state derived from /api/status', () => {
	beforeEach(() => {
		getStatus.mockReset();
	});

	// Guessing "read-only" from a status we never received would disable every
	// save button on a healthy install.
	it('assumes writable before the first poll', () => {
		expect(get(anyReadOnly)).toBe(false);
	});

	// The state RouteBox persists is spread over three directories. A store that
	// cannot be written has to raise the badge even though the config is fine —
	// this is the common case, not an exotic one.
	it('raises the badge for a store while leaving the config flag alone', async () => {
		getStatus.mockResolvedValue({
			running: true,
			config_read_only: false,
			read_only_paths: ['/etc/routebox/users.toml']
		});
		await refreshStatus();

		expect(get(anyReadOnly)).toBe(true);
		expect(get(readOnlyPaths)).toEqual(['/etc/routebox/users.toml']);
		// config_read_only is what greys out the config save buttons; an
		// unwritable users.toml must not disable config editing.
		expect(get(configReadOnly)).toBe(false);
	});

	it('reports every unwritable path, not just the first', async () => {
		getStatus.mockResolvedValue({
			running: true,
			config_read_only: true,
			config_read_only_path: '/etc/sing-box/config.json',
			read_only_paths: ['/etc/routebox/users.toml', '/etc/sing-box/config.json']
		});
		await refreshStatus();

		expect(get(readOnlyPaths)).toHaveLength(2);
		expect(get(configReadOnly)).toBe(true);
		expect(get(configReadOnlyPath)).toBe('/etc/sing-box/config.json');
	});

	it('shows nothing on a healthy install', async () => {
		getStatus.mockResolvedValue({ running: true });
		await refreshStatus();

		expect(get(anyReadOnly)).toBe(false);
		expect(get(readOnlyPaths)).toEqual([]);
	});

});
