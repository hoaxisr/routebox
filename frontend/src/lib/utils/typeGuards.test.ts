import { describe, it, expect } from 'vitest';
import type { Outbound } from '$lib/types';
import {
	isVlessOutbound,
	isHysteria2Outbound,
	isShadowsocksOutbound,
	isShadowtlsOutbound,
	isAnytlsOutbound,
	isNaiveOutbound,
	isSelectorOutbound,
	isUrltestOutbound,
	isServiceOutbound,
	isDirectOutbound,
	isBlockOutbound,
	isProxyOutbound,
	supportsTls,
	supportsMultiplex,
	getOutboundServer,
	getOutboundPort
} from './typeGuards';

const createOutbound = (type: string, extra: Partial<Outbound> = {}): Outbound => ({
	type,
	tag: 'test',
	...extra
});

describe('isVlessOutbound', () => {
	it('returns true for vless type', () => {
		expect(isVlessOutbound(createOutbound('vless'))).toBe(true);
	});

	it('returns false for other types', () => {
		expect(isVlessOutbound(createOutbound('hysteria2'))).toBe(false);
		expect(isVlessOutbound(createOutbound('direct'))).toBe(false);
	});
});

describe('isHysteria2Outbound', () => {
	it('returns true for hysteria2 type', () => {
		expect(isHysteria2Outbound(createOutbound('hysteria2'))).toBe(true);
	});

	it('returns true for hy2 alias', () => {
		expect(isHysteria2Outbound(createOutbound('hy2'))).toBe(true);
	});

	it('returns false for other types', () => {
		expect(isHysteria2Outbound(createOutbound('vless'))).toBe(false);
	});
});

describe('isShadowsocksOutbound', () => {
	it('returns true for shadowsocks type', () => {
		expect(isShadowsocksOutbound(createOutbound('shadowsocks'))).toBe(true);
	});

	it('returns true for ss alias', () => {
		expect(isShadowsocksOutbound(createOutbound('ss'))).toBe(true);
	});

	it('returns false for other types', () => {
		expect(isShadowsocksOutbound(createOutbound('vless'))).toBe(false);
	});
});

describe('isShadowtlsOutbound', () => {
	it('returns true for shadowtls type', () => {
		expect(isShadowtlsOutbound(createOutbound('shadowtls'))).toBe(true);
	});

	it('returns false for other types', () => {
		expect(isShadowtlsOutbound(createOutbound('shadowsocks'))).toBe(false);
	});
});

describe('isAnytlsOutbound', () => {
	it('returns true for anytls type', () => {
		expect(isAnytlsOutbound(createOutbound('anytls'))).toBe(true);
	});

	it('returns false for other types', () => {
		expect(isAnytlsOutbound(createOutbound('vless'))).toBe(false);
	});
});

describe('isSelectorOutbound', () => {
	it('returns true for selector type', () => {
		expect(isSelectorOutbound(createOutbound('selector'))).toBe(true);
	});

	it('returns false for other types', () => {
		expect(isSelectorOutbound(createOutbound('urltest'))).toBe(false);
	});
});

describe('isUrltestOutbound', () => {
	it('returns true for urltest type', () => {
		expect(isUrltestOutbound(createOutbound('urltest'))).toBe(true);
	});

	it('returns false for other types', () => {
		expect(isUrltestOutbound(createOutbound('selector'))).toBe(false);
	});
});

describe('isServiceOutbound', () => {
	it('returns true for selector', () => {
		expect(isServiceOutbound(createOutbound('selector'))).toBe(true);
	});

	it('returns true for urltest', () => {
		expect(isServiceOutbound(createOutbound('urltest'))).toBe(true);
	});

	it('returns false for proxy types', () => {
		expect(isServiceOutbound(createOutbound('vless'))).toBe(false);
		expect(isServiceOutbound(createOutbound('direct'))).toBe(false);
	});
});

describe('isDirectOutbound', () => {
	it('returns true for direct type', () => {
		expect(isDirectOutbound(createOutbound('direct'))).toBe(true);
	});

	it('returns false for other types', () => {
		expect(isDirectOutbound(createOutbound('block'))).toBe(false);
	});
});

