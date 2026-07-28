import { describe, it, expect } from 'vitest';
import en from './locales/en.json';
import ru from './locales/ru.json';

// Keys of the drop-in notice: the only trace RouteBox leaves of the one file it
// writes outside its own. It is shown on every page for as long as the drop-in
// is installed, so a missing key would render a dotted path forever.
const REQUIRED_KEYS = [
	'configDropIn.title',
	'configDropIn.body',
	'configDropIn.bodyUnknown',
	'configDropIn.pending',
	'configDropIn.remove',
	'configDropIn.removeHint',
	'configDropIn.removed',
	'configDropIn.removedPartly',
	'configDropIn.failed'
];

const REQUIRED_PLACEHOLDERS: Record<string, string[]> = {
	// The notice exists to answer "where did this override come from": the file
	// on disk and the path the unit was retargeted at are the whole answer.
	'configDropIn.title': ['{service}'],
	'configDropIn.body': ['{file}', '{path}', '{service}'],
	// Content unreadable — the file path is all we have, and it is the part the
	// user needs to remove it by hand.
	'configDropIn.bodyUnknown': ['{file}', '{service}'],
	// "Written but not applied" is about a specific unit path the process is
	// still being started with.
	'configDropIn.pending': ['{unit}'],
	// Removal brings the unit back to its own ExecStart — which is why the
	// mismatch may return. Saying so needs both the unit and our path.
	'configDropIn.removeHint': ['{service}', '{ours}'],
	'configDropIn.removed': ['{service}'],
	'configDropIn.removedPartly': ['{error}'],
	'configDropIn.failed': ['{error}']
};

// The point of the hint is the warning, not the button label: taking the drop-in
// off can bring the config path mismatch back, and that has to be said BEFORE
// the click, not shown as a surprise afterwards.
const MISMATCH_WARNED: Record<string, RegExp> = {
	en: /mismatch/i,
	ru: /расхожден/i
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
])('config drop-in i18n keys in %s.json', (name, locale) => {
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

	it('warns before the click that the mismatch can come back', () => {
		const v = resolve(locale, 'configDropIn.removeHint') as string;
		expect(v).toMatch(MISMATCH_WARNED[name as string]);
	});
});
