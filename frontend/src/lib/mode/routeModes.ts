// Single source of truth for mode-aware navigation, route guarding, and the
// inbound type picker. The sidebar, the soft-redirect guard, and InboundForm ALL
// import from here so visibility and reachability can never drift.
//
// Fail-safe contract: router mode is the full UI minus the panel-only sections
// (Users, per-user monitor). vps mode SUBTRACTS the router-only sections and
// adds the panel-only ones. The AWG server page is shared (both modes); router
// mode restricts it to the sing-box backend in the page itself. Anything NOT
// listed here is allowed everywhere.

export type Mode = 'router' | 'vps';

export interface Section {
	/** Absolute route path (no trailing slash). Matched by longest prefix. */
	path: string;
	/** Modes in which this section is visible/reachable. */
	modes: Mode[];
}

const BOTH: Mode[] = ['router', 'vps'];
const ROUTER_ONLY: Mode[] = ['router'];
const PANEL_ONLY: Mode[] = ['vps'];

// Shared sections are listed too so tests can assert router=full UI over the
// whole governed set and so the table doubles as documentation. Order does not
// matter; matching is by longest path prefix.
export const SECTIONS: Section[] = [
	// Shared (visible in both modes)
	{ path: '/', modes: BOTH },
	{ path: '/config/endpoints', modes: BOTH },
	{ path: '/config/outbounds', modes: BOTH },
	{ path: '/config/inbounds', modes: BOTH },
	{ path: '/config/dns', modes: BOTH },
	{ path: '/config/rule-sets', modes: BOTH },
	{ path: '/config/domains', modes: BOTH },
	{ path: '/config/routes', modes: BOTH },
	{ path: '/config/app', modes: BOTH },
	{ path: '/config/settings', modes: BOTH },
	{ path: '/config/updates', modes: BOTH },
	{ path: '/monitor/logs', modes: BOTH },
	{ path: '/monitor/connections', modes: BOTH },
	{ path: '/config/awg', modes: BOTH },

	// Panel-only
	{ path: '/config/users', modes: PANEL_ONLY },
	{ path: '/monitor/users', modes: PANEL_ONLY },

	// Router-only
	{ path: '/config/clients', modes: ROUTER_ONLY },
	{ path: '/config/subscriptions', modes: ROUTER_ONLY },
	{ path: '/monitor/traffic', modes: ROUTER_ONLY },
	{ path: '/monitor/breakdown', modes: ROUTER_ONLY },
	{ path: '/monitor/proxies', modes: ROUTER_ONLY },
	{ path: '/monitor/route-inspector', modes: ROUTER_ONLY }
];

// matchSection returns the most specific (longest-path) section governing `path`,
// or undefined if none governs it. A section governs `path` when path === s.path
// or path is a child segment of it. The exact-or-child rule prevents '/' from
// swallowing everything and prevents '/config/clients' matching '/config/clientsX'.
function matchSection(path: string): Section | undefined {
	let best: Section | undefined;
	for (const s of SECTIONS) {
		const governs = path === s.path || path.startsWith(s.path === '/' ? '/' : s.path + '/');
		if (!governs) continue;
		if (s.path === '/' && path !== '/') continue; // '/' only governs itself
		if (!best || s.path.length > best.path.length) best = s;
	}
	return best;
}

/**
 * isPathAllowed reports whether `path` may be shown/reached in `mode`.
 * Fail-open: an unlisted path is allowed in every mode (we never block what we do
 * not govern — keeps auth redirects, /login, and future routes working).
 */
export function isPathAllowed(path: string, mode: Mode): boolean {
	const s = matchSection(path);
	if (!s) return true;
	return s.modes.includes(mode);
}

// Inbound type picker options by mode. Router = client/LAN types; vps = server
// protocols. This filters ONLY the add-new chooser. Each call returns a fresh
// array so callers can never mutate shared state.
const ROUTER_INBOUND_TYPES = ['tun', 'mixed', 'socks', 'http'] as const;
const VPS_INBOUND_TYPES = ['vless', 'trojan', 'naive', 'hysteria2', 'mieru'] as const;

export function inboundTypesFor(mode: Mode): string[] {
	return mode === 'vps' ? [...VPS_INBOUND_TYPES] : [...ROUTER_INBOUND_TYPES];
}

/**
 * visibleInboundTypes returns the picker options for `mode`, always INCLUDING
 * `currentType` even if it is not in the mode's add-list. This is the edit-safety
 * rule: a config hand-edited to contain e.g. `vless` on a router, or `tun` on a
 * vps, must still be selectable/editable; only the new-type choices are filtered.
 * Empty/undefined `currentType` adds nothing; an already-present type is not
 * duplicated.
 */
export function visibleInboundTypes(mode: Mode, currentType?: string): string[] {
	const base = inboundTypesFor(mode);
	if (currentType && !base.includes(currentType)) return [...base, currentType];
	return base;
}
