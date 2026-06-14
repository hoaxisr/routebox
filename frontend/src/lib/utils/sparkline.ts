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
