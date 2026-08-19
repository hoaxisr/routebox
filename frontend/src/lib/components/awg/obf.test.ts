import { describe, it, expect } from 'vitest';
import { PRESETS, applyVersion, awgVersion } from './obf';

// The obf presets randomise awg3 timers per call; the one invariant that MUST hold
// on every draw is rekey_after < reject_after (a session must rekey before it is
// rejected, else the tunnel drops). Sample enough draws to catch a bad range.
const hi = (range: string) => Math.max(...range.split('-').map(Number));
const lo = (range: string) => Math.min(...range.split('-').map(Number));

describe('awg3 preset timers', () => {
	for (const name of ['dns', 'web', 'stealth']) {
		it(`${name}: rekey_after always < reject_after`, () => {
			for (let i = 0; i < 200; i++) {
				const o = PRESETS[name]();
				expect(hi(o.rekey_after_time!)).toBeLessThan(lo(o.reject_after_time!));
			}
		});
	}

	// Header protection (AWG 3.0) makes the fork reject any S1-S4 below 12, so a
	// generated profile that draws one is a preset the operator cannot enable —
	// the DNS preset's S4 was 4-10 and so never could be.
	for (const name of ['dns', 'web', 'stealth']) {
		it(`${name}: every S padding is at least 12`, () => {
			for (let i = 0; i < 500; i++) {
				const o = PRESETS[name]();
				for (const k of ['s1', 's2', 's3', 's4'] as const) {
					expect(o[k], `${name}.${k} draw ${i}`).toBeGreaterThanOrEqual(12);
				}
			}
		});

		// The other rule the fork enforces on the pair: an init packet padded by
		// S1 must not come out the size of a response padded by S2.
		it(`${name}: s1 + 56 never equals s2`, () => {
			for (let i = 0; i < 500; i++) {
				const o = PRESETS[name]();
				expect(o.s1 + 56).not.toBe(o.s2);
			}
		});
	}

	it('off clears all awg3 fields', () => {
		const o = PRESETS.off();
		expect(o.content_padding_addition).toBe('');
		expect(o.reject_after_time).toBe('');
		expect(o.max_handshake_attempts).toBe('');
	});
});

describe('awg protocol version', () => {
	// Header protection is the gate the BACKEND uses (manager.go clientConfFor
	// strips every awg3 field when the header key is empty), so filled CPA/RAT
	// with HP off is still a 2.0 config in the client's hands (#60, #64, #76).
	it('reports 2.0 while header protection is off, whatever is filled in', () => {
		const o = PRESETS.web();
		expect(awgVersion(o, false)).toBe('2.0');
		expect(awgVersion({ ...o, random_trailers: true }, false)).toBe('2.0');
	});

	it('reports 3.0 with header protection, 3.1 once trailers are on', () => {
		const o = PRESETS.web();
		expect(awgVersion(o, true)).toBe('3.0');
		expect(awgVersion({ ...o, random_trailers: true }, true)).toBe('3.1');
	});

	it('2.0 clears the awg3 fields and both 3.1 flags', () => {
		const o = applyVersion({ ...PRESETS.stealth(), random_trailers: true, disable_cookies: true }, '2.0', 'stealth');
		expect(o.content_padding_addition).toBe('');
		expect(o.reject_after_time).toBe('');
		expect(o.random_trailers).toBe(false);
		expect(o.disable_cookies).toBe(false);
		expect(o.jc).toBeGreaterThan(0); // J/S/H survive — that is the 2.0 obfuscation
	});

	it('3.0 fills the awg3 fields and keeps the 3.1 flags off', () => {
		const o = applyVersion({ ...PRESETS.off(), jc: 4 }, '3.0', 'web');
		expect(o.content_padding_addition).not.toBe('');
		expect(o.max_handshake_attempts).not.toBe('');
		expect(o.random_trailers).toBe(false);
		expect(awgVersion(o, true)).toBe('3.0');
	});

	it('3.1 turns BOTH new flags on without redrawing existing awg3 values', () => {
		const base = PRESETS.dns();
		const o = applyVersion(base, '3.1', 'dns');
		expect(o.content_padding_addition).toBe(base.content_padding_addition);
		expect(o.random_trailers).toBe(true);
		expect(o.disable_cookies).toBe(true);
		expect(awgVersion(o, true)).toBe('3.1');
	});

	// Either flag alone is a 3.1-only key, so the version badge has to read 3.1
	// or the form goes on offering a "3.0" that the backend exports as 3.1 (#74).
	it('reads 3.1 from either flag alone', () => {
		const o = PRESETS.web();
		expect(awgVersion({ ...o, disable_cookies: true }, true)).toBe('3.1');
	});
});

// ponytail: source-text check, not a render test. The awg3.1 flags were invisible
// in the panel for a release because ServerSettingsForm forgot to forward
// awg31Available — Svelte drops an unpassed prop silently. Cheapest guard.
describe('awg3.1 flag gating', () => {
	it('ServerSettingsForm forwards awg31Available to ObfuscationControl', async () => {
		const src = (await import('./ServerSettingsForm.svelte?raw')).default;
		const tag = src.match(/<ObfuscationControl[^>]*>/)?.[0] ?? '';
		expect(tag).toContain('awg31Available');
	});

	// The preset name is the ONLY key the backend has for the client's CPS mimicry
	// (cps.Mimic(preset) — "custom" yields an empty set, so the .conf loses I1-I5).
	// Detaching the preset on every keystroke silently dropped the mimicry (#76).
	it('ObfuscationControl never rewrites the preset to "custom"', async () => {
		const src = (await import('./ObfuscationControl.svelte?raw')).default;
		expect(src).not.toContain("'custom'");
	});
});
