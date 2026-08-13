import { describe, it, expect } from 'vitest';
import en from './locales/en.json';
import ru from './locales/ru.json';

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
	'inbounds.server.bbr.standard',
	'inbounds.server.bbr.conservative',
	'inbounds.server.bbr.aggressive'
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
