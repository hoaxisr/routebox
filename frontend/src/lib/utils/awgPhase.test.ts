import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import en from '../i18n/locales/en.json';
import ru from '../i18n/locales/ru.json';
import { ENABLE_IN_FLIGHT_PHASES, isEnableInFlight } from './awgPhase';

describe('isEnableInFlight', () => {
	// A terminal phase reading as in-flight would leave the spinner up forever and
	// the Enable button disabled with it.
	it('is false for terminal phases and for nothing at all', () => {
		for (const p of ['idle', 'ready', 'failed', '', undefined, null]) {
			expect(isEnableInFlight(p)).toBe(false);
		}
	});

	it('is true for a phase string the backend actually emits', () => {
		expect(isEnableInFlight('installing')).toBe(true);
	});
});

// The list here is a copy of Go's awg.EnablePhase, and nothing but this test
// connects them: rename a constant on the backend and the spinner would simply
// stop appearing, with every other test still green. Asserting against the Go
// source is the only check that can fail for that.
describe('the phase strings match backend/internal/awg/enable.go', () => {
	// vitest runs with the repo's `frontend` as cwd.
	const source = readFileSync(resolve('../backend/internal/awg/enable.go'), 'utf8');
	// PhaseValidating EnablePhase = "validating"
	const declared = [...source.matchAll(/Phase\w+\s+EnablePhase\s+=\s+"([\w-]+)"/g)].map((m) => m[1]);

	it('finds the declarations at all (guards the regex itself)', () => {
		expect(declared.length).toBeGreaterThanOrEqual(7);
		expect(declared).toContain('idle');
	});

	it('every in-flight phase is a phase the backend can emit', () => {
		for (const p of ENABLE_IN_FLIGHT_PHASES) {
			expect(declared).toContain(p);
		}
	});

	// The terminal ones are the whole remainder: a phase added on the backend and
	// forgotten here would leave the panel showing nothing while work is going on.
	it('accounts for every declared phase, in-flight or terminal', () => {
		const terminal = ['idle', 'ready', 'failed'];
		expect([...declared].sort()).toEqual([...ENABLE_IN_FLIGHT_PHASES, ...terminal].sort());
	});
});

// Each in-flight phase is shown by name while the user waits; a missing key would
// render the raw path during the one moment the page is supposed to explain itself.
describe('every in-flight phase has a label in both locales', () => {
	const keyOf = (phase: string) =>
		'phase' + phase.split('-').map((w) => w[0].toUpperCase() + w.slice(1)).join('');

	for (const phase of ENABLE_IN_FLIGHT_PHASES) {
		it(phase, () => {
			const key = keyOf(phase);
			expect(typeof (en.awg as Record<string, unknown>)[key]).toBe('string');
			expect(typeof (ru.awg as Record<string, unknown>)[key]).toBe('string');
		});
	}

	it('the install hint exists too — it is what stops people reloading', () => {
		expect(typeof (en.awg as Record<string, unknown>).phaseInstallingHint).toBe('string');
		expect(typeof (ru.awg as Record<string, unknown>).phaseInstallingHint).toBe('string');
	});
});
