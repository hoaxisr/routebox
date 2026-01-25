import { writable } from 'svelte/store';
import type { Toast } from '$lib/types';

function createNotificationStore() {
	const { subscribe, update } = writable<Toast[]>([]);

	function generateId(): string {
		return Math.random().toString(36).substring(2, 9);
	}

	function add(type: Toast['type'], message: string, duration = 5000) {
		const id = generateId();
		const toast: Toast = { id, type, message, duration };

		update((toasts) => [...toasts, toast]);

		if (duration > 0) {
			setTimeout(() => {
				remove(id);
			}, duration);
		}

		return id;
	}

	function remove(id: string) {
		update((toasts) => toasts.filter((t) => t.id !== id));
	}

	return {
		subscribe,
		success: (message: string, duration?: number) => add('success', message, duration),
		error: (message: string, duration?: number) => add('error', message, duration),
		warning: (message: string, duration?: number) => add('warning', message, duration),
		info: (message: string, duration?: number) => add('info', message, duration),
		remove,
		clear: () => update(() => [])
	};
}

export const notifications = createNotificationStore();
