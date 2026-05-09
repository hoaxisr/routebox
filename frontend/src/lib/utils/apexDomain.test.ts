import { describe, it, expect } from 'vitest';
import { apexDomain } from './apexDomain';

describe('apexDomain', () => {
	it.each([
		['googlevideo.com', 'googlevideo.com'],
		['r3---sn-uxa.googlevideo.com', 'googlevideo.com'],
		['a.b.c.googlevideo.com', 'googlevideo.com'],
		['bbc.co.uk', 'bbc.co.uk'],
		['news.bbc.co.uk', 'bbc.co.uk'],
		['1.2.3.4', '1.2.3.4'],
		['::1', '::1'],
		['2001:db8::1', '2001:db8::1'],
		['localhost', 'localhost'],
		['', ''],
		['-', '-']
	])('apexDomain(%j) → %j', (input, want) => {
		expect(apexDomain(input)).toBe(want);
	});
});
