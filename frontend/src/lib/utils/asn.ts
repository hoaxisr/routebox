// Pure helpers for ASN rule sets (#103).

export const ASN_INTERVALS = [6, 12, 24, 168] as const;

export function parseAsn(s: string): number | null {
	let t = s.trim();
	if (/^as/i.test(t)) t = t.slice(2);
	if (!/^\d+$/.test(t)) return null;
	const n = Number(t);
	return n >= 1 && n <= 4294967295 ? n : null;
}

export const formatAsn = (n: number) => `AS${n}`;

export function splitAsnInput(s: string): string[] {
	return s.split(/[\s,]+/).map((x) => x.trim()).filter(Boolean);
}

export function agoParts(ts: number, now: number): { n: number; unit: 'm' | 'h' | 'd' } | null {
	if (!ts) return null;
	const sec = Math.max(0, now - ts);
	if (sec < 3600) return { n: Math.max(1, Math.floor(sec / 60)), unit: 'm' };
	if (sec < 48 * 3600) return { n: Math.floor(sec / 3600), unit: 'h' };
	return { n: Math.floor(sec / 86400), unit: 'd' };
}
