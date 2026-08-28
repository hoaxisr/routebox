import { describe, it, expect } from 'vitest';
import type { Outbound } from '$lib/types';
import { groupTags, isGroupOnlyChain } from './chainLabel';

const ob = (tag: string, type: string): Outbound => ({ tag, type }) as Outbound;

describe('groupTags', () => {
	it('collects only the types that pick a member at dial time', () => {
		const tags = groupTags([
			ob('auto', 'urltest'),
			ob('manual', 'selector'),
			ob('d1', 'direct'),
			ob('vl', 'vless'),
			ob('wg', 'wireguard')
		]);
		expect([...tags].sort()).toEqual(['auto', 'manual']);
	});

	it('is empty when the list has no groups at all', () => {
		expect(groupTags([ob('d1', 'direct')]).size).toBe(0);
		expect(groupTags([]).size).toBe(0);
	});
});

describe('isGroupOnlyChain', () => {
	const groups = new Set(['auto', 'manual']);

	it('flags a chain that is a group and nothing else', () => {
		expect(isGroupOnlyChain(['auto'], groups)).toBe(true);
		expect(isGroupOnlyChain('auto', groups)).toBe(true);
	});

	it('leaves a complete chain alone', () => {
		expect(isGroupOnlyChain(['d1', 'auto'], groups)).toBe(false);
		expect(isGroupOnlyChain('d1 → auto', groups)).toBe(false);
	});

	// A lone outbound is a complete answer: the traffic went straight through it.
	it('leaves a lone non-group outbound alone', () => {
		expect(isGroupOnlyChain(['direct'], groups)).toBe(false);
		expect(isGroupOnlyChain('awg-2-ee', groups)).toBe(false);
	});

	it('says nothing about empty or placeholder chains', () => {
		expect(isGroupOnlyChain([], groups)).toBe(false);
		expect(isGroupOnlyChain('', groups)).toBe(false);
		expect(isGroupOnlyChain('-', groups)).toBe(false);
	});

	// With no outbound list loaded nothing is claimed, rather than everything
	// being marked or a stale list mislabelling a renamed tag.
	it('claims nothing without a group list', () => {
		expect(isGroupOnlyChain(['auto'], new Set())).toBe(false);
	});
});
