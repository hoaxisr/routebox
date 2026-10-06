import { redirect } from '@sveltejs/kit';

// Folded into /monitor/consumers (#109); old bookmarks land there.
export function load() {
	redirect(307, '/monitor/consumers');
}
