import { describe, it, expect } from 'vitest';
import en from './locales/en.json';
import ru from './locales/ru.json';
import { BBR_PROFILES } from '$lib/utils/serverInbound';

// Keys of the hysteria2 congestion-control controls (#59). Every one of these
// sits on a toggle button, a checkbox label or the hint that explains what the
// rate fields actually do — a missing key shows up as a raw dotted path in the
// middle of the form, and the hints are the whole point of the change.
const REQUIRED_KEYS = [
	'outbounds.bandwidthHint',
	'outbounds.bbrProfile',
	'outbounds.bbrProfileHint',
	'inbounds.server.bandwidthHint',
	'inbounds.server.ignoreClientBandwidth',
	'inbounds.server.ccClientDecides',
	'inbounds.server.ccBbrOnly',
	'inbounds.server.ccBrutalOnly',
	'inbounds.server.bbrProfile',
	'inbounds.server.bbrProfileHint',
	// Derived, not listed: the forms render one button per entry of this list, so
	// a profile added there without a label would otherwise ship as a raw dotted
	// path on a button and no test would notice.
	...BBR_PROFILES.map((p) => `inbounds.server.bbr.${p}`)
];

function resolve(obj: unknown, path: string): unknown {
	return path.split('.').reduce<unknown>((acc, k) => (acc as Record<string, unknown>)?.[k], obj);
}

describe('hysteria2 congestion control i18n keys', () => {
	for (const [name, dict] of [
		['en', en],
		['ru', ru]
	] as const) {
		for (const key of REQUIRED_KEYS) {
			it(`${name} has ${key}`, () => {
				const value = resolve(dict, key);
				expect(typeof value).toBe('string');
				expect(value).not.toBe('');
			});
		}
	}
});
