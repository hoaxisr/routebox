import { describe, it, expect } from 'vitest';
import en from './locales/en.json';
import ru from './locales/ru.json';

// Keys the TrustTunnel outbound form and import modal render; a missing one shows the raw path.
const REQUIRED_KEYS = [
	'outbounds.trusttunnel',
	'outbounds.trusttunnelDesc',
	'outbounds.importFromTrustTunnel',
	'outbounds.trustTunnelForm.insecure',
	'outbounds.trustTunnelForm.antiDpi',
	'outbounds.trustTunnelForm.antiDpiHint',
	'outbounds.trustTunnelForm.healthCheck',
	'outbounds.trustTunnelForm.multiAddress'
];

function lookup(obj: unknown, path: string): unknown {
	return path.split('.').reduce<unknown>((o, k) => (o && typeof o === 'object' ? (o as Record<string, unknown>)[k] : undefined), obj);
}

describe('i18n: TrustTunnel keys', () => {
	for (const key of REQUIRED_KEYS) {
		it(`${key} exists in en and ru`, () => {
			expect(typeof lookup(en, key)).toBe('string');
			expect(typeof lookup(ru, key)).toBe('string');
		});
	}
	it('importUnknownFormat mentions tt://', () => {
		expect(String(lookup(en, 'outbounds.importUnknownFormat'))).toContain('tt://');
		expect(String(lookup(ru, 'outbounds.importUnknownFormat'))).toContain('tt://');
	});
});
