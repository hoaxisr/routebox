import { writable, derived } from 'svelte/store';
import type { SingBoxVersion } from '$lib/types';
import { api } from '$lib/api/client';

const versionStore = writable<SingBoxVersion | null>(null);

export async function loadVersion() {
	try {
		const data = await api.getVersion();
		versionStore.set(data);
	} catch (e) {
		console.error('Failed to load sing-box version:', e);
	}
}

export const singboxVersion = { subscribe: versionStore.subscribe };

export const featureFlags = derived(versionStore, ($v) => $v?.features ?? {});

export function hasFeature(name: string): boolean {
	let flags: Record<string, boolean> = {};
	featureFlags.subscribe((f) => (flags = f))();
	return flags[name] ?? false;
}
