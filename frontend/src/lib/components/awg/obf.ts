import type { AwgObf } from '$lib/types';

// Four AmneziaWG H1-H4 header magics as ranges "lo-hi" (span >=1000), one per
// quadrant of [5, 2^31-1] so they never overlap. AWG randomizes the header per
// packet within the range — a single fixed value would be a static signature.
export function randH(): string[] {
	const MAX = 2147483647;
	const q = Math.floor((MAX - 5) / 4);
	return [0, 1, 2, 3].map((i) => {
		const start = 5 + i * q;
		const span = 1000 + Math.floor(Math.random() * 9000); // 1000..9999
		const lo = start + Math.floor(Math.random() * (q - span));
		return `${lo}-${lo + span}`;
	});
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

export const OBF_J = ['jc', 'jmin', 'jmax'] as const;
export const OBF_S = ['s1', 's2', 's3', 's4'] as const;
export const OBF_NUM = [...OBF_J, ...OBF_S] as const;
export const OBF_STR = ['h1', 'h2', 'h3', 'h4'] as const;

// AWG3 device params. Ranges centred on amneziawg-go stock (rekey_after=120,
// reject_after=180, rekey_timeout=5, keepalive=10, max_handshake=18); the fork
// picks a per-session value inside each range so the timing is not the stock
// WireGuard fingerprint. cpaHi = upper content-padding bound (scales per preset).
// ponytail: bounds chosen so rekey_after (<=160) is always < reject_after (>=180)
// — the session must rekey before it is rejected; keep that invariant if you widen.
function awg3(cpaHi: number): Partial<AwgObf> {
	const ratLo = r(110, 125);
	return {
		content_padding_addition: `0-${cpaHi}`,
		rekey_after_time: `${ratLo}-${ratLo + r(20, 35)}`, // <=160
		rekey_timeout: '5',
		reject_after_time: `${r(180, 195)}-${r(200, 220)}`, // >160
		keepalive_timeout: `${r(10, 15)}-${r(20, 30)}`,
		max_handshake_attempts: '18'
	};
}

// off clears every AWG3 field so switching to it turns obfuscation fully off.
const AWG3_OFF: Partial<AwgObf> = {
	content_padding_addition: '', rekey_after_time: '', rekey_timeout: '',
	reject_after_time: '', keepalive_timeout: '', max_handshake_attempts: ''
};

export const PRESETS: Record<string, () => AwgObf> = {
	off: () => ({ jc: 0, jmin: 0, jmax: 0, s1: 0, s2: 0, s3: 0, s4: 0, h1: '', h2: '', h3: '', h4: '', ...AWG3_OFF }),
	dns: () => {
		const [h1, h2, h3, h4] = randH();
		const [s1, s2] = sPair(97, 107, 17, 27);
		return { jc: r(3, 5), jmin: r(5, 15), jmax: r(45, 55), s1, s2, s3: r(16, 26), s4: r(4, 10), h1, h2, h3, h4, ...awg3(48) };
	},
	web: () => {
		const [h1, h2, h3, h4] = randH();
		const [s1, s2] = sPair(30, 80, 30, 80);
		return { jc: r(5, 8), jmin: r(30, 80), jmax: r(100, 250), s1, s2, s3: r(15, 32), s4: r(10, 20), h1, h2, h3, h4, ...awg3(64) };
	},
	stealth: () => {
		const [h1, h2, h3, h4] = randH();
		const [s1, s2] = sPair(15, 150, 15, 150);
		return { jc: r(4, 16), jmin: r(50, 256), jmax: r(300, 1000), s1, s2, s3: r(8, 64), s4: r(6, 31), h1, h2, h3, h4, ...awg3(80) };
	}
};
