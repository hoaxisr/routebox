import type { PanelBinding } from '$lib/types';

/** Pill label for a binding: PROTOCOL, plus " · name" when name adds information. */
export function pillLabel(binding: Pick<PanelBinding, 'protocol' | 'name'>): string {
	const proto = binding.protocol.toUpperCase();
	const name = binding.name?.trim() ?? '';
	if (name && name.toLowerCase() !== binding.protocol.toLowerCase()) {
		return `${proto} · ${name}`;
	}
	return proto;
}

/** First binding, or null when there are none. */
export function defaultBinding<T extends { inbound_tag: string }>(bindings: T[]): T | null {
	return bindings.length > 0 ? bindings[0] : null;
}

/**
 * Fetch one connection's share link. Never throws: on failure returns the error
 * message (or `fallbackMsg` for a non-Error throw) so the caller can render it inline.
 * `getLink` is injected (the component passes `api.getUserLink`) to keep this testable.
 */
export async function loadConnectionLink(
	getLink: (id: string, tag: string, host: string) => Promise<{ link: string }>,
	userId: string,
	tag: string,
	host: string,
	fallbackMsg: string
): Promise<{ link: string; error: string }> {
	try {
		const { link } = await getLink(userId, tag, host);
		return { link, error: '' };
	} catch (e) {
		return { link: '', error: e instanceof Error ? e.message : fallbackMsg };
	}
}
