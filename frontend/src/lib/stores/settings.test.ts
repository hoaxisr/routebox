import { describe, it, expect } from 'vitest';
import { formatBytes, formatSpeed, speedUnit } from './settings';

describe('formatBytes / formatSpeed', () => {
	it('keeps a unit for fractions of a byte (#99)', () => {
		expect(formatBytes(0.4958)).toBe('0.5 B');
		expect(formatSpeed(0.05)).toBe('0.1 B/s');
		speedUnit.set('bits');
		expect(formatSpeed(0.05)).toBe('0.4 bps');
		speedUnit.set('bytes');
	});

	it('picks the usual units', () => {
		expect(formatBytes(0)).toBe('0 B');
		expect(formatBytes(1536)).toBe('1.5 KB');
		expect(formatSpeed(2 * 1024 * 1024)).toBe('2.0 MB/s');
	});

	it('steps up a unit at 1000, so a number is never four digits wide (#110)', () => {
		expect(formatBytes(999 * 1024)).toBe('999.0 KB');
		expect(formatSpeed(1023.4 * 1024)).toBe('1.0 MB/s');
		expect(formatBytes(1000)).toBe('1.0 KB');
	});
});
