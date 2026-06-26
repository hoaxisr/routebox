export type ExpiryState = 'none' | 'active' | 'suspended';

/** Three-state status. Boundary is strict: now >= expiresAt is suspended. */
export function expiryStatus(expiresAt: number, now: number): ExpiryState {
	if (expiresAt === 0) return 'none';
	return now < expiresAt ? 'active' : 'suspended';
}

/** Unix seconds -> "YYYY-MM-DD" (UTC). 0 -> "". */
export function unixToDateInput(unix: number): string {
	if (!unix) return '';
	return new Date(unix * 1000).toISOString().slice(0, 10);
}

/** "YYYY-MM-DD" -> UTC-midnight unix seconds. "" -> 0. */
export function dateInputToUnix(value: string): number {
	if (!value) return 0;
	return Math.floor(new Date(value + 'T00:00:00Z').getTime() / 1000);
}

/** now + days (whole days), in unix seconds. For the "+30d" / "+90d" presets. */
export function presetExpiry(days: number, now: number): number {
	return now + days * 86400;
}
