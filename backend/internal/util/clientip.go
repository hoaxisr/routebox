package util

import "net/netip"

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
