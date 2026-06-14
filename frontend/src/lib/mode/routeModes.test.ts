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
		// in router mode. (The lone panel-only section, /config/users, is asserted
		// separately below — it is deliberately NOT router-reachable.)
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
		expect(inboundTypesFor('vps')).toEqual(['vless', 'naive', 'hysteria2']);
	});
	it('returns a fresh array each call (callers may not mutate shared state)', () => {
		const a = inboundTypesFor('router');
		const b = inboundTypesFor('router');
		expect(a).not.toBe(b);
		expect(a).toEqual(b);
	});
});

describe('visibleInboundTypes — edit-safety', () => {
	it('add-new in vps shows only server types', () => {
		expect(visibleInboundTypes('vps')).toEqual(['vless', 'naive', 'hysteria2']);
	});
	it('add-new in router shows only LAN types', () => {
		expect(visibleInboundTypes('router')).toEqual(['tun', 'mixed', 'socks', 'http']);
	});
	it('editing a vless inbound in router keeps vless selectable', () => {
		expect(visibleInboundTypes('router', 'vless')).toEqual(['tun', 'mixed', 'socks', 'http', 'vless']);
	});
	it('editing a tun inbound in vps keeps tun selectable', () => {
		expect(visibleInboundTypes('vps', 'tun')).toEqual(['vless', 'naive', 'hysteria2', 'tun']);
	});
	it('does not duplicate a current type already in the list', () => {
		expect(visibleInboundTypes('vps', 'vless')).toEqual(['vless', 'naive', 'hysteria2']);
		expect(visibleInboundTypes('router', 'tun')).toEqual(['tun', 'mixed', 'socks', 'http']);
	});
	it('empty/undefined currentType adds nothing', () => {
		expect(visibleInboundTypes('vps', '')).toEqual(['vless', 'naive', 'hysteria2']);
		expect(visibleInboundTypes('vps', undefined)).toEqual(['vless', 'naive', 'hysteria2']);
	});
});
