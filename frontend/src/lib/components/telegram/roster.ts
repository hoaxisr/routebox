import type { MtprotoClient } from '$lib/types';

export type ClientStatus = 'online' | 'offline' | 'disabled' | 'expired';

/**
 * clientStatus collapses the roster flags into the one word a row shows.
 *
 * Expired outranks disabled because the two coincide once the server's sweep
 * runs, and expired is the state that says what to do about it — extend the
 * date, rather than just switch the client back on.
 */
export function clientStatus(c: MtprotoClient, now: number): ClientStatus {
	// Inclusive, matching the server's Active(): at the deadline it is out.
	if (c.expires_at !== 0 && now >= c.expires_at) return 'expired';
	if (!c.enabled) return 'disabled';
	return c.online ? 'online' : 'offline';
}

/** Short relative phrase: "30d" when a day or more remains, else "5h". */
export function relExpiry(expiresAt: number, now: number): string {
	const s = Math.max(0, expiresAt - now);
	if (s >= 86400) return `${Math.floor(s / 86400)}d`;
	return `${Math.max(1, Math.floor(s / 3600))}h`;
}

/**
 * canShare reports whether a link can be built at all.
 *
 * Mirrors the backend's check. A link missing the masking domain or the public
 * host is well-formed and then fails silently inside Telegram, so the UI
 * withholds it and says what is missing instead of handing over a dud.
 */
export function canShare(maskingDomain: string, publicHost: string): boolean {
	return maskingDomain.trim() !== '' && publicHost.trim() !== '';
}