describe('isBlockOutbound', () => {
	it('returns true for block type', () => {
		expect(isBlockOutbound(createOutbound('block'))).toBe(true);
	});

	it('returns false for other types', () => {
		expect(isBlockOutbound(createOutbound('direct'))).toBe(false);
	});
});

describe('isProxyOutbound', () => {
	it('returns true for proxy protocols', () => {
		expect(isProxyOutbound(createOutbound('vless'))).toBe(true);
		expect(isProxyOutbound(createOutbound('hysteria2'))).toBe(true);
		expect(isProxyOutbound(createOutbound('hy2'))).toBe(true);
		expect(isProxyOutbound(createOutbound('shadowsocks'))).toBe(true);
		expect(isProxyOutbound(createOutbound('ss'))).toBe(true);
		expect(isProxyOutbound(createOutbound('shadowtls'))).toBe(true);
		expect(isProxyOutbound(createOutbound('anytls'))).toBe(true);
	});

	it('returns false for service and special types', () => {
		expect(isProxyOutbound(createOutbound('selector'))).toBe(false);
		expect(isProxyOutbound(createOutbound('urltest'))).toBe(false);
		expect(isProxyOutbound(createOutbound('direct'))).toBe(false);
		expect(isProxyOutbound(createOutbound('block'))).toBe(false);
	});
});

describe('supportsTls', () => {
	it('returns true for TLS-capable protocols', () => {
		expect(supportsTls(createOutbound('vless'))).toBe(true);
		expect(supportsTls(createOutbound('hysteria2'))).toBe(true);
		expect(supportsTls(createOutbound('hy2'))).toBe(true);
		expect(supportsTls(createOutbound('shadowtls'))).toBe(true);
		expect(supportsTls(createOutbound('anytls'))).toBe(true);
	});

	it('returns false for non-TLS protocols', () => {
		expect(supportsTls(createOutbound('shadowsocks'))).toBe(false);
		expect(supportsTls(createOutbound('direct'))).toBe(false);
	});
});

describe('supportsMultiplex', () => {
	it('returns true for shadowsocks', () => {
		expect(supportsMultiplex(createOutbound('shadowsocks'))).toBe(true);
		expect(supportsMultiplex(createOutbound('ss'))).toBe(true);
	});

	it('returns false for other protocols', () => {
		expect(supportsMultiplex(createOutbound('vless'))).toBe(false);
		expect(supportsMultiplex(createOutbound('hysteria2'))).toBe(false);
	});
});

describe('getOutboundServer', () => {
	it('returns server for proxy outbounds', () => {
		const ob = createOutbound('vless', { server: 'example.com' });
		expect(getOutboundServer(ob)).toBe('example.com');
	});

	it('returns undefined for non-proxy outbounds', () => {
		const ob = createOutbound('direct');
		expect(getOutboundServer(ob)).toBeUndefined();
	});
});

describe('getOutboundPort', () => {
	it('returns port for proxy outbounds', () => {
		const ob = createOutbound('vless', { server_port: 443 });
		expect(getOutboundPort(ob)).toBe(443);
	});

	it('returns undefined for non-proxy outbounds', () => {
		const ob = createOutbound('direct');
		expect(getOutboundPort(ob)).toBeUndefined();
	});
});

describe('isNaiveOutbound', () => {
	it('returns true for naive outbound', () => {
		expect(isNaiveOutbound({ type: 'naive', tag: 'n' })).toBe(true);
	});

	it('returns false for other types', () => {
		expect(isNaiveOutbound({ type: 'vless', tag: 'v' })).toBe(false);
	});
});

describe('naive in aggregate guards', () => {
	it('isProxyOutbound includes naive', () => {
		expect(isProxyOutbound({ type: 'naive', tag: 'n' })).toBe(true);
	});

	it('supportsTls includes naive', () => {
		expect(supportsTls({ type: 'naive', tag: 'n' })).toBe(true);
	});
});
