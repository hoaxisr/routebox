/** The proxy's listen address as two fields, which is how every other listener
 *  in the panel is edited (issue #62). */
export type ListenParts = {
	host: string;
	/** null when the stored value carries no port, or the field was cleared. */
	port: number | null;
};

/**
 * Splits a stored "host:port" into the two fields the form edits.
 *
 * IPv6 is the whole reason this is a function and not a `split(':')`: an
 * unbracketed "::" is a host with no port, while "[::]:9443" is a host AND a
 * port, and splitting on the last colon gets the first case wrong in a way that
 * silently moves the listener.
 */
export function splitListen(listen: string): ListenParts {
	const value = listen.trim();

	if (value === '') {
		return { host: '', port: null };
	}

	// Bracketed IPv6, with or without a port.
	if (value.startsWith('[')) {
		const close = value.indexOf(']');

		if (close !== -1) {
			const host = value.slice(1, close);
			const rest = value.slice(close + 1);

			return { host, port: rest.startsWith(':') ? toPort(rest.slice(1)) : null };
		}
	}

	const lastColon = value.lastIndexOf(':');

	if (lastColon === -1) {
		return { host: value, port: null };
	}

	// More than one colon and no brackets: a bare IPv6 address, not host:port.
	if (value.indexOf(':') !== lastColon) {
		return { host: value, port: null };
	}

	return { host: value.slice(0, lastColon), port: toPort(value.slice(lastColon + 1)) };
}

/** Rebuilds the stored form, bracketing an IPv6 host so it round-trips. */
export function joinListen(host: string, port: number | null): string {
	const trimmed = host.trim();

	if (port === null) {
		// Nothing sensible to append. Returning "host:null" would be persisted
		// verbatim and fail to bind; the bare host lets the server say so.
		return trimmed;
	}

	if (trimmed === '') {
		return `:${port}`;
	}

	const needsBrackets = trimmed.includes(':') && !trimmed.startsWith('[');

	return `${needsBrackets ? `[${trimmed}]` : trimmed}:${port}`;
}

function toPort(raw: string): number | null {
	if (!/^\d+$/.test(raw)) {
		return null;
	}

	return Number(raw);
}
