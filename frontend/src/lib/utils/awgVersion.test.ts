import { describe, it, expect } from 'vitest';
import type { Endpoint } from '$lib/types';
import { getAWGVersion } from './awgVersion';

const ep = (extra: Record<string, unknown>): Endpoint =>
	({ type: 'awg', tag: 'e', address: ['10.0.0.2/32'], private_key: 'k', peers: [], ...extra }) as unknown as Endpoint;

describe('getAWGVersion', () => {
	it('reports plain WG when no obfuscation is configured', () => {
		expect(getAWGVersion(ep({}))).toBe('WG');
	});

	it('reads junk packets alone as 1.0, i1-i5 as 1.5, s3/s4 as 2.0', () => {
		expect(getAWGVersion(ep({ jc: 4, jmin: 40, jmax: 70 }))).toBe('AWG 1.0');
		expect(getAWGVersion(ep({ jc: 4, i1: '<b 0x01>' }))).toBe('AWG 1.5');
		expect(getAWGVersion(ep({ jc: 4, s3: 10, s4: 10 }))).toBe('AWG 2.0');
	});

	it('reads an awg3-only parameter as 3.0 even alongside 2.0 parameters', () => {
		expect(getAWGVersion(ep({ s3: 10, header_protection_key: 'k' }))).toBe('AWG 3.0');
		expect(getAWGVersion(ep({ content_padding_addition: 5 }))).toBe('AWG 3.0');
		expect(getAWGVersion(ep({ rekey_after_time: '2m' }))).toBe('AWG 3.0');
	});

	it('treats an h1-h4 range as 2.0', () => {
		expect(getAWGVersion(ep({ h1: '1-5' }))).toBe('AWG 2.0');
	});

	// A range is "lo-hi", not "has a minus in it somewhere". A negative header
	// type is a typo, and reading it as 2.0 would label the endpoint off one.
	it('does not read a negative or malformed h1-h4 as a range', () => {
		expect(getAWGVersion(ep({ jc: 4, h1: -1 }))).toBe('AWG 1.0');
		expect(getAWGVersion(ep({ jc: 4, h1: '1-' }))).toBe('AWG 1.0');
		expect(getAWGVersion(ep({ jc: 4, h1: '-' }))).toBe('AWG 1.0');
		expect(getAWGVersion(ep({ h1: ' 1-5 ' }))).toBe('AWG 2.0');
	});

	// sing-box takes h1-h4 as integers; only AWG 3.0 widens them to "lo-hi".
	// Reading a number as a string used to throw and take the whole page down.
	it('survives numeric h1-h4 from a hand-written or imported config', () => {
		expect(getAWGVersion(ep({ jc: 4, h1: 1, h2: 2, h3: 3, h4: 4 }))).toBe('AWG 1.0');
		expect(getAWGVersion(ep({ h1: 0 }))).toBe('WG');
	});
});
