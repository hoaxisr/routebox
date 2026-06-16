import type { AwgObf } from '$lib/types';

// Four distinct random magic-header values (uint32-ish range), returned as strings.
export function randH(): string[] {
	const s = new Set<number>();
	while (s.size < 4) s.add(5 + Math.floor(Math.random() * 2147483640));
	return [...s].map(String);
}

export const PRESETS: Record<'off' | 'standard' | 'mobile', () => AwgObf> = {
	off: () => ({ jc: 0, jmin: 0, jmax: 0, s1: 0, s2: 0, s3: 0, s4: 0, h1: '', h2: '', h3: '', h4: '' }),
	standard: () => {
		const [h1, h2, h3, h4] = randH();
		return { jc: 4, jmin: 40, jmax: 70, s1: 0, s2: 0, s3: 0, s4: 0, h1, h2, h3, h4 };
	},
	mobile: () => {
		const [h1, h2, h3, h4] = randH();
		return { jc: 8, jmin: 40, jmax: 70, s1: 88, s2: 176, s3: 0, s4: 0, h1, h2, h3, h4 };
	}
};

// Numeric vs string obf keys kept as separate `as const` tuples so binds stay
// monomorphic (bind:value={form.obf[k]} resolves to number / string, no union).
export const OBF_NUM = ['jc', 'jmin', 'jmax', 's1', 's2', 's3', 's4'] as const;
export const OBF_STR = ['h1', 'h2', 'h3', 'h4'] as const;
