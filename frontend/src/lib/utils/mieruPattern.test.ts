import { describe, it, expect } from 'vitest';
import {
	encodeTrafficPattern,
	decodeTrafficPattern,
	rotationValue,
	rotationParts,
	isEmptyPattern,
	type TrafficPattern
} from './mieruPattern';

// Golden vectors produced by mieru v3.35.0 itself:
//   trafficpattern.Encode(&appctlpb.TrafficPattern{...})
const GOLDEN: [string, TrafficPattern, string][] = [
	['empty', {}, ''],
	['seed only', { seed: 12345, unlockAll: true }, 'CLlgEAE='],
	[
		'full',
		{
			seed: 7,
			unlockAll: false,
			tcpFragment: { enable: true, maxSleepMs: 10 },
			nonce: { type: 1, applyToAllUDPPacket: true, minLen: 6, maxLen: 8 },
			padding: { maxMiddlePaddingLen: 64, maxEndPaddingLen: 128 },
			lowEntropy: { mode: 1, maskRotation: 7 }
		},
		'CAcQABoECAEQCiIICAEQARgGIAgqBQhAEIABMgQIARAH'
	],
	[
		'explicit zeros keep presence',
		{
			tcpFragment: { enable: false, maxSleepMs: 0 },
			padding: { maxMiddlePaddingLen: 0, maxEndPaddingLen: 0 },
			lowEntropy: { mode: 0, maskRotation: 0 }
		},
		'GgQIABAAKgQIABAAMgQIABAA'
	],
	[
		'fixed nonce prefixes',
		{ nonce: { type: 3, customHexStrings: ['00010203', '04050607'] } },
		'IhYIAyoIMDAwMTAyMDMqCDA0MDUwNjA3'
	],
	['left rotation', { lowEntropy: { mode: 4, maskRotation: 112 } }, 'MgQIBBBw'],
	['max seed', { seed: 2147483647 }, 'CP////8H']
];

describe('mieru traffic pattern codec', () => {
	for (const [name, pattern, encoded] of GOLDEN) {
		it(`encodes ${name} like mieru does`, () => {
			expect(encodeTrafficPattern(pattern)).toBe(encoded);
		});
		it(`round-trips ${name}`, () => {
			expect(encodeTrafficPattern(decodeTrafficPattern(encoded)!)).toBe(encoded);
		});
	}

	it('decodes into editable fields', () => {
		const p = decodeTrafficPattern('CAcQABoECAEQCiIICAEQARgGIAgqBQhAEIABMgQIARAH')!;
		expect(p.seed).toBe(7);
		expect(p.unlockAll).toBe(false);
		expect(p.tcpFragment).toEqual({ enable: true, maxSleepMs: 10 });
		expect(p.nonce).toEqual({ type: 1, applyToAllUDPPacket: true, minLen: 6, maxLen: 8 });
		expect(p.padding).toEqual({ maxMiddlePaddingLen: 64, maxEndPaddingLen: 128 });
		expect(p.lowEntropy).toEqual({ mode: 1, maskRotation: 7 });
	});

	it('drops submessages with nothing set', () => {
		expect(encodeTrafficPattern({ nonce: {}, padding: {}, lowEntropy: {} })).toBe('');
		expect(isEmptyPattern({ tcpFragment: {} })).toBe(true);
		expect(isEmptyPattern({ tcpFragment: { enable: false } })).toBe(false);
	});

	it('returns an empty pattern for a blank string', () => {
		expect(decodeTrafficPattern('  ')).toEqual({});
	});

	it('rejects garbage instead of silently dropping settings', () => {
		expect(decodeTrafficPattern('not base64 !!')).toBeNull();
		expect(decodeTrafficPattern('////')).toBeNull(); // valid base64, invalid protobuf
	});

	it('keeps unknown fields from a newer mieru out of the way', () => {
		// field 99 varint appended to the "seed only" vector: decodes, seed survives.
		const withUnknown = btoa(atob('CLlgEAE=') + String.fromCharCode(0xd8, 0x06, 0x2a));
		expect(decodeTrafficPattern(withUnknown)).toEqual({ seed: 12345, unlockAll: true });
	});
});

describe('mask rotation', () => {
	it('maps direction and steps onto the enum', () => {
		expect(rotationValue('none', 5)).toBe(0);
		expect(rotationValue('right', 7)).toBe(7);
		expect(rotationValue('left', 7)).toBe(112);
		expect(rotationValue('left', 15)).toBe(240);
	});

	it('clamps steps into 1..15', () => {
		expect(rotationValue('right', 0)).toBe(1);
		expect(rotationValue('right', 99)).toBe(15);
	});

	it('reads the enum back', () => {
		expect(rotationParts(0)).toEqual({ direction: 'none', steps: 1 });
		expect(rotationParts(7)).toEqual({ direction: 'right', steps: 7 });
		expect(rotationParts(112)).toEqual({ direction: 'left', steps: 7 });
	});
});
