package util

import "testing"

func TestCanonicalClientIP(t *testing.T) {
	cases := map[string]string{
		// what a dual-stack inbound reports for an IPv4 client
		"::ffff:203.0.113.7": "203.0.113.7",
		"::ffff:192.168.1.4": "192.168.1.4",
		// the same address in the hex spelling netip also accepts
		"::ffff:cb00:7107": "203.0.113.7",
		// left alone
		"203.0.113.7": "203.0.113.7",
		"2001:db8::1": "2001:db8::1",
		"::1":         "::1",
		// not an address: a display key, not a validator
		"":                 "",
		"unknown":          "unknown",
		"nonsense.example": "nonsense.example",
	}
	for in, want := range cases {
		if got := CanonicalClientIP(in); got != want {
			t.Errorf("CanonicalClientIP(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsLocalClientIP(t *testing.T) {
	cases := map[string]bool{
		// LAN and tunnel
		"192.168.1.14": true,
		"10.10.64.2":   true,
		"172.16.0.5":   true,
		"100.64.0.7":   true, // RFC 6598, shared address space
		"127.0.0.1":    true,
		"169.254.1.1":  true,
		"fd00::1":      true, // ULA
		"::1":          true,
		// the mapped spelling of a LAN address is still the LAN
		"::ffff:192.168.1.14": true,
		// public IPv4 — what #102 is about
		"172.217.116.4":      false,
		"8.8.8.8":            false,
		"::ffff:172.217.1.1": false,
		// a LAN client's IPv6 is globally routable by design: kept, because the
		// address cannot say whether it is a device, and dropping erases one
		"2001:db8::1": true,
		"2a02:6b8::1": true,
		// these can never be a client
		"ff02::1": false,
		"::":      false,
		// not an address at all
		"":        false,
		"unknown": false,
		// a valid address that can never belong to a device
		"0.0.0.0": false,
	}
	for in, want := range cases {
		if got := IsLocalClientIP(in); got != want {
			t.Errorf("IsLocalClientIP(%q) = %v, want %v", in, got, want)
		}
	}
}

// The AWG tunnel subnet is configurable, and an operator who picked something
// globally routable still has peers — the peer pages read their traffic by
// tunnel IP, so the subnet has to widen the answer (#102 review).
func TestIsLocalClientIP_ExtraPrefixes(t *testing.T) {
	tunnel := ParsePrefixes("26.26.26.0/24", "not a prefix", "  10.77.0.0/16 ")
	if len(tunnel) != 2 {
		t.Fatalf("ParsePrefixes = %v, want the two valid prefixes", tunnel)
	}
	if IsLocalClientIP("26.26.26.2") {
		t.Error("26.26.26.2 is public without the tunnel subnet")
	}
	for _, ip := range []string{"26.26.26.2", "10.77.0.9"} {
		if !IsLocalClientIP(ip, tunnel...) {
			t.Errorf("IsLocalClientIP(%q, tunnel) = false, want true", ip)
		}
	}
	if IsLocalClientIP("172.217.116.4", tunnel...) {
		t.Error("a prefix that does not contain the address must not make it local")
	}
}
