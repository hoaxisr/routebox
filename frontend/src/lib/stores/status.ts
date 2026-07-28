import { writable, derived } from 'svelte/store';
import type { ProcessStatus } from '$lib/types';
import { api } from '$lib/api/client';

// Last known /api/status payload. Deliberately not a poller of its own: the
// Dashboard already polls on a timer and publishes here, and everything else
// only needs the value it read at page load plus a refresh after acting on it.
const statusStore = writable<ProcessStatus | null>(null);

/**
 * Fetches /api/status once and publishes it to subscribers.
 * Returns the fetched status so callers that need it inline don't have to read
 * the store back. Errors propagate — callers decide what a failed poll means.
 */
export async function refreshStatus(): Promise<ProcessStatus> {
	const status = await api.getStatus();
	statusStore.set(status);
	return status;
}

export const processStatus = { subscribe: statusStore.subscribe };

/**
 * Which config file RouteBox, the systemd unit and the live process point at,
 * plus the two verdicts derived from them. Null before the first poll.
 */
export const configPaths = derived(statusStore, ($s) => $s?.config_paths ?? null);

/**
 * True while the backend refuses writes because the config file is not
 * writable. Defaults to false before the first poll: guessing "read-only" from
 * a missing status would disable every save button on a healthy install.
 */
export const configReadOnly = derived(statusStore, ($s) => $s?.config_read_only === true);

/** The config path RouteBox could not write. Empty string unless read-only. */
export const configReadOnlyPath = derived(statusStore, ($s) => $s?.config_read_only_path ?? '');

/**
 * Every file RouteBox currently cannot write — the sing-box config among them,
 * plus its own state files (settings, panel users, subscriptions, known clients,
 * AWG peer secrets). They live in different directories and can be mounted
 * separately, so this is a list and not a flag: telling the user that "something"
 * is read-only is not an instruction, and naming the files is.
 */
export const readOnlyPaths = derived(statusStore, ($s) => $s?.read_only_paths ?? []);

/**
 * True while anything RouteBox persists is unwritable. This is what the header
 * badge stands for. Save buttons keep using the narrower `configReadOnly`: an
 * unwritable users.toml must not grey out config editing.
 */
export const anyReadOnly = derived(readOnlyPaths, ($paths) => $paths.length > 0);
