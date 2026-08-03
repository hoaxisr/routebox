package api

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
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
			// The rightmost hop is the one value the trusted proxy itself wrote,
			// so its ip:port spelling (Azure App Gateway style) is tolerated —
			// the client observed by the proxy wins, never 6.6.6.6 to its left.
			name:    "ip:port rightmost hop resolves to the proxy-observed client",
			peer:    "172.18.0.5:40000",
			headers: map[string][]string{"X-Forwarded-For": {"6.6.6.6, 198.51.100.9:443"}},
			want:    "198.51.100.9",
		},
		{
			// Left of the rightmost hop is client-written: an ip:port there is
			// NOT tolerated. The walk fails closed at the unverifiable hop.
			name:    "ip:port hop left of the rightmost stops the walk at the peer",
			peer:    "172.18.0.5:40000",
			headers: map[string][]string{"X-Forwarded-For": {"6.6.6.6, 9.9.9.9:443, 172.18.0.9"}},
			want:    "172.18.0.5",
		},
		{
			// The RFC 7239 "unknown" placeholder is not an address in any
			// spelling, so nothing to its left is vouched for.
			name:    "unknown hop stops the walk at the peer",
			peer:    "172.18.0.5:40000",
			headers: map[string][]string{"X-Forwarded-For": {"6.6.6.6, unknown"}},
			want:    "172.18.0.5",
		},
		{
			// An empty entry is as unverifiable as "unknown": fail closed, do
			// not skip it.
			name:    "empty hop stops the walk at the peer",
			peer:    "172.18.0.5:40000",
			headers: map[string][]string{"X-Forwarded-For": {"1.2.3.4, , 172.18.0.9"}},
			want:    "172.18.0.5",
		},
		{
			// A whitespace-only header is a failed walk, not an absent one, so
			// it must not fall through to X-Real-IP either.
			name:    "whitespace-only XFF does not fall through to X-Real-IP",
			peer:    "172.18.0.5:40000",
			headers: map[string][]string{"X-Forwarded-For": {" , , "}, "X-Real-IP": {"6.6.6.6"}},
			want:    "172.18.0.5",
		},
		{
			// Zoned IPv6 can neither match a trusted prefix nor serve as a
			// stable limiter key: the zone is stripped before both.
			name:    "IPv6 zone is stripped from the returned key",
			peer:    "172.18.0.5:40000",
			headers: map[string][]string{"X-Forwarded-For": {"fe80::1%eth0"}},
			want:    "fe80::1",
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
		{
			// Fail-closed means closed: an unverifiable X-Forwarded-For must not
			// fall through to X-Real-IP either — both headers travelled the same
			// untrusted path, so the peer is the only thing still vouched for.
			name:    "unparseable hop does not fall through to X-Real-IP",
			peer:    "172.18.0.5:40000",
			headers: map[string][]string{"X-Forwarded-For": {"not-an-ip"}, "X-Real-IP": {"5.5.5.5"}},
			want:    "172.18.0.5",
		},
		{
			// The XFF result must be Unmapped like the trust-check side already
			// is, or ::ffff:a.b.c.d and a.b.c.d would be two limiter keys for one
			// client — a free second identity.
			name:    "IPv4-mapped forwarded address collapses to plain IPv4",
			peer:    "172.18.0.5:40000",
			headers: map[string][]string{"X-Forwarded-For": {"::ffff:198.51.100.9"}},
			want:    "198.51.100.9",
		},
		{
			name:    "garbage X-Real-IP falls back to the peer",
			peer:    "172.18.0.5:40000",
			headers: map[string][]string{"X-Real-IP": {"not-an-ip"}},
			want:    "172.18.0.5",
		},
		{
			// A trusted proxy naming another trusted address is self-attribution,
			// not a client; believing it would key the limiter on infrastructure.
			name:    "X-Real-IP naming a trusted proxy falls back to the peer",
			peer:    "172.18.0.5:40000",
			headers: map[string][]string{"X-Real-IP": {"10.0.0.1"}},
			want:    "172.18.0.5",
		},
		{
			name:    "IPv4-mapped X-Real-IP collapses to plain IPv4",
			peer:    "172.18.0.5:40000",
			headers: map[string][]string{"X-Real-IP": {"::ffff:1.2.3.4"}},
			want:    "1.2.3.4",
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

// TestWarnsOnceAboutAnUntrustedForwarder: the misconfiguration this catches is
// silent by nature — the trust list looks set, the header is ignored, and every
// client collapses onto one address. It must warn, and warn once per peer, so a
// busy proxy cannot flood the log.
func TestWarnsOnceAboutAnUntrustedForwarder(t *testing.T) {
	untrustedForwarders.Lock()
	untrustedForwarders.seen = nil
	untrustedForwarders.Unlock()

	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	trusted := parseTrustedProxies([]string{"172.20.0.0/24"}) // IPv4 only...
	// ...while the proxy actually connects over the network's IPv6 ULA.
	for i := 0; i < 3; i++ {
		clientIP(requestFrom("[fd00:d0c:1::5]:40000",
			map[string][]string{"X-Forwarded-For": {"198.51.100.9"}}), trusted)
	}
	if got := strings.Count(buf.String(), "not in security.trusted_proxies"); got != 1 {
		t.Fatalf("warned %d times, want exactly 1: %s", got, buf.String())
	}
	if !strings.Contains(buf.String(), "fd00:d0c:1::5") {
		t.Errorf("the warning must name the address to add: %s", buf.String())
	}

	// A second, different peer is its own warning.
	clientIP(requestFrom("10.9.9.9:40000",
		map[string][]string{"X-Real-IP": {"198.51.100.9"}}), trusted)
	if got := strings.Count(buf.String(), "not in security.trusted_proxies"); got != 2 {
		t.Fatalf("a distinct peer should warn too, got %d", got)
	}

	// No forwarding headers => nothing to warn about; this is just a direct client.
	buf.Reset()
	clientIP(requestFrom("203.0.113.7:1234", nil), trusted)
	if buf.Len() != 0 {
		t.Fatalf("a plain direct request must not warn: %s", buf.String())
	}
}
