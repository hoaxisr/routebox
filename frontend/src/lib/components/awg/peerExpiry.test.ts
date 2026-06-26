import { describe, it, expect } from 'vitest';
import { expiryStatus, unixToDateInput, dateInputToUnix, presetExpiry } from './peerExpiry';

const NOW = 1_700_000_000; // fixed reference

describe('expiryStatus', () => {
	it('none when 0', () => expect(expiryStatus(0, NOW)).toBe('none'));
	it('active when in the future', () => expect(expiryStatus(NOW + 100, NOW)).toBe('active'));
	it('suspended at the exact boundary', () => expect(expiryStatus(NOW, NOW)).toBe('suspended'));
	it('suspended when past', () => expect(expiryStatus(NOW - 1, NOW)).toBe('suspended'));
});

describe('unixToDateInput / dateInputToUnix', () => {
	it('formats a unix ts to a UTC date string', () => {
		expect(unixToDateInput(1_700_000_000)).toBe('2023-11-14');
	});
	it('returns empty for 0', () => expect(unixToDateInput(0)).toBe(''));
	it('parses a date string to UTC-midnight unix', () => {
		expect(dateInputToUnix('2023-11-14')).toBe(1_699_920_000);
	});
	it('returns 0 for empty', () => expect(dateInputToUnix('')).toBe(0));
});

describe('presetExpiry', () => {
	it('adds whole days', () => expect(presetExpiry(30, NOW)).toBe(NOW + 30 * 86400));
});
