import { describe, it, expect } from 'vitest';
import { clientStatus, relExpiry, canShare, type ClientStatus } from './roster';

const at = (expires: number, enabled = true, online = false) => ({
	name: 'x',
	enabled,
	online,
	created_at: 0,
	expires_at: expires
});

describe('clientStatus', () => {
	const now = 1_000_000;

	it('reports an online client', () => {
		expect(clientStatus(at(0, true, true), now)).toBe<ClientStatus>('online');
	});

	it('reports an enabled idle client as offline', () => {
		expect(clientStatus(at(0, true, false), now)).toBe<ClientStatus>('offline');
	});

	it('reports a disabled client as disabled', () => {
		expect(clientStatus(at(0, false, false), now)).toBe<ClientStatus>('disabled');
	});

	it('reports expired over disabled', () => {
		// The two coincide once the sweep runs, and expired is the one that says
		// what to do about it — extend, rather than just switch back on.
		expect(clientStatus(at(now - 1, false, false), now)).toBe<ClientStatus>('expired');
	});

	it('treats the deadline itself as expired, matching the server', () => {
		expect(clientStatus(at(now, true, false), now)).toBe<ClientStatus>('expired');
		expect(clientStatus(at(now + 1, true, false), now)).toBe<ClientStatus>('offline');
	});

	it('treats zero as never expiring', () => {
		expect(clientStatus(at(0, true, false), Number.MAX_SAFE_INTEGER)).toBe<ClientStatus>('offline');
	});

	it('does not call a disabled client online even if a stream is still open', () => {
		// A rebuild drops its connections a moment later; showing "online" for a
		// client an admin just switched off reads as the toggle having failed.
		expect(clientStatus(at(0, false, true), now)).toBe<ClientStatus>('disabled');
	});
});

describe('relExpiry', () => {
	const now = 1_000_000;

	it('uses days when a day or more remains', () => {
		expect(relExpiry(now + 86400 * 30, now)).toBe('30d');
	});

	it('uses hours below a day', () => {
		expect(relExpiry(now + 3600 * 5, now)).toBe('5h');
	});

	it('never reports zero hours for something not yet expired', () => {
		expect(relExpiry(now + 60, now)).toBe('1h');
	});

	it('clamps a past deadline rather than going negative', () => {
		expect(relExpiry(now - 86400, now)).toBe('1h');
	});
});

describe('canShare', () => {
	it('needs both a masking domain and a public host', () => {
		expect(canShare('example.com', 'panel.example.com')).toBe(true);
	});

	it('refuses without a masking domain', () => {
		// The domain is part of the secret; without it Telegram fails silently.
		expect(canShare('', 'panel.example.com')).toBe(false);
	});

	it('refuses without a public host', () => {
		expect(canShare('example.com', '')).toBe(false);
	});

	it('treats whitespace as missing', () => {
		expect(canShare('  ', 'panel.example.com')).toBe(false);
		expect(canShare('example.com', ' ')).toBe(false);
	});
});
