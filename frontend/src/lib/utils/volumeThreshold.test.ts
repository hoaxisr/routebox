import { describe, it, expect } from 'vitest';
import { bytesFromUnit, splitBytes, PRESETS } from './volumeThreshold';

describe('bytesFromUnit', () => {
	it.each([
		[1, 'KB' as const, 1024],
		[10, 'MB' as const, 10 * 1024 * 1024],
		[100, 'MB' as const, 100 * 1024 * 1024],
		[1, 'GB' as const, 1024 * 1024 * 1024],
		[0.5, 'GB' as const, 512 * 1024 * 1024],
		[0, 'MB' as const, 0]
	])('bytesFromUnit(%d, %s) → %d', (value, unit, want) => {
		expect(bytesFromUnit(value, unit)).toBe(want);
	});

	it.each([
		[-5, 'MB' as const],
		[NaN, 'MB' as const],
		[Infinity, 'MB' as const]
	])('bytesFromUnit(%j, %s) → 0 (clamped)', (value, unit) => {
		expect(bytesFromUnit(value, unit)).toBe(0);
	});
});

describe('splitBytes', () => {
	it.each([
		[0, { value: 0, unit: 'MB' as const }],
		[1024, { value: 1, unit: 'KB' as const }],
		[1024 * 1024, { value: 1, unit: 'MB' as const }],
		[10 * 1024 * 1024, { value: 10, unit: 'MB' as const }],
		[100 * 1024 * 1024, { value: 100, unit: 'MB' as const }],
		[1024 * 1024 * 1024, { value: 1, unit: 'GB' as const }],
		[1536 * 1024 * 1024, { value: 1.5, unit: 'GB' as const }],
		[512, { value: 0.5, unit: 'KB' as const }]
	])('splitBytes(%d) → %j', (bytes, want) => {
		expect(splitBytes(bytes)).toEqual(want);
	});
});

describe('PRESETS', () => {
	it('contains Off + 3 size presets in ascending order', () => {
		expect(PRESETS.map(p => p.label)).toEqual(['Off', '10 MB', '100 MB', '1 GB']);
		expect(PRESETS.map(p => p.value)).toEqual([
			0,
			10 * 1024 * 1024,
			100 * 1024 * 1024,
			1024 * 1024 * 1024
		]);
	});
});
