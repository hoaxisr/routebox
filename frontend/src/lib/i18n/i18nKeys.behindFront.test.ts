import { describe, it, expect } from 'vitest';
import en from './locales/en.json';
import ru from './locales/ru.json';

// The monitor's explanation for why client addresses are missing behind the
// front. Checked in both locales, not just en: an untranslated explanation is
// the one string a Russian-speaking operator would read at the moment they are
// wondering whether their server is broken.
const REQUIRED_KEYS = [
	'connections.clientAddressUnavailable',
	'connections.clientAddressUnavailableHint',
	'connections.clientAddressUnavailableShort'
];

function resolve(obj: unknown, dotted: string): unknown {
	return dotted.split('.').reduce<unknown>((acc, part) => {
		if (acc && typeof acc === 'object' && part in (acc as Record<string, unknown>)) {
			return (acc as Record<string, unknown>)[part];
		}
		return undefined;
	}, obj);
}

describe('behind-the-front monitor i18n keys', () => {
	for (const key of REQUIRED_KEYS) {
		for (const [name, locale] of [['en', en], ['ru', ru]] as const) {
			it(`${name} resolves ${key}`, () => {
				const v = resolve(locale, key);
				expect(typeof v).toBe('string');
				expect((v as string).length).toBeGreaterThan(0);
			});
		}
	}
});

// naive has no inbound in an out-of-the-box install, so the panel names it in a
// card of its own; the AWG picker names why a backend it cannot run is absent.
const NAIVE_AND_BACKEND_KEYS = [
	'inbounds.naiveServedByDest',
	'inbounds.naiveCopyLink',
	'inbounds.naiveNoLink',
	'awg.backendKernelUnavailable',
	'awg.listenPortFixed'
];

describe('dest-served naive and kernel-backend i18n keys', () => {
	for (const key of NAIVE_AND_BACKEND_KEYS) {
		for (const [name, locale] of [['en', en], ['ru', ru]] as const) {
			it(`${name} resolves ${key}`, () => {
				const v = resolve(locale, key);
				expect(typeof v).toBe('string');
				expect((v as string).length).toBeGreaterThan(0);
			});
		}
	}
});
