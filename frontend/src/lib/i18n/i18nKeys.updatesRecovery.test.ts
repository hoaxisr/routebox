import { describe, it, expect } from 'vitest';
import en from './locales/en.json';
import ru from './locales/ru.json';

// Keys the updates page renders after a lost apply response; a missing one shows the raw path.
const REQUIRED_KEYS = [
	'updates.connectionLost',
	'updates.notStarted',
	'updates.lastAttemptFailed',
	'updates.updateFailed',
	'updates.updatedTo'
];

function lookup(obj: unknown, path: string): unknown {
	return path.split('.').reduce<unknown>((o, k) => (o && typeof o === 'object' ? (o as Record<string, unknown>)[k] : undefined), obj);
}

describe('i18n: updates recovery keys', () => {
	for (const key of REQUIRED_KEYS) {
		it(`${key} exists in en and ru`, () => {
			expect(typeof lookup(en, key)).toBe('string');
			expect(typeof lookup(ru, key)).toBe('string');
		});
	}
});
