package api

import (
	"log"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"routebox/backend/internal/settings"
)

// clientIP returns the address a request is attributed to for rate limiting and
// lockout.
//
// The TCP peer is the answer unless it is a proxy the operator has declared
// trusted (security.trusted_proxies). X-Forwarded-For is otherwise
// attacker-controlled: honouring it on a direct connection would let a flood
// mint a fresh identity per request and walk straight past per-IP lockout.
//
// With trusted proxies configured, the client is the RIGHTMOST forwarded
// address that is not itself trusted. Everything to the left of it was written
// by whoever connected to the outermost proxy and may be invented; the rightmost
// untrusted entry is the one our own trusted hop observed, so it is the last
// value in the chain an attacker cannot choose.
//
// Empty list => exactly the old behaviour, which is also what a misconfigured
// deployment gets: attribution collapses onto the proxy, throttling everyone
// behind it together, rather than trusting a header nobody vouched for.
func clientIP(r *http.Request, trusted []netip.Prefix) string {
	peer := peerIP(r)
	if len(trusted) == 0 {
		return peer
	}
	peerAddr, err := netip.ParseAddr(peer)
	if err != nil || !inPrefixes(peerAddr, trusted) {
		return peer
	}
	// nginx needs `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for`;
	// Caddy and Traefik send it by default. X-Real-IP is the common nginx-only
	// alternative, so it is accepted too — from a trusted peer, and only when
	// there is no usable X-Forwarded-For.
	for _, hop := range reversedForwardedFor(r) {
		addr, err := netip.ParseAddr(hop)
		if err != nil {
			continue
		}
		if !inPrefixes(addr, trusted) {
			return addr.String()
		}
	}
	if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
		if addr, err := netip.ParseAddr(real); err == nil {
			return addr.String()
		}
	}
	// Every hop was trusted (or the header was absent/garbage): the peer is the
	// most specific thing actually known.
	return peer
}

// peerIP is the TCP source address, with the port stripped.
func peerIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// reversedForwardedFor returns the X-Forwarded-For hops, nearest proxy first.
// Values may arrive as one comma-separated header or as repeated headers; both
// are the same list, in order, so they are concatenated before reversing.
func reversedForwardedFor(r *http.Request) []string {
	var hops []string
	for _, header := range r.Header.Values("X-Forwarded-For") {
		for _, part := range strings.Split(header, ",") {
			if part = strings.TrimSpace(part); part != "" {
				hops = append(hops, part)
			}
		}
	}
	for i, j := 0, len(hops)-1; i < j; i, j = i+1, j-1 {
		hops[i], hops[j] = hops[j], hops[i]
	}
	return hops
}

func inPrefixes(addr netip.Addr, prefixes []netip.Prefix) bool {
	addr = addr.Unmap() // an IPv4-mapped IPv6 peer must match an IPv4 prefix
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// parseTrustedProxies turns the configured entries into prefixes. A bare
// address is its own /32 or /128. Unparseable entries are dropped with a log
// line rather than failing startup: the consequence of ignoring one is the
// pre-existing behaviour (attribute to the peer), and a panel that refuses to
// boot over a typo in a rate-limiting hint would be the worse outcome.
func parseTrustedProxies(entries []string) []netip.Prefix {
	var out []netip.Prefix
	for _, raw := range entries {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			out = append(out, netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()))
			continue
		}
		if addr, err := netip.ParseAddr(entry); err == nil {
			addr = addr.Unmap()
			out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
			continue
		}
		log.Printf("settings: ignoring security.trusted_proxies entry %q (not an IP or CIDR)", raw)
	}
	return out
}

// trustedProxiesFrom reads and parses the setting. Parsed per call: the list is
// normally empty or a single entry, the paths that use it are logins and
// subscription fetches rather than anything hot, and caching it would have to
// be invalidated on every settings reload to stay correct.
func trustedProxiesFrom(sm *settings.Manager) []netip.Prefix {
	if sm == nil {
		return nil
	}
	return parseTrustedProxies(sm.Get().Security.TrustedProxies)
}
