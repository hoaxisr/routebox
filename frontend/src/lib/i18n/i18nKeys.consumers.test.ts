import { describe, it, expect } from 'vitest';
import en from './locales/en.json';
import ru from './locales/ru.json';

// Keys of the unified consumers monitor (/monitor/consumers, #109). A missing
// one renders the raw dotted path in the sidebar or the table header.
const REQUIRED_KEYS = [
	'nav.consumers',
	'consumers.title', 'consumers.filterAll',
	'consumers.kind.user', 'consumers.kind.awg', 'consumers.kind.mtproto', 'consumers.kind.lan',
	'consumers.period.live', 'consumers.period.1h', 'consumers.period.24h', 'consumers.period.7d', 'consumers.period.30d',
	'consumers.nowAll', 'consumers.activeOf', 'consumers.periodTotal',
	'consumers.colWho', 'consumers.colType', 'consumers.colNow', 'consumers.colPeriod', 'consumers.colLive',
	'consumers.download', 'consumers.upload', 'consumers.peak', 'consumers.manage',
	'consumers.address', 'consumers.handshake', 'consumers.quota', 'consumers.expires', 'consumers.never',
	'consumers.state.disabled', 'consumers.state.quota', 'consumers.state.expired',
	'consumers.unavailable', 'consumers.noData', 'consumers.empty', 'consumers.loadFailed'
];

function lookup(obj: unknown, path: string): unknown {
	return path.split('.').reduce<unknown>((o, k) => (o && typeof o === 'object' ? (o as Record<string, unknown>)[k] : undefined), obj);
}

describe('i18n: consumers keys', () => {
	for (const key of REQUIRED_KEYS) {
		it(`${key} exists in en and ru`, () => {
			expect(typeof lookup(en, key)).toBe('string');
			expect(typeof lookup(ru, key)).toBe('string');
		});
	}
});
