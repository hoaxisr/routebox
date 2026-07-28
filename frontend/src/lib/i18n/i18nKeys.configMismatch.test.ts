import { describe, it, expect } from 'vitest';
import en from './locales/en.json';
import ru from './locales/ru.json';

// Keys used by the config-path-mismatch banner (shown on every page).
// Both locales must carry all of them: a missing key renders the raw dotted
// path to the user, and this banner only ever appears when something is wrong.
const REQUIRED_KEYS = [
	'configMismatch.title',
	'configMismatch.body',
	'configMismatch.useUnit',
	'configMismatch.useUnitHint',
	'configMismatch.fixUnit',
	'configMismatch.foreignUnit',
	'configMismatch.fixed',
	'configMismatch.switched',
	'configMismatch.notPersisted',
	'configMismatch.refreshFailed',
	'configMismatch.restartTitle',
	'configMismatch.restartBody',
	'configMismatch.restartNow',
	'configMismatch.failed'
];

// Placeholders the banner interpolates, per key. A locale that drops one shows
// a message with a hole in it exactly where the config path should be.
// {service} names the systemd unit both fixes would modify: without it the
// banner talks about "the systemd unit" while the user cannot tell which one.
const REQUIRED_PLACEHOLDERS: Record<string, string[]> = {
	'configMismatch.body': ['{ours}', '{unit}', '{service}'],
	'configMismatch.fixUnit': ['{service}'],
	'configMismatch.useUnitHint': ['{service}'],
	'configMismatch.foreignUnit': ['{service}'],
	'configMismatch.switched': ['{path}'],
	// Why the adopted path will not survive a restart comes from the backend;
	// without the placeholder the user is told "something went wrong" and
	// nothing about what.
	'configMismatch.notPersisted': ['{error}'],
	// The process disagreement is about two specific files: the one the running
	// process was started with and the one RouteBox edits. Naming only ours
	// leaves the user unable to tell which file the process is actually on.
	'configMismatch.restartBody': ['{ours}', '{process}'],
	'configMismatch.failed': ['{error}']
};

// The two buttons do different things, and the foreign-unit warning is where
// the user decides whether to press either: "Point {service}.service at our
// config" writes a drop-in into that unit, while "Adopt the detected config
// path" changes RouteBox and leaves the unit untouched. The warning used to say
// both of them change the unit, which is exactly the misunderstanding that
// makes someone edit a foreign unit by mistake.
const FOREIGN_UNIT_BOTH_CLAIM: Record<string, RegExp> = {
	en: /both\s+(fixes|buttons|actions)/i,
	ru: /об[ае]\s+(кнопки|варианта|действия)/i
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
])('config-mismatch i18n keys in %s.json', (name, locale) => {
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

	it('does not claim both fixes change the foreign unit', () => {
		const v = resolve(locale, 'configMismatch.foreignUnit') as string;
		expect(v).not.toMatch(FOREIGN_UNIT_BOTH_CLAIM[name as string]);
		// Only one of the two touches the unit; the other one moves RouteBox, and
		// the warning has to name it so the difference is visible.
		expect(v).toContain('RouteBox');
	});
});
