import type { Endpoint } from '$lib/types';

/**
 * Which AWG generation an endpoint's obfuscation parameters describe:
 * 'AWG 3.0', 'AWG 2.0', 'AWG 1.5', 'AWG 1.0', or plain 'WG'.
 *
 * The config is not ours to trust: sing-box takes h1–h4 as integers, and only
 * AWG 3.0 widens them to a "lo-hi" string. A hand-written or imported config
 * therefore routinely carries numbers here, so every value is read as text
 * rather than assumed to be a string.
 */
export function getAWGVersion(ep: Endpoint): string {
	const hasI = !!(ep.i1 || ep.i2 || ep.i3 || ep.i4 || ep.i5);
	const hasS3S4 = !!(ep.s3 || ep.s4);
	const hasJunk = !!(ep.jc || ep.jmin || ep.jmax);
	// AWG3 signature: header protection, content padding, or rekey-after-time.
	const hasAwg3 = !!(ep.header_protection_key || ep.content_padding_addition || ep.rekey_after_time);

	// h1-h4 hold a range ("lo-hi") rather than a single header type.
	const isRange = (val: unknown) => val !== undefined && val !== null && String(val).includes('-');
	const hasHRanges = isRange(ep.h1) || isRange(ep.h2) || isRange(ep.h3) || isRange(ep.h4);

	// AWG 3.0: awg3-only params present (also carries s/h, so check before 2.0)
	if (hasAwg3) return 'AWG 3.0';

	// AWG 2.0: s3-s4 configured (with or without i1-i5) OR h1-h4 as ranges
	if (hasS3S4 || hasHRanges) return 'AWG 2.0';

	// AWG 1.5: i1-i5 configured without s3-s4
	if (hasI) return 'AWG 1.5';

	// AWG 1.0: only junk packets (jc/jmin/jmax) configured
	if (hasJunk) return 'AWG 1.0';

	return 'WG';
}
