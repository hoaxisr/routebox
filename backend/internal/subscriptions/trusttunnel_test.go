package subscriptions

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"time"
)

func selfSignedDER(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "tt"},
		NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

// tlv encodes one TLV entry with single-byte varints (values < 64).
func tlv(tag byte, val []byte) []byte {
	return append([]byte{tag, byte(len(val))}, val...)
}

func ttPayload(entries ...[]byte) string {
	var raw []byte
	for _, e := range entries {
		raw = append(raw, e...)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func TestParseTrustTunnelDeepLink(t *testing.T) {
	payload := ttPayload(
		tlv(ttTagVersion, []byte{1}),
		tlv(ttTagHostname, []byte("vpn.example.com")),
		tlv(ttTagAddresses, []byte("1.2.3.4:443")),
		tlv(ttTagUsername, []byte("alice")),
		tlv(ttTagPassword, []byte("s3cret")),
		tlv(ttTagSkipVerify, []byte{1}),
		tlv(ttTagAntiDPI, []byte{1}),
		tlv(0x0B, []byte("ignored-unknown-tag")),
	)
	nodes, err := parseTrustTunnel("tt://?" + payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("nodes = %d", len(nodes))
	}
	want := map[string]interface{}{
		"type": "trusttunnel", "server": "1.2.3.4", "server_port": 443,
		"username": "alice", "password": "s3cret",
		"tls": map[string]interface{}{"enabled": true, "server_name": "vpn.example.com", "insecure": true, "fragment": true},
	}
	if !reflect.DeepEqual(nodes[0].Outbound, want) {
		t.Errorf("outbound = %v\nwant %v", nodes[0].Outbound, want)
	}
	if nodes[0].Name != "vpn.example.com" {
		t.Errorf("name falls back to hostname, got %q", nodes[0].Name)
	}
}

func TestParseTrustTunnelConnectURLMultiAddress(t *testing.T) {
	payload := ttPayload(
		tlv(ttTagHostname, []byte("vpn.example.com")),
		tlv(ttTagAddresses, []byte("1.2.3.4:443")),
		tlv(ttTagAddresses, []byte("[2001:db8::1]:8443")),
		tlv(ttTagCustomSNI, []byte("cdn.example.net")),
		tlv(ttTagUsername, []byte("u")),
		tlv(ttTagPassword, []byte("p")),
		tlv(ttTagName, []byte("Home")),
	)
	nodes, err := parseTrustTunnel("https://connect.example.com/link?d=" + payload + "&name=Office")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("one outbound per address, got %d", len(nodes))
	}
	if nodes[0].Name != "Office-1" || nodes[1].Name != "Office-2" {
		t.Errorf("url name= wins and multi-address tags get -N: %q %q", nodes[0].Name, nodes[1].Name)
	}
	if nodes[1].Outbound["server"] != "2001:db8::1" || nodes[1].Outbound["server_port"] != 8443 {
		t.Errorf("ipv6 address: %v", nodes[1].Outbound)
	}
	tls := nodes[0].Outbound["tls"].(map[string]interface{})
	if tls["server_name"] != "cdn.example.net" {
		t.Errorf("custom SNI: %v", tls)
	}
	if _, has := tls["insecure"]; has {
		t.Errorf("no skip-verify → no insecure: %v", tls)
	}
}

func TestParseTrustTunnelRejects(t *testing.T) {
	cases := map[string]string{
		"no creds":      ttPayload(tlv(ttTagHostname, []byte("h")), tlv(ttTagAddresses, []byte("1.2.3.4:1"))),
		"no addresses":  ttPayload(tlv(ttTagHostname, []byte("h")), tlv(ttTagUsername, []byte("u")), tlv(ttTagPassword, []byte("p"))),
		"newer version": ttPayload(tlv(ttTagVersion, []byte{2}), tlv(ttTagHostname, []byte("h"))),
		"truncated":     base64.RawURLEncoding.EncodeToString([]byte{ttTagHostname, 10, 'a'}),
		"empty":         "",
	}
	for name, payload := range cases {
		if _, err := parseTrustTunnel("tt://?" + payload); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

// Values ≥ 64 bytes need the 2-byte varint length prefix (0x40|hi, lo);
// certificates and long hostnames always do.
func TestParseTrustTunnelMultiByteVarintCertificateAndPadding(t *testing.T) {
	longHost := strings.Repeat("h", 70) + ".example"
	tlv2 := func(tag byte, val []byte) []byte {
		return append([]byte{tag, 0x40 | byte(len(val)>>8), byte(len(val))}, val...)
	}
	der := selfSignedDER(t)
	raw := append(append(append(append(
		tlv2(ttTagHostname, []byte(longHost)),
		tlv(ttTagAddresses, []byte("1.2.3.4:443"))...),
		tlv(ttTagUsername, []byte("u"))...),
		tlv(ttTagPassword, []byte("p"))...),
		tlv2(ttTagCertificate, der)...)
	raw = append(raw, tlv(ttTagSkipVerify, []byte{0})...)
	// Standard alphabet WITH padding must decode like base64url without it.
	padded := base64.StdEncoding.EncodeToString(raw)
	nodes, err := parseTrustTunnel("tt://?" + padded)
	if err != nil {
		t.Fatal(err)
	}
	tls := nodes[0].Outbound["tls"].(map[string]interface{})
	if tls["server_name"] != longHost {
		t.Errorf("2-byte length hostname: %v", tls["server_name"])
	}
	cert, _ := tls["certificate"].([]string)
	if len(cert) < 3 || cert[0] != "-----BEGIN CERTIFICATE-----" || cert[len(cert)-1] != "-----END CERTIFICATE-----" {
		t.Errorf("certificate must be PEM split into lines: %v", cert)
	}
	if _, has := tls["insecure"]; has {
		t.Errorf("skip_verification=0 must not set insecure: %v", tls)
	}
}

func TestParseLinksRoutesTrustTunnel(t *testing.T) {
	payload := ttPayload(tlv(ttTagHostname, []byte("h.example")), tlv(ttTagAddresses, []byte("h.example:443")),
		tlv(ttTagUsername, []byte("u")), tlv(ttTagPassword, []byte("p")))
	nodes, skipped := ParseLinks([]string{"tt://?" + payload, "http://x.example/c?d=" + payload, "https://x.example/no-payload", "garbage"})
	if skipped != 2 || len(nodes) != 2 {
		t.Fatalf("nodes=%d skipped=%d", len(nodes), skipped)
	}
	for _, n := range nodes {
		if n.Outbound["type"] != "trusttunnel" || !strings.HasPrefix(n.Name, "h.example") {
			t.Errorf("unexpected node %v", n)
		}
	}
}
