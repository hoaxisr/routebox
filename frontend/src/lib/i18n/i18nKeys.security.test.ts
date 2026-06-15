import { describe, it, expect } from 'vitest';
import en from './locales/en.json';

// Every NEW i18n key introduced by the Panel Security section.
const REQUIRED_KEYS = [
	'settings.securitySection',
	'settings.securityHint',
	'settings.username',
	'settings.saveUsername',
	'settings.usernameSaved',
	'settings.currentPassword',
	'settings.newPassword',
	'settings.confirmPassword',
	'settings.changePassword',
	'settings.passwordChanged',
	'settings.passwordMismatch',
	'settings.passwordTooShort',
	'settings.currentPasswordWrong'
];

function resolve(obj: unknown, dotted: string): unknown {
	return dotted.split('.').reduce<unknown>((acc, part) => {
		if (acc && typeof acc === 'object' && part in (acc as Record<string, unknown>)) {
			return (acc as Record<string, unknown>)[part];
		}
		return undefined;
	}, obj);
}

describe('panel-security i18n keys exist in en.json', () => {
	for (const key of REQUIRED_KEYS) {
		it(`resolves ${key}`, () => {
			const v = resolve(en, key);
			expect(typeof v).toBe('string');
			expect((v as string).length).toBeGreaterThan(0);
		});
	}
});
