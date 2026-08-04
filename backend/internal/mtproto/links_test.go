package mtproto

import (
	"encoding/hex"
	"net/url"
	"strings"
	"testing"
)

func TestGenerateSecretIs32HexChars(t *testing.T) {
	s, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}

	if len(s) != 32 {
		t.Errorf("len = %d, want 32 (16 bytes hex-encoded)", len(s))
	}

	if _, err := hex.DecodeString(s); err != nil {
		t.Errorf("not hex: %v", err)
	}
}

func TestGenerateSecretIsNotConstant(t *testing.T) {
	a, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}

	b, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}

	if a == b {
		t.Error("two generated secrets are identical")
	}
}

func TestFakeTLSSecretPrefixesEEAndAppendsTheDomain(t *testing.T) {
	got := FakeTLSSecret("00112233445566778899aabbccddeeff", "example.com")

	want := "ee00112233445566778899aabbccddeeff" + hex.EncodeToString([]byte("example.com"))
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestProxyLinkCarriesServerPortAndSecret(t *testing.T) {
	got := ProxyLink("panel.example.com", 443, "00112233445566778899aabbccddeeff", "example.com")

	if !strings.HasPrefix(got, "tg://proxy?") {
		t.Fatalf("got %q, want a tg://proxy link", got)
	}

	q, err := url.ParseQuery(strings.TrimPrefix(got, "tg://proxy?"))
	if err != nil {
		t.Fatal(err)
	}

	if q.Get("server") != "panel.example.com" {
		t.Errorf("server = %q", q.Get("server"))
	}

	if q.Get("port") != "443" {
		t.Errorf("port = %q", q.Get("port"))
	}

	if want := FakeTLSSecret("00112233445566778899aabbccddeeff", "example.com"); q.Get("secret") != want {
		t.Errorf("secret = %q, want %q", q.Get("secret"), want)
	}
}

func TestWebLinkCarriesAnIdenticalQuery(t *testing.T) {
	const (
		host   = "panel.example.com"
		secret = "00112233445566778899aabbccddeeff"
		domain = "example.com"
	)

	tg := ProxyLink(host, 443, secret, domain)
	web := WebLink(host, 443, secret, domain)

	// The two forms exist because tg:// does not survive every chat client and
	// browser, not because they carry different information.
	if strings.TrimPrefix(tg, "tg://proxy?") != strings.TrimPrefix(web, "https://t.me/proxy?") {
		t.Errorf("queries differ:\n tg  = %s\n web = %s", tg, web)
	}

	if !strings.HasPrefix(web, "https://t.me/proxy?") {
		t.Errorf("got %q, want an https://t.me/proxy link", web)
	}
}

func TestLinksEscapeAnIPv6Host(t *testing.T) {
	got := ProxyLink("2001:db8::1", 443, "00112233445566778899aabbccddeeff", "example.com")

	q, err := url.ParseQuery(strings.TrimPrefix(got, "tg://proxy?"))
	if err != nil {
		t.Fatalf("an IPv6 host produced an unparseable query %q: %v", got, err)
	}

	if q.Get("server") != "2001:db8::1" {
		t.Errorf("server = %q, want the address to survive escaping", q.Get("server"))
	}
}

func TestCanIssueLinkNeedsBothDomainAndHost(t *testing.T) {
	for _, tt := range []struct {
		name   string
		domain string
		host   string
		want   bool
	}{
		{"both set", "example.com", "panel.example.com", true},
		{"no domain", "", "panel.example.com", false},
		{"no host", "example.com", "", false},
		{"blank domain", "   ", "panel.example.com", false},
		{"blank host", "example.com", "  ", false},
		{"neither", "", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// A link built from a missing piece looks fine and then fails
			// silently inside Telegram, so the caller has to be able to ask.
			if got := CanIssueLink(tt.domain, tt.host); got != tt.want {
				t.Errorf("CanIssueLink(%q, %q) = %v, want %v", tt.domain, tt.host, got, tt.want)
			}
		})
	}
}
