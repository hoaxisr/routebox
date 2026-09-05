import { describe, it, expect } from 'vitest';
import en from './locales/en.json';
import ru from './locales/ru.json';

// Keys of the AWG backup card (#97). A missing one renders the raw dotted path
// on the card that hands out every server secret.
const REQUIRED_KEYS = [
	'awg.backup',
	'awg.backupSub',
	'awg.backupRestoreLocked',
	'awg.backupExport',
	'awg.backupRestore',
	'awg.backupRestoring',
	'awg.backupRestoreConfirm',
	'awg.backupRestored',
	'awg.backupRestoreFailed'
];

function lookup(obj: unknown, path: string): unknown {
	return path.split('.').reduce<unknown>((o, k) => (o && typeof o === 'object' ? (o as Record<string, unknown>)[k] : undefined), obj);
}

describe('i18n: AWG backup keys', () => {
	for (const key of REQUIRED_KEYS) {
		it(`${key} exists in en and ru`, () => {
			expect(typeof lookup(en, key)).toBe('string');
			expect(typeof lookup(ru, key)).toBe('string');
		});
	}
	it('backupRestored carries the {count} placeholder in both locales', () => {
		expect(lookup(en, 'awg.backupRestored')).toContain('{count}');
		expect(lookup(ru, 'awg.backupRestored')).toContain('{count}');
	});
});
