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

// Smallest S1-S4 padding the fork accepts with header protection (AWG 3.0) on.
// Below it the endpoint is rejected outright, so a preset that draws under it is
// one the operator simply cannot enable — the DNS preset drew S4 from 4-10 and
// so could never be enabled with header protection at all.
const S_MIN = 12;

// rs draws an S padding, never below the floor. Every S value goes through it so
// widening a range later cannot quietly reintroduce an unenablable preset.
const rs = (lo: number, hi: number) => r(Math.max(lo, S_MIN), Math.max(hi, S_MIN));

// regenerate S2 until S1+56 != S2 (mirrors the backend guard)
function sPair(s1lo: number, s1hi: number, s2lo: number, s2hi: number): [number, number] {
	const s1 = rs(s1lo, s1hi);
	let s2 = rs(s2lo, s2hi);
	while (s1 + 56 === s2) s2 = rs(s2lo, s2hi);
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
	reject_after_time: '', keepalive_timeout: '', max_handshake_attempts: '',
	// AWG 3.1 flags are opt-in, never turned ON by a preset — but "off" has to
	// clear them, or switching the preset to off would leave obfuscation
	// half-on and keep breaking pre-3.1 clients.
	random_trailers: false, disable_cookies: false
};

// Upper content-padding bound per preset — the louder the cover traffic, the more
// padding it can hide behind.
const CPA_HI: Record<string, number> = { dns: 48, web: 64, stealth: 80 };

export const PRESETS: Record<string, () => AwgObf> = {
	off: () => ({ jc: 0, jmin: 0, jmax: 0, s1: 0, s2: 0, s3: 0, s4: 0, h1: '', h2: '', h3: '', h4: '', ...AWG3_OFF }),
	dns: () => {
		const [h1, h2, h3, h4] = randH();
		const [s1, s2] = sPair(97, 107, 17, 27);
		return { jc: r(3, 5), jmin: r(5, 15), jmax: r(45, 55), s1, s2, s3: rs(16, 26), s4: rs(12, 22), h1, h2, h3, h4, ...awg3(CPA_HI.dns) };
	},
	web: () => {
		const [h1, h2, h3, h4] = randH();
		const [s1, s2] = sPair(30, 80, 30, 80);
		return { jc: r(5, 8), jmin: r(30, 80), jmax: r(100, 250), s1, s2, s3: rs(15, 32), s4: rs(10, 20), h1, h2, h3, h4, ...awg3(CPA_HI.web) };
	},
	stealth: () => {
		const [h1, h2, h3, h4] = randH();
		const [s1, s2] = sPair(15, 150, 15, 150);
		return { jc: r(4, 16), jmin: r(50, 256), jmax: r(300, 1000), s1, s2, s3: rs(8, 64), s4: rs(6, 31), h1, h2, h3, h4, ...awg3(CPA_HI.stealth) };
	}
};

export type AwgVersion = '2.0' | '3.0' | '3.1';
export const AWG_VERSIONS: AwgVersion[] = ['2.0', '3.0', '3.1'];

// awgVersion reports the version the CLIENT ends up with. Header protection is
// the gate: the backend strips every awg3 field — the 3.1 flags included — from
// an export whose header key is empty (manager.go clientConfFor), so filled
// CPA/RAT with protection off still hands out a plain 2.0 config. Deriving this
// from the fields alone would label 3.0 what AmneziaWG calls 2.0 (#60, #64).
export function awgVersion(obf: AwgObf, headerProtection: boolean): AwgVersion {
	if (!headerProtection) return '2.0';
	return obf.random_trailers || obf.disable_cookies ? '3.1' : '3.0';
}

// applyVersion reshapes obf for the target version, drawing the awg3 params from
// the preset's own range when they are absent and never redrawing ones already
// set. The caller owns header_protection — awgVersion reads it back.
export function applyVersion(obf: AwgObf, v: AwgVersion, preset: string): AwgObf {
	if (v === '2.0') return { ...obf, ...AWG3_OFF };
	const filled = obf.content_padding_addition ? obf : { ...obf, ...awg3(CPA_HI[preset] ?? CPA_HI.web) };
	return {
		...filled,
		// Both flags ARE 3.1 here: picking the version turns the pair on, and
		// dropping below it clears the pair. Leaving DisableCookies opt-in gave
		// exported 3.1 configs one of the version's two new parameters.
		random_trailers: v === '3.1',
		disable_cookies: v === '3.1'
	};
}
