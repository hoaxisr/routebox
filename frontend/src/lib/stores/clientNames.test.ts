import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import { clientNames, applyClients, clientName } from './clientNames';

describe('clientNames store', () => {
	beforeEach(() => {
		clientNames.set(new Map());
	});

	it('stores only entries with non-empty name', () => {
		applyClients([
			{ ip: '1.1.1.1', name: 'A', note: '', first_seen: 0, last_seen: 0, online: true },
			{ ip: '2.2.2.2', name: '', note: '', first_seen: 0, last_seen: 0, online: false }
		]);
		const map = get(clientNames);
		expect(map.get('1.1.1.1')).toBe('A');
		expect(map.has('2.2.2.2')).toBe(false);
	});

	it('clientName returns name for IP if known', () => {
		applyClients([
			{ ip: '10.0.0.1', name: 'Pi', note: '', first_seen: 0, last_seen: 0, online: true }
		]);
		expect(clientName('10.0.0.1')).toBe('Pi');
		expect(clientName('10.0.0.2')).toBeUndefined();
	});

	it('applyClients replaces previous map (does not merge)', () => {
		applyClients([
			{ ip: '1.1.1.1', name: 'Old', note: '', first_seen: 0, last_seen: 0, online: false }
		]);
		applyClients([
			{ ip: '2.2.2.2', name: 'New', note: '', first_seen: 0, last_seen: 0, online: false }
		]);
		const map = get(clientNames);
		expect(map.has('1.1.1.1')).toBe(false);
		expect(map.get('2.2.2.2')).toBe('New');
	});

	it('clientName trims whitespace-only names as empty', () => {
		applyClients([
			{ ip: '3.3.3.3', name: '   ', note: '', first_seen: 0, last_seen: 0, online: false }
		]);
		expect(clientName('3.3.3.3')).toBeUndefined();
	});
});
