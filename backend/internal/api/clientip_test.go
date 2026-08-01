package api

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func requestFrom(peer string, headers map[string][]string) *http.Request {
	r := httptest.NewRequest("GET", "/sub/tok", nil)
	r.RemoteAddr = peer
	for k, vs := range headers {
		for _, v := range vs {
			r.Header.Add(k, v)
		}
	}
	return r
}

// TestClientIPIgnoresForwardedHeadersByDefault is the property that must never
// regress: with no trusted proxies configured, a header anyone can set has no
// influence at all. Believing it would let one attacker present a new identity
// per request and never trip per-IP lockout.
func TestClientIPIgnoresForwardedHeadersByDefault(t *testing.T) {
	r := requestFrom("203.0.113.7:5555", map[string][]string{
		"X-Forwarded-For": {"1.2.3.4"},
		"X-Real-IP":       {"5.6.7.8"},
	})
	if got := clientIP(r, nil); got != "203.0.113.7" {
		t.Fatalf("clientIP = %q, want the TCP peer 203.0.113.7", got)
	}
}

// TestClientIPIgnoresForwardedFromUntrustedPeer: configuring a trusted proxy
// must not make the header trustworthy from anywhere else. Someone reaching the
// panel directly still counts as themselves.
func TestClientIPIgnoresForwardedFromUntrustedPeer(t *testing.T) {
	trusted := parseTrustedProxies([]string{"172.18.0.0/16"})
	r := requestFrom("203.0.113.7:5555", map[string][]string{"X-Forwarded-For": {"1.2.3.4"}})
	if got := clientIP(r, trusted); got != "203.0.113.7" {
		t.Fatalf("clientIP = %q; a header from an untrusted peer must be ignored", got)
	}
}

func TestClientIPThroughTrustedProxy(t *testing.T) {
	trusted := parseTrustedProxies([]string{"172.18.0.0/16", "10.0.0.1"})

	cases := []struct {
		name    string
		peer    string
		headers map[string][]string
		want    string
	}{
		{
			name:    "single hop",
			peer:    "172.18.0.5:40000",
			headers: map[string][]string{"X-Forwarded-For": {"198.51.100.9"}},
			want:    "198.51.100.9",
		},
		{
			// Spoof attempt: the client prepends a fake entry before talking to
			// the proxy. The proxy appends what it actually saw, so the rightmost
			// untrusted hop is still the real one.
			name:    "client-prepended junk is skipped",
			peer:    "172.18.0.5:40000",
			headers: map[string][]string{"X-Forwarded-For": {"9.9.9.9, 198.51.100.9"}},
			want:    "198.51.100.9",
		},
		{
			name:    "chain of trusted proxies unwinds to the first untrusted hop",
			peer:    "172.18.0.5:40000",
			headers: map[string][]string{"X-Forwarded-For": {"198.51.100.9, 10.0.0.1, 172.18.0.9"}},
			want:    "198.51.100.9",
		},
		{
			name:    "repeated headers are one list",
			peer:    "172.18.0.5:40000",
			headers: map[string][]string{"X-Forwarded-For": {"198.51.100.9", "172.18.0.9"}},
			want:    "198.51.100.9",
		},
		{
			name:    "X-Real-IP is accepted when nothing else is usable",
			peer:    "172.18.0.5:40000",
			headers: map[string][]string{"X-Real-IP": {"198.51.100.9"}},
			want:    "198.51.100.9",
		},
		{
			name:    "X-Forwarded-For wins over X-Real-IP",
			peer:    "172.18.0.5:40000",
			headers: map[string][]string{"X-Forwarded-For": {"198.51.100.9"}, "X-Real-IP": {"5.5.5.5"}},
			want:    "198.51.100.9",
		},
		{
			name:    "garbage falls back to the peer rather than to a made-up key",
			peer:    "172.18.0.5:40000",
			headers: map[string][]string{"X-Forwarded-For": {"not-an-ip"}},
			want:    "172.18.0.5",
		},
		{
			name:    "no header at all",
			peer:    "172.18.0.5:40000",
			headers: nil,
			want:    "172.18.0.5",
		},
		{
			name:    "every hop trusted => the peer is the most specific thing known",
			peer:    "172.18.0.5:40000",
			headers: map[string][]string{"X-Forwarded-For": {"10.0.0.1, 172.18.0.9"}},
			want:    "172.18.0.5",
		},
		{
			name:    "IPv6 client through an IPv4 proxy",
			peer:    "172.18.0.5:40000",
			headers: map[string][]string{"X-Forwarded-For": {"2001:db8::1"}},
			want:    "2001:db8::1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clientIP(requestFrom(tc.peer, tc.headers), trusted); got != tc.want {
				t.Fatalf("clientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestClientIPTrustsIPv6MappedPeer: Go reports a dual-stack listener's IPv4
// peers as ::ffff:a.b.c.d, which must still match an IPv4 prefix — otherwise
// the trust list silently does nothing on exactly the setup most people run.
func TestClientIPTrustsIPv6MappedPeer(t *testing.T) {
	trusted := parseTrustedProxies([]string{"172.18.0.0/16"})
	r := requestFrom("[::ffff:172.18.0.5]:40000", map[string][]string{"X-Forwarded-For": {"198.51.100.9"}})
	if got := clientIP(r, trusted); got != "198.51.100.9" {
		t.Fatalf("clientIP = %q; an IPv4-mapped peer must match an IPv4 trusted prefix", got)
	}
}

func TestParseTrustedProxies(t *testing.T) {
	got := parseTrustedProxies([]string{
		"172.18.0.0/16",
		" 10.0.0.1 ", // bare address, padded
		"2001:db8::/32",
		"",            // skipped
		"nonsense",    // dropped with a log line
		"999.1.1.1/8", // dropped
	})
	if len(got) != 3 {
		t.Fatalf("parsed %d prefixes, want 3: %v", len(got), got)
	}
	// A bare address becomes a host route, not a wildcard.
	if got[1].Bits() != 32 {
		t.Errorf("bare IPv4 should parse as /32, got /%d", got[1].Bits())
	}
	if got[1].Contains(mustAddr(t, "10.0.0.2")) {
		t.Error("a bare address must not match its neighbours")
	}
}

// TestLockKeySeparatesClients: the lockout key must differ per resolved client,
// or one user behind a proxy could lock out the rest.
func TestLockKeySeparatesClients(t *testing.T) {
	trusted := parseTrustedProxies([]string{"172.18.0.0/16"})
	a := lockKey(requestFrom("172.18.0.5:1", map[string][]string{"X-Forwarded-For": {"198.51.100.1"}}), "admin", trusted)
	b := lockKey(requestFrom("172.18.0.5:2", map[string][]string{"X-Forwarded-For": {"198.51.100.2"}}), "admin", trusted)
	if a == b {
		t.Fatal("two clients behind one proxy share a lockout key")
	}
	// Same client, same key — backoff has to accumulate.
	c := lockKey(requestFrom("172.18.0.5:3", map[string][]string{"X-Forwarded-For": {"198.51.100.1"}}), "admin", trusted)
	if a != c {
		t.Fatal("the same client produced two different lockout keys")
	}
}

func mustAddr(t *testing.T, s string) netip.Addr {
	t.Helper()
	a, err := netip.ParseAddr(s)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
