package settings

import (
	"log"
	"net/netip"
	"strings"
)

// ParseTrustedProxies turns the configured entries into prefixes. A bare
// address is its own /32 or /128. Unparseable entries are dropped with a log
// line rather than failing startup: the consequence of ignoring one is the
// pre-existing behaviour (attribute to the peer), and a panel that refuses to
// boot over a typo in a rate-limiting hint would be the worse outcome.
//
// It lives here so the panel's client-IP logic and the Telegram WEB proxy
// read security.trusted_proxies identically.
func ParseTrustedProxies(entries []string) []netip.Prefix {
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
