import { describe, it, expect } from 'vitest';
import { PRESETS } from './obf';

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
