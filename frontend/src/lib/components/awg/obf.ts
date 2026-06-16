import type { AwgObf } from '$lib/types';

// Distinct random header magics in [5, 2^31-1], min span 1000 (camouflage, not secrets).
export function randH(): string[] {
	const out: number[] = [];
	while (out.length < 4) {
		const v = 5 + Math.floor(Math.random() * 2147483640);
		if (out.every((o) => Math.abs(o - v) >= 1000)) out.push(v);
	}
	return out.map(String);
}

// inclusive random integer in [lo, hi]
const r = (lo: number, hi: number) => lo + Math.floor(Math.random() * (hi - lo + 1));

// regenerate S2 until S1+56 != S2 (mirrors the backend guard)
function sPair(s1lo: number, s1hi: number, s2lo: number, s2hi: number): [number, number] {
	const s1 = r(s1lo, s1hi);
	let s2 = r(s2lo, s2hi);
	while (s1 + 56 === s2) s2 = r(s2lo, s2hi);
	return [s1, s2];
}

export const OBF_NUM = ['jc', 'jmin', 'jmax', 's1', 's2', 's3', 's4'] as const;
export const OBF_STR = ['h1', 'h2', 'h3', 'h4'] as const;

export const PRESETS: Record<string, () => AwgObf> = {
	off: () => ({ jc: 0, jmin: 0, jmax: 0, s1: 0, s2: 0, s3: 0, s4: 0, h1: '', h2: '', h3: '', h4: '' }),
	dns: () => {
		const [h1, h2, h3, h4] = randH();
		const [s1, s2] = sPair(97, 107, 17, 27);
		return { jc: r(3, 5), jmin: r(5, 15), jmax: r(45, 55), s1, s2, s3: r(16, 26), s4: r(4, 10), h1, h2, h3, h4 };
	},
	web: () => {
		const [h1, h2, h3, h4] = randH();
		const [s1, s2] = sPair(30, 80, 30, 80);
		return { jc: r(5, 8), jmin: r(30, 80), jmax: r(100, 250), s1, s2, s3: r(15, 32), s4: r(10, 20), h1, h2, h3, h4 };
	},
	stealth: () => {
		const [h1, h2, h3, h4] = randH();
		const [s1, s2] = sPair(15, 150, 15, 150);
		return { jc: r(4, 16), jmin: r(50, 256), jmax: r(300, 1000), s1, s2, s3: r(8, 64), s4: r(6, 31), h1, h2, h3, h4 };
	}
};
