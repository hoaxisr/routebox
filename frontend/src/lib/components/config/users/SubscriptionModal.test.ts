import { describe, it, expect } from 'vitest';
import { buildSubUrl, effectiveSubUrl } from './subscription-url';

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
	it('forces https on a scheme-less host but preserves the port', () => {
		expect(buildSubUrl('1.2.3.4:8080', 'tok')).toBe('https://1.2.3.4:8080/sub/tok');
	});
	it('preserves an uppercase scheme without double-prefixing', () => {
		expect(buildSubUrl('HTTPS://vpn.example.com', 'tok')).toMatch(/^HTTPS:\/\//);
	});
	it('collapses multiple trailing slashes', () => {
		expect(buildSubUrl('host//', 'tok')).toBe('https://host/sub/tok');
	});
	it('strips surrounding whitespace and control chars from the host', () => {
		expect(buildSubUrl(' vpn.example.com\n ', 'tok')).toBe('https://vpn.example.com/sub/tok');
	});
	it('returns empty when host normalizes to empty', () => {
		expect(buildSubUrl('  \n ', 'tok')).toBe('');
	});
});

describe('effectiveSubUrl (sticky-revoke gate)', () => {
	it('returns empty for a revoked user even with a non-empty token', () => {
		// load-bearing: token_disabled wins over a present token
		expect(effectiveSubUrl({ token: 'tok', token_disabled: true }, 'vpn.example.com')).toBe('');
	});
	it('returns the full URL for an active user', () => {
		expect(effectiveSubUrl({ token: 'tok', token_disabled: false }, 'vpn.example.com')).toBe(
			'https://vpn.example.com/sub/tok'
		);
	});
	it('returns empty for a pending user (no token, not disabled)', () => {
		expect(effectiveSubUrl({ token: '', token_disabled: false }, 'vpn.example.com')).toBe('');
	});
});
