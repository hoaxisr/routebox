import { describe, it, expect } from 'vitest';
import en from './locales/en.json';
import ru from './locales/ru.json';

// Keys used by the read-only indicator: the header badge, the panel it opens
// (which is how a touch user reads the unwritable paths — a title attribute
// never appears without a mouse) and the title on every disabled save button.
// Both locales must carry all of them — a missing key renders the raw dotted
// path, and these strings only ever show up when the user is already blocked
// and needs to understand why.
const REQUIRED_KEYS = [
	'readOnly.badge',
	'readOnly.reason',
	'readOnly.path',
	'readOnly.paths',
	'readOnly.details',
	'readOnly.hint',
	'readOnly.saveBlocked'
];

// The unwritable path is the one actionable fact in the tooltip. A locale that
// drops the placeholder leaves the user with "cannot write:" and nothing else.
const REQUIRED_PLACEHOLDERS: Record<string, string[]> = {
	'readOnly.path': ['{path}']
};

function resolve(obj: unknown, dotted: string): unknown {
	return dotted.split('.').reduce<unknown>((acc, part) => {
		if (acc && typeof acc === 'object' && part in (acc as Record<string, unknown>)) {
			return (acc as Record<string, unknown>)[part];
		}
		return undefined;
	}, obj);
}

describe.each([
	['en', en],
	['ru', ru]
])('read-only i18n keys in %s.json', (_name, locale) => {
	for (const key of REQUIRED_KEYS) {
		it(`resolves ${key} to a non-empty string`, () => {
			const v = resolve(locale, key);
			expect(typeof v).toBe('string');
			expect((v as string).length).toBeGreaterThan(0);
		});
	}

	for (const [key, placeholders] of Object.entries(REQUIRED_PLACEHOLDERS)) {
		it(`keeps the placeholders of ${key}`, () => {
			const v = resolve(locale, key) as string;
			for (const p of placeholders) {
				expect(v).toContain(p);
			}
		});
	}
});

// The badge is the only user-facing text that is not a full sentence, and the
// English backend error must never leak into it: the message the API returns
// ("read-only: RouteBox cannot write ...") is English-only, so the Russian
// strings have to stand on their own.
describe('read-only strings are actually localised', () => {
	for (const key of REQUIRED_KEYS) {
		it(`${key} differs between en and ru`, () => {
			expect(resolve(ru, key)).not.toBe(resolve(en, key));
		});
	}
});
