package traffic

import (
	"net"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// ApexDomain collapses a host to its effective TLD+1 (apex domain).
// IPs and unparseable inputs are returned unchanged. Empty and the "-"
// placeholder are returned unchanged so callers don't need to special-case them.
func ApexDomain(host string) string {
	if host == "" || host == "-" {
		return host
	}
	if ip := net.ParseIP(host); ip != nil {
		return host
	}
	h := strings.ToLower(host)
	apex, err := publicsuffix.EffectiveTLDPlusOne(h)
	if err != nil || apex == "" {
		return host
	}
	return apex
}
