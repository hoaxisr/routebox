/**
 * Type guards for outbound types
 * Use these for type-safe access to protocol-specific fields
 */

import type {
	Outbound,
	OutboundVless,
	OutboundHysteria2,
	OutboundShadowsocks,
	OutboundShadowtls,
	OutboundAnytls,
	OutboundSelector,
	OutboundUrltest,
	OutboundDirect,
	OutboundBlock
} from '$lib/types';

/**
 * Check if outbound is VLESS type
 */
export function isVlessOutbound(ob: Outbound): ob is OutboundVless {
	return ob.type === 'vless';
}

/**
 * Check if outbound is Hysteria2 type
 */
export function isHysteria2Outbound(ob: Outbound): ob is OutboundHysteria2 {
	return ob.type === 'hysteria2' || ob.type === 'hy2';
}

/**
 * Check if outbound is Shadowsocks type
 */
export function isShadowsocksOutbound(ob: Outbound): ob is OutboundShadowsocks {
	return ob.type === 'shadowsocks' || ob.type === 'ss';
}

/**
 * Check if outbound is ShadowTLS type
 */
export function isShadowtlsOutbound(ob: Outbound): ob is OutboundShadowtls {
	return ob.type === 'shadowtls';
}

/**
 * Check if outbound is AnyTLS type
 */
export function isAnytlsOutbound(ob: Outbound): ob is OutboundAnytls {
	return ob.type === 'anytls';
}

/**
 * Check if outbound is selector type
 */
export function isSelectorOutbound(ob: Outbound): ob is OutboundSelector {
	return ob.type === 'selector';
}

/**
 * Check if outbound is urltest type
 */
export function isUrltestOutbound(ob: Outbound): ob is OutboundUrltest {
	return ob.type === 'urltest';
}

/**
 * Check if outbound is a service type (selector or urltest)
 */
export function isServiceOutbound(ob: Outbound): ob is OutboundSelector | OutboundUrltest {
	return ob.type === 'selector' || ob.type === 'urltest';
}

/**
 * Check if outbound is direct type
 */
export function isDirectOutbound(ob: Outbound): ob is OutboundDirect {
	return ob.type === 'direct';
}

/**
 * Check if outbound is block type
 */
export function isBlockOutbound(ob: Outbound): ob is OutboundBlock {
	return ob.type === 'block';
}

/**
 * Check if outbound is a proxy type (has server/port)
 */
export function isProxyOutbound(
	ob: Outbound
): ob is OutboundVless | OutboundHysteria2 | OutboundShadowsocks | OutboundShadowtls | OutboundAnytls {
	return ['vless', 'hysteria2', 'hy2', 'shadowsocks', 'ss', 'shadowtls', 'anytls'].includes(
		ob.type
	);
}

/**
 * Check if outbound supports TLS configuration
 */
export function supportsTls(
	ob: Outbound
): ob is OutboundVless | OutboundHysteria2 | OutboundShadowtls | OutboundAnytls {
	return ['vless', 'hysteria2', 'hy2', 'shadowtls', 'anytls'].includes(ob.type);
}

/**
 * Check if outbound supports multiplex
 */
export function supportsMultiplex(ob: Outbound): ob is OutboundShadowsocks {
	return ob.type === 'shadowsocks' || ob.type === 'ss';
}

/**
 * Get server address from outbound (if applicable)
 */
export function getOutboundServer(ob: Outbound): string | undefined {
	if (isProxyOutbound(ob)) {
		return ob.server;
	}
	return undefined;
}

/**
 * Get server port from outbound (if applicable)
 */
export function getOutboundPort(ob: Outbound): number | undefined {
	if (isProxyOutbound(ob)) {
		return ob.server_port;
	}
	return undefined;
}
