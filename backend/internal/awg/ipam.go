package awg

import (
	"fmt"
	"net/netip"
)

// NextFree returns the lowest-numbered unused host in cidr, scanning from
// network+1, excluding the network address, the broadcast address and serverHost.
// `used` may contain anything; only host addresses WITHIN cidr count.
// Pure and order-independent (it builds a set). IPv4 only.
func NextFree(cidr string, used []netip.Addr, serverHost netip.Addr) (netip.Addr, error) {
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("bad cidr: %w", err)
	}
	p = p.Masked()
	if !p.Addr().Is4() {
		return netip.Addr{}, fmt.Errorf("cidr must be IPv4")
	}
	if p.Bits() > 30 {
		return netip.Addr{}, fmt.Errorf("subnet too small for hosts")
	}

	taken := map[netip.Addr]bool{serverHost: true}
	for _, u := range used {
		if u.Is4() && p.Contains(u) {
			taken[u] = true
		}
	}

	// Scan from network+1 upward. A host is usable while it stays inside the
	// prefix AND its successor is still inside the prefix — the latter excludes
	// the broadcast (last) address without any bit arithmetic.
	for a := p.Addr().Next(); p.Contains(a) && p.Contains(a.Next()); a = a.Next() {
		if !taken[a] {
			return a, nil
		}
	}
	return netip.Addr{}, fmt.Errorf("subnet exhausted")
}
