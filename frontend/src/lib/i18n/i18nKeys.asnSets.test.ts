import { describe, it, expect } from 'vitest';
import en from './locales/en.json';
import ru from './locales/ru.json';

// Keys of the ASN rule-set form and the Rule Sets page rows (#103).
const REQUIRED_KEYS = [
	'asnSets.type', 'asnSets.badge', 'asnSets.numbers', 'asnSets.numbersHint', 'asnSets.invalid',
	'asnSets.interval', 'asnSets.intervalH', 'asnSets.intervalD', 'asnSets.create', 'asnSets.save',
	'asnSets.prefixes', 'asnSets.updatedAgoM', 'asnSets.updatedAgoH', 'asnSets.updatedAgoD', 'asnSets.never',
	'asnSets.refreshNow', 'asnSets.refreshed', 'asnSets.refreshFailed', 'asnSets.lastErrorHint', 'asnSets.created'
];

function lookup(obj: unknown, path: string): unknown {
	return path.split('.').reduce<unknown>((o, k) => (o && typeof o === 'object' ? (o as Record<string, unknown>)[k] : undefined), obj);
}

describe('i18n: ASN set keys', () => {
	for (const key of REQUIRED_KEYS) {
		it(`${key} exists in en and ru`, () => {
			expect(typeof lookup(en, key)).toBe('string');
			expect(typeof lookup(ru, key)).toBe('string');
		});
	}
});
