package awg

import (
	"net/netip"
	"testing"
)

func addrs(ss ...string) []netip.Addr {
	out := make([]netip.Addr, len(ss))
	for i, s := range ss {
		out[i] = netip.MustParseAddr(s)
	}
	return out
}

func TestNextFree(t *testing.T) {
	server := netip.MustParseAddr("10.10.0.1")
	cidr := "10.10.0.0/24"

	got, err := NextFree(cidr, addrs("10.10.0.2", "10.10.0.4"), server)
	if err != nil || got.String() != "10.10.0.3" {
		t.Fatalf("lowest-free = %v,%v; want 10.10.0.3", got, err)
	}

	// Order-independence: shuffled used-set -> same result.
	got2, _ := NextFree(cidr, addrs("10.10.0.4", "10.10.0.2"), server)
	if got2 != got {
		t.Fatalf("order dependence: %v vs %v", got, got2)
	}

	// Reuse-after-delete: freeing .3 yields .3 again.
	got3, _ := NextFree(cidr, addrs("10.10.0.2", "10.10.0.4", "10.10.0.5"), server)
	if got3.String() != "10.10.0.3" {
		t.Fatalf("reuse gap: %v; want 10.10.0.3", got3)
	}

	// Out-of-CIDR and non-relevant entries ignored; serverHost excluded implicitly.
	got4, _ := NextFree(cidr, addrs("192.168.1.9"), server)
	if got4.String() != "10.10.0.2" {
		t.Fatalf("first host after server = %v; want 10.10.0.2", got4)
	}
}

func TestNextFreeExhaustionAndBadInput(t *testing.T) {
	server := netip.MustParseAddr("10.10.0.1")
	// /30: hosts .1 (server) and .2 -> after using .2, exhausted.
	if _, err := NextFree("10.10.0.0/30", addrs("10.10.0.2"), server); err == nil {
		t.Fatal("want subnet exhausted")
	}
	if _, err := NextFree("nonsense", nil, server); err == nil {
		t.Fatal("want bad cidr")
	}
	if _, err := NextFree("10.0.0.0/31", nil, server); err == nil {
		t.Fatal("want unusable /31")
	}
	if _, err := NextFree("fd00::/64", nil, server); err == nil {
		t.Fatal("want v6 rejected")
	}
}
