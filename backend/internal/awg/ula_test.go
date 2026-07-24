package awg

import (
	"net/netip"
	"testing"
)

func TestMapV4ToV6EmbedsFull32Bits(t *testing.T) {
	pfx := netip.MustParsePrefix("fd00:abcd:ef01::/64")
	for _, tc := range []struct{ v4, want string }{
		{"10.10.0.1", "fd00:abcd:ef01::a0a:1"},
		{"10.10.0.5", "fd00:abcd:ef01::a0a:5"},
		{"172.16.5.9", "fd00:abcd:ef01::ac10:509"},
	} {
		got, err := MapV4ToV6(pfx, netip.MustParseAddr(tc.v4))
		if err != nil {
			t.Fatalf("%s: %v", tc.v4, err)
		}
		if got.String() != tc.want {
			t.Errorf("%s -> %s, want %s", tc.v4, got, tc.want)
		}
	}
}

func TestMapV4ToV6RejectsNonV4(t *testing.T) {
	pfx := netip.MustParsePrefix("fd00:abcd:ef01::/64")
	if _, err := MapV4ToV6(pfx, netip.MustParseAddr("::1")); err == nil {
		t.Fatal("expected error for non-IPv4 input")
	}
}

func TestGenerateULAPrefixIsFdULA64(t *testing.T) {
	p, err := GenerateULAPrefix()
	if err != nil {
		t.Fatal(err)
	}
	if p.Bits() != 64 {
		t.Fatalf("bits = %d, want 64", p.Bits())
	}
	if b := p.Addr().As16(); b[0] != 0xfd {
		t.Fatalf("first byte = %#x, want 0xfd (ULA)", b[0])
	}
	// Two draws must differ (40 random bits).
	q, _ := GenerateULAPrefix()
	if p == q {
		t.Fatal("two prefixes identical — not random")
	}
}
