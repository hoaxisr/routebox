/**
 * The mieru server's `listen_ports`: extra "lo-hi" ranges it binds alongside
 * `listen_port`. The panel edits them as one free-text field, so this is where
 * what people type becomes what the config gets — and where a typo is reported
 * instead of silently dropped.
 *
 * A bare port is NOT accepted here even though it looks harmless: the fork's
 * range parser reads "%d-%d" and rejects anything else, so a lone number would
 * fail at apply time with a much worse message. That is what listen_port is for.
 */
export function parseMieruListenPorts(text: string): { ranges: string[]; invalid: string[] } {
	const ranges: string[] = [];
	const invalid: string[] = [];
	const seen = new Set<string>();
	for (const rawToken of text.split(/[\s,]+/)) {
		const token = rawToken.trim();
		if (!token) continue;
		const parts = token.split('-');
		const nums = parts.map((p) => (/^\d+$/.test(p) ? parseInt(p, 10) : NaN));
		const ok =
			parts.length === 2 &&
			nums.every((n) => Number.isInteger(n) && n >= 1 && n <= 65535) &&
			nums[0] <= nums[1];
		if (!ok) {
			invalid.push(token);
			continue;
		}
		const canonical = `${nums[0]}-${nums[1]}`;
		if (seen.has(canonical)) continue;
		seen.add(canonical);
		ranges.push(canonical);
	}
	return { ranges, invalid };
}

/** The stored ranges as the text field shows them. */
export function formatMieruListenPorts(ranges: string[] | undefined): string {
	return (ranges ?? []).join(', ');
}
