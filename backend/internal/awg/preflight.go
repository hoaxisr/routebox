package awg

import (
	"net"
	"net/netip"
	"time"
)

// ipv6PreflightTargets are dialed to confirm real egress; 2 hosts so one blocked
// target is not a false negative.
var ipv6PreflightTargets = []string{"[2606:4700:4700::1111]:443", "[2001:4860:4860::8888]:443"}

// egressProbe decides whether the host can egress IPv6: a global-scope v6 address
// must exist locally AND at least one external target must be reachable.
type egressProbe struct {
	hasGlobalV6 func() bool
	dial        func(target string) bool
}

func (p egressProbe) ok() bool {
	if !p.hasGlobalV6() {
		return false
	}
	for _, t := range ipv6PreflightTargets {
		if p.dial(t) {
			return true
		}
	}
	return false
}

func defaultEgressProbe() egressProbe {
	return egressProbe{hasGlobalV6: hostHasGlobalV6, dial: dialV6}
}

// hostHasGlobalV6 reports whether any up, non-loopback interface has a
// global-unicast IPv6 address (excludes link-local/ULA/loopback).
func hostHasGlobalV6() bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok {
			if ad, ok := netip.AddrFromSlice(ipn.IP); ok && ad.Is6() && ad.IsGlobalUnicast() && !ad.IsPrivate() {
				return true
			}
		}
	}
	return false
}

func dialV6(target string) bool {
	c, err := net.DialTimeout("tcp6", target, 3*time.Second)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}
