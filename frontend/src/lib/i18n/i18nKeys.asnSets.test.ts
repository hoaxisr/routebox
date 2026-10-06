import { describe, it, expect } from 'vitest';
import IntlMessageFormat from 'intl-messageformat';
import en from './locales/en.json';
import ru from './locales/ru.json';

// Keys of the ASN rule-set form and the Rule Sets page rows (#103).
const REQUIRED_KEYS = [
	'asnSets.type', 'asnSets.badge', 'asnSets.numbers', 'asnSets.numbersHint', 'asnSets.invalid',
	'asnSets.interval', 'asnSets.intervalH', 'asnSets.intervalD', 'asnSets.create', 'asnSets.save',
	'asnSets.prefixes', 'asnSets.updatedAgoM', 'asnSets.updatedAgoH', 'asnSets.updatedAgoD', 'asnSets.never',
	'asnSets.refreshNow', 'asnSets.refreshed', 'asnSets.refreshFailed', 'asnSets.lastErrorHint', 'asnSets.created',
	'asnSets.removeAsn'
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

// "{count} префиксов" reads wrong for 1 and 2–4; both locales carry an ICU
// plural and svelte-i18n (intl-messageformat) renders it.
describe('i18n: asnSets.prefixes plural', () => {
	const fmt = (msg: unknown, locale: string, count: number) =>
		new IntlMessageFormat(msg as string, locale).format({ count });
	it('ru declines префикс by count', () => {
		const msg = lookup(ru, 'asnSets.prefixes');
		expect(fmt(msg, 'ru', 1)).toBe('1 префикс');
		expect(fmt(msg, 'ru', 3)).toBe('3 префикса');
		expect(fmt(msg, 'ru', 5)).toBe('5 префиксов');
		expect(fmt(msg, 'ru', 21)).toBe('21 префикс');
		expect(fmt(msg, 'ru', 0)).toBe('0 префиксов');
	});
	it('en has singular and plural', () => {
		const msg = lookup(en, 'asnSets.prefixes');
		expect(fmt(msg, 'en', 1)).toBe('1 prefix');
		expect(fmt(msg, 'en', 2)).toBe('2 prefixes');
	});
});
