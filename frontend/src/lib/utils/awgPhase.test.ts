import { describe, it, expect } from 'vitest';
import en from '../i18n/locales/en.json';
import ru from '../i18n/locales/ru.json';
import { ENABLE_IN_FLIGHT_PHASES, isEnableInFlight } from './awgPhase';

describe('isEnableInFlight', () => {
	it('is true for every phase the orchestrator passes through', () => {
		for (const p of ENABLE_IN_FLIGHT_PHASES) {
			expect(isEnableInFlight(p)).toBe(true);
		}
	});

	// A terminal phase reading as in-flight would leave the spinner up forever and
	// the Enable button disabled with it.
	it('is false for terminal phases and for nothing at all', () => {
		for (const p of ['idle', 'ready', 'failed', '', undefined, null]) {
			expect(isEnableInFlight(p)).toBe(false);
		}
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
