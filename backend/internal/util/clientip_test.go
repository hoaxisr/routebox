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
