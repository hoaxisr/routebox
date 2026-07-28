import { describe, it, expect } from 'vitest';
import { parseMieruListenPorts, formatMieruListenPorts } from './mieruPorts';

// Issue #37: the server form only had a numeric listen_port, so a mieru server
// could not be told to bind a range at all — while the client side has accepted
// ranges all along. The field takes free text and has to survive what people
// actually type: commas, spaces, trailing separators.
describe('parseMieruListenPorts', () => {
	it('accepts a single range', () => {
		expect(parseMieruListenPorts('25010-25012')).toEqual({ ranges: ['25010-25012'], invalid: [] });
	});

	it.each([
		['25010-25012,26000-26100'],
		['25010-25012, 26000-26100'],
		['25010-25012 26000-26100'],
		[' 25010-25012 ,  26000-26100 , '],
		['25010-25012\n26000-26100']
	])('splits %p on commas and whitespace', (input) => {
		expect(parseMieruListenPorts(input).ranges).toEqual(['25010-25012', '26000-26100']);
	});

	it('is empty for empty input', () => {
		expect(parseMieruListenPorts('')).toEqual({ ranges: [], invalid: [] });
		expect(parseMieruListenPorts('   ,  ')).toEqual({ ranges: [], invalid: [] });
	});

	// The fork rejects a bare port in listen_ports — that is what listen_port is
	// for — so a lone number must be reported rather than silently widened.
	it('rejects a bare port', () => {
		expect(parseMieruListenPorts('8443')).toEqual({ ranges: [], invalid: ['8443'] });
	});

	it.each([
		['25012-25010', 'reversed'],
		['0-100', 'below 1'],
		['1-70000', 'above 65535'],
		['abc', 'not numeric'],
		['1-2-3', 'too many parts'],
		['-100', 'missing low'],
		['100-', 'missing high']
	])('rejects %p (%s)', (input) => {
		const got = parseMieruListenPorts(input);
		expect(got.ranges).toEqual([]);
		expect(got.invalid).toEqual([input.trim()]);
	});

	it('keeps the good ones and reports the bad ones', () => {
		expect(parseMieruListenPorts('25010-25012, nope, 26000-26100')).toEqual({
			ranges: ['25010-25012', '26000-26100'],
			invalid: ['nope']
		});
	});

	it('de-duplicates identical ranges', () => {
		expect(parseMieruListenPorts('2000-2100, 2000-2100').ranges).toEqual(['2000-2100']);
	});
});

describe('formatMieruListenPorts', () => {
	it('round-trips through the text field', () => {
		const text = formatMieruListenPorts(['25010-25012', '26000-26100']);
		expect(parseMieruListenPorts(text).ranges).toEqual(['25010-25012', '26000-26100']);
	});

	it('is empty for nothing', () => {
		expect(formatMieruListenPorts([])).toBe('');
		expect(formatMieruListenPorts(undefined)).toBe('');
	});
});
