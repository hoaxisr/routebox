import { describe, it, expect } from 'vitest';
import { splitPortRanges, parsePortList, invalidPortRanges } from './portInputs';

describe('port inputs (#111)', () => {
	it('splits ranges on spaces, commas and newlines', () => {
		expect(splitPortRanges('50000:50099 19200:19400')).toEqual(['50000:50099', '19200:19400']);
		expect(splitPortRanges(' 1:2,\n3:4 ,5: ')).toEqual(['1:2', '3:4', '5:']);
	});
	it('parses a space-separated port list', () => {
		expect(parsePortList('443 8443, 80\n0 70000 x')).toEqual([443, 8443, 80]);
	});
	it('flags what sing-box rejects', () => {
		expect(invalidPortRanges(['1000:2000', ':3000', '4000:', '0:65535'])).toEqual([]);
		expect(invalidPortRanges([':', '443', '2000:1000', '1:70000', 'a:b', '50099 19200:19400'])).toEqual([
			':',
			'443',
			'2000:1000',
			'1:70000',
			'a:b',
			'50099 19200:19400'
		]);
	});
});
