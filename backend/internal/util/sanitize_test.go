package util

import "testing"

// TestSanitizeName pins the exact rune-for-rune mapping. The callers that used to
// own this coverage (awg.SanitizeName's test) are gone, and what remains is
// property-based: api's TestSanitizeFilename_RejectsHeaderInjection asserts that
// nothing dangerous LEAKS, and awg's TestPeerTagStableAndCollisionScope asserts
// tag stability. Neither pins what a given input actually becomes, so a change to
// the allowed set (say, dropping '.') would slip through both.
func TestSanitizeName(t *testing.T) {
	cases := []struct {
		in, fallback, want string
	}{
		{"phone", "name", "phone"},                                // benign name survives verbatim
		{"a.b-c_d.1", "name", "a.b-c_d.1"},                        // every allowed non-alphanumeric
		{"a\nPublicKey=ATTACKER", "name", "a_PublicKey_ATTACKER"}, // newline cannot forge a .conf directive
		{"x [Peer] # y", "name", "x__Peer____y"},                  // one '_' per disallowed rune, no collapsing
		{"Ноутбук", "name", "name"},                               // non-Latin reduces to nothing usable
		{"!!!", "name", "name"},                                   // nothing usable -> fallback
		{"", "name", "name"},                                      // empty -> fallback
		{"___", "name", "name"},                                   // trims to empty -> fallback
		{"__phone__", "name", "phone"},                            // leading/trailing '_' trimmed
		{"a\r\nSet-Cookie: x=y", "sub", "a__Set-Cookie__x_y"},     // CRLF neutralised, not rejected
		{"../../etc", "sub", ".._.._etc"},                         // '.' is allowed; only '/' is replaced
		{"!!!", "", ""},                                           // fallback is returned as given, even empty
	}
	for _, c := range cases {
		if got := SanitizeName(c.in, c.fallback); got != c.want {
			t.Errorf("SanitizeName(%q, %q) = %q; want %q", c.in, c.fallback, got, c.want)
		}
	}
}

// TestSanitizeNameTrimsOnlyUnderscore documents a sharp edge: Trim strips '_',
// NOT '-', so a name starting with a dash keeps it. Callers that put the result
// into an argv position must not rely on this function to prevent a leading '-'
// being read as a flag — awg.PeerTag is safe because it prefixes "awg-".
func TestSanitizeNameTrimsOnlyUnderscore(t *testing.T) {
	if got := SanitizeName("-rf", "name"); got != "-rf" {
		t.Errorf("SanitizeName(%q) = %q; want %q (leading '-' is NOT trimmed)", "-rf", got, "-rf")
	}
}
