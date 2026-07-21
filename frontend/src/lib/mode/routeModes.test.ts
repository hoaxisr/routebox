import { describe, it, expect } from 'vitest';
import {
	SECTIONS,
	isPathAllowed,
	inboundTypesFor,
	visibleInboundTypes
} from './routeModes';

describe('SECTIONS table (DRY single source)', () => {
	it('every section lists at least one valid mode', () => {
		for (const s of SECTIONS) {
			expect(s.modes.length).toBeGreaterThan(0);
			for (const m of s.modes) expect(['router', 'vps']).toContain(m);
		}
	});
	it('uses absolute paths and has no duplicate paths (drift guard)', () => {
		const seen = new Set<string>();
		for (const s of SECTIONS) {
			expect(s.path.startsWith('/')).toBe(true);
			expect(seen.has(s.path)).toBe(false);
			seen.add(s.path);
		}
	});
});

describe('isPathAllowed — additivity / router = full UI', () => {
	it('router mode allows EVERY router-governed section (gating only subtracts in vps)', () => {
		// Additivity: every section whose modes include 'router' must be reachable
		// in router mode. (The panel-only sections — /config/users, /monitor/users —
		// are asserted separately below; they are deliberately NOT router-reachable.)
		for (const s of SECTIONS) {
			if (s.modes.includes('router')) {
				expect(isPathAllowed(s.path, 'router')).toBe(true);
			}
		}
	});
	it('Overview "/" is allowed in BOTH modes (redirect target cannot loop)', () => {
		expect(isPathAllowed('/', 'router')).toBe(true);
		expect(isPathAllowed('/', 'vps')).toBe(true);
	});
});

describe('isPathAllowed — vps subtracts router-only', () => {
	it('router-only paths are blocked in vps', () => {
		expect(isPathAllowed('/config/clients', 'vps')).toBe(false);
		expect(isPathAllowed('/config/subscriptions', 'vps')).toBe(false);
		expect(isPathAllowed('/monitor/traffic', 'vps')).toBe(false);
		expect(isPathAllowed('/monitor/breakdown', 'vps')).toBe(false);
		expect(isPathAllowed('/monitor/proxies', 'vps')).toBe(false);
		expect(isPathAllowed('/monitor/route-inspector', 'vps')).toBe(false);
	});
	it('router-only paths are allowed in router', () => {
		expect(isPathAllowed('/config/clients', 'router')).toBe(true);
		expect(isPathAllowed('/monitor/traffic', 'router')).toBe(true);
	});
	it('panel-only Users is blocked in router, allowed in vps', () => {
		expect(isPathAllowed('/config/users', 'router')).toBe(false);
		expect(isPathAllowed('/config/users', 'vps')).toBe(true);
	});
	it('AWG server page is shared: allowed in BOTH modes (router restricts it to sing-box in-page)', () => {
		expect(isPathAllowed('/config/awg', 'vps')).toBe(true);
		expect(isPathAllowed('/config/awg', 'router')).toBe(true);
	});
	it('shared paths are allowed in both modes', () => {
		for (const p of ['/config/endpoints', '/config/outbounds', '/config/inbounds', '/config/dns', '/config/routes', '/config/rule-sets', '/config/domains', '/config/app', '/config/settings', '/config/updates', '/monitor/logs', '/monitor/connections']) {
			expect(isPathAllowed(p, 'router')).toBe(true);
			expect(isPathAllowed(p, 'vps')).toBe(true);
		}
	});
});

describe('isPathAllowed — unknown / login / nesting safety', () => {
	it('unlisted paths are allowed in both modes (fail-open; never block what we do not govern)', () => {
		expect(isPathAllowed('/login', 'vps')).toBe(true);
		expect(isPathAllowed('/login', 'router')).toBe(true);
		expect(isPathAllowed('/totally/unknown', 'vps')).toBe(true);
		expect(isPathAllowed('/some/future/page', 'router')).toBe(true);
	});
	it('matches nested sub-paths by longest matching section prefix', () => {
		expect(isPathAllowed('/config/clients/1.2.3.4', 'vps')).toBe(false);
		expect(isPathAllowed('/config/users/123', 'router')).toBe(false);
		expect(isPathAllowed('/config/dns/anything', 'vps')).toBe(true);
	});
	it('"/" does not prefix-swallow everything', () => {
		expect(isPathAllowed('/config/clients', 'vps')).toBe(false);
	});
});

