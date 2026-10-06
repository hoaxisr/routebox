import { describe, it, expect } from 'vitest';
import { parseAsn, formatAsn, splitAsnInput, agoParts } from './asn';

describe('asn helpers', () => {
	it('parses AS numbers in the accepted spellings', () => {
		expect(parseAsn('13335')).toBe(13335);
		expect(parseAsn('AS13335')).toBe(13335);
		expect(parseAsn(' as32934 ')).toBe(32934);
		expect(parseAsn('4294967295')).toBe(4294967295);
		for (const bad of ['', '0', 'AS', 'AS-1', '13335x', '4294967296', '1.5']) expect(parseAsn(bad)).toBeNull();
		expect(formatAsn(13335)).toBe('AS13335');
	});
	it('splits pasted lists', () => {
		expect(splitAsnInput('AS13335, 32934\nAS15169  ')).toEqual(['AS13335', '32934', 'AS15169']);
	});
	it('formats relative age', () => {
		const now = 100_000;
		expect(agoParts(0, now)).toBeNull();
		expect(agoParts(now - 30, now)).toEqual({ n: 1, unit: 'm' });
		expect(agoParts(now - 600, now)).toEqual({ n: 10, unit: 'm' });
		expect(agoParts(now - 3 * 3600, now)).toEqual({ n: 3, unit: 'h' });
		expect(agoParts(now - 72 * 3600, now)).toEqual({ n: 3, unit: 'd' });
	});
});
