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

	it('off clears all awg3 fields', () => {
		const o = PRESETS.off();
		expect(o.content_padding_addition).toBe('');
		expect(o.reject_after_time).toBe('');
		expect(o.max_handshake_attempts).toBe('');
	});
});
