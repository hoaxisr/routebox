package util

import (
	"net/netip"
	"strings"
)

// CanonicalClientIP collapses an IPv4-mapped IPv6 address ("::ffff:203.0.113.7")
// to the plain IPv4 it stands for, and leaves everything else alone.
//
// Inbounds bind a dual-stack socket, so sing-box reports an IPv4 client through
// the Clash API in the mapped form. Left as-is it is a DIFFERENT string from the
// same client's plain address: the client list grows a second entry that has to
// be named again, traffic history splits into two buckets for one device, and the
// connections monitor shows "::ffff:77.94…" where a name was expected (#71).
//
// Anything unparseable is returned untouched — this normalises a display and
// lookup key, it is not a validator.
func CanonicalClientIP(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	if addr.Is4In6() {
		return addr.Unmap().String()
	}
	return ip
}

// cgnat is RFC 6598 100.64.0.0/10 — the shared address space some tunnel
// setups hand out to peers. netip has no helper for it, hence the constant.
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// ParsePrefixes turns configured CIDR strings into prefixes, skipping anything
// unparseable. Feeding IsLocalClientIP a setting is then one call, and a typo in
// the settings file costs that one prefix rather than an error path in every
// caller.
func ParsePrefixes(list ...string) []netip.Prefix {
	var out []netip.Prefix
	for _, s := range list {
		if p, err := netip.ParsePrefix(strings.TrimSpace(s)); err == nil {
			out = append(out, p.Masked())
		}
	}
	return out
}

// IsLocalClientIP reports whether ip can be a client of this box: an address
// from the LAN, a tunnel, or the box itself. Public IPv4 addresses are not — in
// router mode they show up in the Clash connection list anyway (#102), where
// they became a "client" row for a Google front-end nobody can name.
//
// extra widens the answer with prefixes this box knows are its own. The AWG
// tunnel subnet is configurable and only defaults to 10.10.0.0/24: an operator
// who picked something globally routable still has peers, and without this their
// traffic would vanish from the peer pages, which read it by tunnel IP.
//
// IPv6 goes the other way on purpose. A LAN client's IPv6 address is globally
// routable by design — a delegated prefix is normal, ULA is the exception — so
// the address alone cannot tell a device from a stranger, and guessing wrong
// erases a real device's history. Only the addresses that can't be a client at
// all (multicast, unspecified) are refused.
//
// Anything unparseable is not local: the value came from a connection's
// sourceIP, and a non-address there is not a device either.
func IsLocalClientIP(ip string, extra ...netip.Prefix) bool {
	addr, err := netip.ParseAddr(CanonicalClientIP(ip))
	if err != nil {
		return false
	}
	for _, p := range extra {
		if p.Contains(addr) {
			return true
		}
	}
	if !addr.Is4() {
		return !addr.IsMulticast() && !addr.IsUnspecified()
	}
	return addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || cgnat.Contains(addr)
}
