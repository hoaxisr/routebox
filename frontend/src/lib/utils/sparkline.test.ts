import { describe, it, expect } from 'vitest';
import { sparklinePath } from './sparkline';

describe('sparklinePath', () => {
	it('returns empty string for <2 points', () => {
		expect(sparklinePath([], 100, 20)).toBe('');
		expect(sparklinePath([5], 100, 20)).toBe('');
	});

	it('maps a flat series to a horizontal line at mid-height', () => {
		const d = sparklinePath([4, 4, 4], 100, 20);
		expect(d.startsWith('M 0 10')).toBe(true);
		expect(d).toContain('L 100 10');
	});

	it('maps min to bottom and max to top', () => {
		// width=100,height=20: first(0)→y=20(bottom), last(10)→y=0(top)
		expect(sparklinePath([0, 10], 100, 20)).toBe('M 0 20 L 100 0');
	});
});
