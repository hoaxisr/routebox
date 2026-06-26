import { describe, it, expect } from 'vitest';
import { pillLabel, defaultBinding, loadConnectionLink } from './share-links';

describe('pillLabel', () => {
	it('upper-cases the protocol', () => {
		expect(pillLabel({ protocol: 'vless', name: '' })).toBe('VLESS');
	});
	it('appends a distinct name', () => {
		expect(pillLabel({ protocol: 'trojan', name: 'Phone' })).toBe('TROJAN · Phone');
	});
	it('omits the name when it just echoes the protocol', () => {
		expect(pillLabel({ protocol: 'naive', name: 'naive' })).toBe('NAIVE');
	});
	it('omits an empty name', () => {
		expect(pillLabel({ protocol: 'hysteria2', name: '' })).toBe('HYSTERIA2');
	});
});

describe('defaultBinding', () => {
	it('returns the first binding', () => {
		expect(defaultBinding([{ inbound_tag: 'a' }, { inbound_tag: 'b' }])).toEqual({
			inbound_tag: 'a'
		});
	});
	it('returns null for an empty list', () => {
		expect(defaultBinding([])).toBeNull();
	});
});

describe('loadConnectionLink', () => {
	it('returns the link and empty error on success', async () => {
		const getLink = async () => ({ link: 'vless://u@h:443#x' });
		expect(await loadConnectionLink(getLink, 'u-1', 'in-vless', 'h', 'fallback')).toEqual({
			link: 'vless://u@h:443#x',
			error: ''
		});
	});
	it('passes the right args through to getLink', async () => {
		let seen: string[] = [];
		const getLink = async (id: string, tag: string, host: string) => {
			seen = [id, tag, host];
			return { link: 'ok' };
		};
		await loadConnectionLink(getLink, 'u-1', 'in-trojan', 'vpn.example.com', 'fallback');
		expect(seen).toEqual(['u-1', 'in-trojan', 'vpn.example.com']);
	});
	it('maps an Error rejection to its message', async () => {
		const getLink = async () => {
			throw new Error('reality key invalid');
		};
		expect(await loadConnectionLink(getLink, 'u-1', 'in-x', 'h', 'fallback')).toEqual({
			link: '',
			error: 'reality key invalid'
		});
	});
	it('uses the fallback message for a non-Error rejection', async () => {
		const getLink = async () => {
			throw 'boom';
		};
		expect(await loadConnectionLink(getLink, 'u-1', 'in-x', 'h', 'fallback')).toEqual({
			link: '',
			error: 'fallback'
		});
	});
});
