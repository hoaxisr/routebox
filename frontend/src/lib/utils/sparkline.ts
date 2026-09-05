// sparklinePath builds an SVG path for values across [0,width]x[0,height], with
// the largest value at the top (y=0) and smallest at the bottom (y=height). A
// flat series renders at vertical center. Returns '' for <2 points. PURE.
export function sparklinePath(values: number[], width: number, height: number): string {
	if (values.length < 2) return '';
	const min = Math.min(...values);
	const max = Math.max(...values);
	const span = max - min;
	const stepX = width / (values.length - 1);
	const y = (v: number) => (span === 0 ? height / 2 : height - ((v - min) / span) * height);
	return values.map((v, i) => `${i === 0 ? 'M' : 'L'} ${i * stepX} ${y(v)}`).join(' ');
}

// areaPaths builds the line and the closed area under it for a series drawn
// against a FIXED max (baseline y=height, max at y=0) so several strips share a
// scale, or one strip keeps its scale while values stream in. Values above max
// clamp to the top. PURE.
export function areaPaths(
	values: number[],
	max: number,
	width: number,
	height: number
): { line: string; area: string } {
	if (values.length < 2) return { line: '', area: '' };
	const stepX = width / (values.length - 1);
	const y = (v: number) => (max <= 0 ? height : height - (Math.min(v, max) / max) * height);
	const pts = values.map((v, i) => `${i * stepX} ${y(v)}`);
	const line = `M ${pts[0]} ` + pts.slice(1).map((p) => `L ${p}`).join(' ');
	const area = `${line} L ${width} ${height} L 0 ${height} Z`;
	return { line, area };
}

// splitUnit separates "5.39 KB/s" into its number and unit so the number can
// be set large and the unit small. PURE.
export function splitUnit(s: string): { value: string; unit: string } {
	const i = s.indexOf(' ');
	if (i < 0) return { value: s, unit: '' };
	return { value: s.slice(0, i), unit: s.slice(i + 1) };
}
