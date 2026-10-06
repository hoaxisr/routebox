// Port and port-range fields of a route rule (#111). Entries may be separated
// by newlines, commas or spaces: "50000:50099 19200:19400" typed on one line
// used to reach sing-box as ONE range and fail Apply with "bad port range".

const SEP = /[\s,]+/;

// splitPortRanges returns the non-empty entries of a port-range field. PURE.
export function splitPortRanges(text: string): string[] {
	return text.split(SEP).filter(Boolean);
}

// parsePortList returns the valid ports of a port field (1..65535). PURE.
export function parsePortList(text: string): number[] {
	return text
		.split(SEP)
		.filter(Boolean)
		.map((s) => Number(s))
		.filter((n) => Number.isInteger(n) && n > 0 && n <= 65535);
}

// invalidPortRanges returns the entries sing-box would reject: a range is
// "start:end", ":end" or "start:", ports 0..65535, start not above end. PURE.
export function invalidPortRanges(entries: string[]): string[] {
	return entries.filter((e) => {
		const m = /^(\d{0,5}):(\d{0,5})$/.exec(e);
		if (!m || (!m[1] && !m[2])) return true;
		const lo = m[1] ? Number(m[1]) : 0;
		const hi = m[2] ? Number(m[2]) : 65535;
		return lo > 65535 || hi > 65535 || lo > hi;
	});
}
