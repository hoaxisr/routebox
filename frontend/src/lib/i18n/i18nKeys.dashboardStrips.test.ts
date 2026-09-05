import { describe, it, expect } from 'vitest';
import en from './locales/en.json';
import ru from './locales/ru.json';

// Keys of the dashboard live strips. A missing one renders the raw dotted
// path on the first screen every operator sees.
const REQUIRED_KEYS = [
	'dashboard.download',
	'dashboard.upload',
	'dashboard.avg',
	'dashboard.cores',
	'dashboard.load',
	'dashboard.ofTotal',
	'dashboard.totalDown',
	'dashboard.totalUp',
	'dashboard.processRss',
	'dashboard.disk',
	'dashboard.lastMinute',
	'dashboard.memory'
];

function lookup(obj: unknown, path: string): unknown {
	return path.split('.').reduce<unknown>((o, k) => (o && typeof o === 'object' ? (o as Record<string, unknown>)[k] : undefined), obj);
}

describe('i18n: dashboard strip keys', () => {
	for (const key of REQUIRED_KEYS) {
		it(`${key} exists in en and ru`, () => {
			expect(typeof lookup(en, key)).toBe('string');
			expect(typeof lookup(ru, key)).toBe('string');
		});
	}
	it('ofTotal carries both placeholders in both locales', () => {
		for (const l of [en, ru]) {
			const s = lookup(l, 'dashboard.ofTotal') as string;
			expect(s).toContain('{used}');
			expect(s).toContain('{total}');
		}
	});
});
