import { writable, derived, get } from 'svelte/store';
import { api } from '$lib/api/client';
import type { Mode } from '$lib/mode/routeModes';

// FAIL-SAFE: the UI starts in router mode = the full existing UI. We only ever
// move to 'vps' after a successful settings read that EXPLICITLY says so. A
// failed/missing/loading read leaves us here, so an admin can never be locked out
// of their own panel and existing router installs see no change. Gating only ever
// subtracts in vps mode (see routeModes.isPathAllowed).
export const serverMode = writable<Mode>('router');

export const panelMode = derived(serverMode, ($m) => $m === 'vps');

export const routerMode = derived(serverMode, ($m) => $m === 'router');

// behindFront: this install came up from the out-of-the-box bootstrap, where
// every inbound sits behind the front on 443. The front relays unauthenticated
// traffic as raw bytes and never carries the client's address to sing-box, so
// every connection reports the loopback as its source. The monitor uses this to
// say so instead of showing 127.0.0.1 as if it were data.
//
// Fail-safe the same way as the mode: false until a settings read explicitly
// says otherwise, and false is the existing UI every other install gets.
export const behindFront = writable<boolean>(false);

// normalize: only the exact string 'vps' becomes vps; everything else (missing,
// empty, garbage, undefined) is router. Keeps the conservative contract central.
function normalize(raw: unknown): Mode {
	return raw === 'vps' ? 'vps' : 'router';
}

// fetchAndApply reads settings and sets the store; on ANY failure it leaves the
// store at its current value (last-good), never throwing to the caller.
async function fetchAndApply(): Promise<void> {
	try {
		const res = await api.getSettings();
		serverMode.set(normalize(res?.settings?.server?.mode));
		behindFront.set(res?.settings?.server?.bootstrapped === true);
	} catch (e) {
		// keep last-good mode (router on first load) — see fail-safe contract
		console.error('Failed to load server mode:', e);
	}
}

// loadMode: initial load (called from +layout onMount). Idempotent.
export async function loadMode(): Promise<void> {
	await fetchAndApply();
}

// refreshMode: re-read after a mode change in Settings so nav/dashboard update
// without a full reload.
export async function refreshMode(): Promise<void> {
	await fetchAndApply();
}

// snapshot for non-reactive consumers.
export function currentMode(): Mode {
	return get(serverMode);
}

// test-only reset.
export function __resetMode(): void {
	serverMode.set('router');
	behindFront.set(false);
}
