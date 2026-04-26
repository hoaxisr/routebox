import { writable, get } from 'svelte/store';
import type { ClientEntry } from '$lib/types';
import { api } from '$lib/api/client';

export const clientNames = writable<Map<string, string>>(new Map());

export function applyClients(list: ClientEntry[]): void {
	const map = new Map<string, string>();
	for (const c of list) {
		if (c.name && c.name.trim()) {
			map.set(c.ip, c.name);
		}
	}
	clientNames.set(map);
}

export function clientName(ip: string): string | undefined {
	return get(clientNames).get(ip);
}

export async function loadClientNames(): Promise<void> {
	try {
		const list = await api.listClients();
		applyClients(list);
	} catch {
		/* ignore — may run before backend is ready */
	}
}
