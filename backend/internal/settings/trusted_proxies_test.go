package settings

import (
	"net/netip"
	"testing"
)

func TestParseTrustedProxies(t *testing.T) {
	got := ParseTrustedProxies([]string{
		"172.18.0.0/16",
		" 10.0.0.1 ", // bare address, padded
		"2001:db8::/32",
		"",                // skipped
		"nonsense",        // dropped with a log line
		"junk",            // dropped
		"999.1.1.1/8",     // dropped
		"::ffff:10.0.0.2", // mapped, unmapped to IPv4
		"10.0.0.0/8",
	})
	want := []string{"172.18.0.0/16", "10.0.0.1/32", "2001:db8::/32", "10.0.0.2/32", "10.0.0.0/8"}
	if len(got) != len(want) {
		t.Fatalf("parsed %d prefixes, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != netip.MustParsePrefix(w) {
			t.Errorf("entry %d = %v, want %s", i, got[i], w)
		}
	}
	// A bare address becomes a host route, not a wildcard.
	if got[1].Contains(netip.MustParseAddr("10.0.0.2")) {
		t.Error("a bare address must not match its neighbours")
	}
}