describe('inboundTypesFor — picker by mode (add-new only)', () => {
	it('router gets LAN/client types in order', () => {
		expect(inboundTypesFor('router')).toEqual(['tun', 'mixed', 'socks', 'http']);
	});
	it('vps gets server types in order', () => {
		expect(inboundTypesFor('vps')).toEqual(['vless', 'trojan', 'naive', 'hysteria2', 'mieru']);
	});
	it('returns a fresh array each call (callers may not mutate shared state)', () => {
		const a = inboundTypesFor('router');
		const b = inboundTypesFor('router');
		expect(a).not.toBe(b);
		expect(a).toEqual(b);
	});
	it('vps offers trojan, router does not (drift guard)', () => {
		expect(inboundTypesFor('vps')).toContain('trojan');
		expect(inboundTypesFor('router')).not.toContain('trojan');
	});
});

describe('visibleInboundTypes — edit-safety', () => {
	it('add-new in vps shows only server types', () => {
		expect(visibleInboundTypes('vps')).toEqual(['vless', 'trojan', 'naive', 'hysteria2', 'mieru']);
	});
	it('add-new in router shows only LAN types', () => {
		expect(visibleInboundTypes('router')).toEqual(['tun', 'mixed', 'socks', 'http']);
	});
	it('editing a vless inbound in router keeps vless selectable', () => {
		expect(visibleInboundTypes('router', 'vless')).toEqual(['tun', 'mixed', 'socks', 'http', 'vless']);
	});
	it('editing a tun inbound in vps keeps tun selectable', () => {
		expect(visibleInboundTypes('vps', 'tun')).toEqual(['vless', 'trojan', 'naive', 'hysteria2', 'mieru', 'tun']);
	});
	it('does not duplicate a current type already in the list', () => {
		expect(visibleInboundTypes('vps', 'vless')).toEqual(['vless', 'trojan', 'naive', 'hysteria2', 'mieru']);
		expect(visibleInboundTypes('router', 'tun')).toEqual(['tun', 'mixed', 'socks', 'http']);
	});
	it('empty/undefined currentType adds nothing', () => {
		expect(visibleInboundTypes('vps', '')).toEqual(['vless', 'trojan', 'naive', 'hysteria2', 'mieru']);
		expect(visibleInboundTypes('vps', undefined)).toEqual(['vless', 'trojan', 'naive', 'hysteria2', 'mieru']);
	});
});

// Drift guard: the sidebar (+layout.svelte) hand-encodes router-only/panel-only
// link visibility as {#if $routerMode} / {#if $panelMode}. SECTIONS is the
// single source of truth the redirect guard uses. If a single-mode section is
// added/changed here without updating the sidebar {#if}, the nav and the guard
// drift (a link shown but bounced, or hidden but reachable). These assertions
// fail loudly so the sidebar gets updated deliberately.
describe('single-mode classification (sidebar drift guard)', () => {
	const singleMode = (m: 'router' | 'vps') =>
		SECTIONS.filter((s) => s.modes.length === 1 && s.modes[0] === m)
			.map((s) => s.path)
			.sort();

	it('router-only sections are exactly these (update +layout.svelte {#if $routerMode} if this changes)', () => {
		expect(singleMode('router')).toEqual([
			'/config/clients',
			'/config/subscriptions',
			'/monitor/breakdown',
			'/monitor/proxies',
			'/monitor/route-inspector',
			'/monitor/traffic'
		]);
	});

	it('panel-only sections are exactly these (update +layout.svelte {#if $panelMode} if this changes)', () => {
		expect(singleMode('vps')).toEqual(['/config/users', '/monitor/users']);
	});
});
