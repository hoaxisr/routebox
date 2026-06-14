import { describe, it, expect } from 'vitest';
import { buildSubUrl } from './subscription-url';

describe('buildSubUrl', () => {
	it('forces https when host has no scheme', () => {
		expect(buildSubUrl('vpn.example.com', 'tok')).toBe('https://vpn.example.com/sub/tok');
	});
	it('keeps an explicit scheme', () => {
		expect(buildSubUrl('http://10.0.0.1:8080', 'tok')).toBe('http://10.0.0.1:8080/sub/tok');
	});
	it('returns empty without host or token', () => {
		expect(buildSubUrl('', 'tok')).toBe('');
		expect(buildSubUrl('vpn.example.com', '')).toBe('');
	});
	it('strips a trailing slash on the host', () => {
		expect(buildSubUrl('vpn.example.com/', 'tok')).toBe('https://vpn.example.com/sub/tok');
	});
	it('url-encodes the token', () => {
		expect(buildSubUrl('vpn.example.com', 'a/b')).toContain('/sub/a%2Fb');
	});
});
