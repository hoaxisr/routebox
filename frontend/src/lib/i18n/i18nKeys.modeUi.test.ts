import { describe, it, expect } from 'vitest';
import en from './locales/en.json';

// Every NEW i18n key introduced by the mode-aware-ui feature (Tasks 5/7/8).
// Asserts each dotted path resolves to a non-empty string in en.json.
const REQUIRED_KEYS = [
	// Task 5 — redirect guard
	'mode.redirected',
	// Task 7 — Overview panel summary
	'dashboard.panelUsers',
	'dashboard.manageUsers',
	'dashboard.publicHost',
	'dashboard.publicHostUnset',
	'dashboard.editSettings',
	'dashboard.serverInbounds',
	'dashboard.serverInboundsHint',
	'dashboard.manageInbounds',
	// Task 8 — Settings mode toggle + panel fields
	'settings.serverMode',
	'settings.serverModeRouter',
	'settings.serverModeVps',
	'settings.serverModeHint',
	'settings.publicPort',
	'settings.publicPortHint',
	'settings.panelTls',
	'settings.panelTlsHint',
	'settings.acmeEnabled',
	'settings.acmeEmail',
	'settings.acmeStaging',
	'settings.acmeCacheDir',
	'settings.tlsCertPath',
	'settings.tlsKeyPath'
];

function resolve(obj: unknown, dotted: string): unknown {
	return dotted.split('.').reduce<unknown>((acc, part) => {
		if (acc && typeof acc === 'object' && part in (acc as Record<string, unknown>)) {
			return (acc as Record<string, unknown>)[part];
		}
		return undefined;
	}, obj);
}

describe('mode-aware-ui i18n keys exist in en.json', () => {
	for (const key of REQUIRED_KEYS) {
		it(`resolves ${key}`, () => {
			const v = resolve(en, key);
			expect(typeof v).toBe('string');
			expect((v as string).length).toBeGreaterThan(0);
		});
	}
});
