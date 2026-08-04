import { describe, it, expect } from 'vitest';
import en from './locales/en.json';
import ru from './locales/ru.json';

// Keys of the mieru traffic-pattern editor. Every label here sits on a toggle
// button or a numeric field, so a missing key shows up as a raw dotted path in
// the middle of a form.
const REQUIRED_KEYS = [
	'mieruPattern.title',
	'mieruPattern.intro',
	'mieruPattern.off',
	'mieruPattern.on',
	'mieruPattern.unparsed',
	'mieruPattern.seed',
	'mieruPattern.seedHint',
	'mieruPattern.autoPlaceholder',
	'mieruPattern.regenerate',
	'mieruPattern.unlockAll',
	'mieruPattern.unlockAllLabel',
	'mieruPattern.unlockAllHint',
	'mieruPattern.showAdvanced',
	'mieruPattern.hideAdvanced',
	'mieruPattern.auto',
	'mieruPattern.onValue',
	'mieruPattern.offValue',
	'mieruPattern.tcpFragment',
	'mieruPattern.tcpFragmentHint',
	'mieruPattern.maxSleepMs',
	'mieruPattern.nonce',
	'mieruPattern.nonceHint',
	'mieruPattern.nonceType.auto',
	'mieruPattern.nonceType.random',
	'mieruPattern.nonceType.printable',
	'mieruPattern.nonceType.printableSubset',
	'mieruPattern.nonceType.fixed',
	'mieruPattern.customHex',
	'mieruPattern.customHexHint',
	'mieruPattern.minLen',
	'mieruPattern.maxLen',
	'mieruPattern.applyToAllUdp',
	'mieruPattern.padding',
	'mieruPattern.paddingHint',
	'mieruPattern.middlePadding',
	'mieruPattern.endPadding',
	'mieruPattern.lowEntropy',
	'mieruPattern.lowEntropyHint',
	'mieruPattern.lowEntropyMode.auto',
	'mieruPattern.lowEntropyMode.off',
	'mieruPattern.lowEntropyMode.32',
	'mieruPattern.lowEntropyMode.40',
	'mieruPattern.lowEntropyMode.48',
	'mieruPattern.lowEntropyMode.56',
	'mieruPattern.maskRotation',
	'mieruPattern.rotation.none',
	'mieruPattern.rotation.right',
	'mieruPattern.rotation.left',
	'mieruPattern.rotationSteps'
];

function resolve(obj: unknown, path: string): unknown {
	return path.split('.').reduce<unknown>((acc, k) => (acc as Record<string, unknown>)?.[k], obj);
}

describe('mieru traffic pattern i18n keys', () => {
	for (const [name, dict] of [
		['en', en],
		['ru', ru]
	] as const) {
		for (const key of REQUIRED_KEYS) {
			it(`${name} has ${key}`, () => {
				const value = resolve(dict, key);
				expect(typeof value).toBe('string');
				expect(value).not.toBe('');
			});
		}
	}
});
