package serverlinks

import (
	"strings"
	"testing"

	"routebox/backend/internal/subscriptions"
)

// parseBack runs a built link through the real subscription parser and returns
// the single resulting client outbound.
func parseBack(t *testing.T, link string) map[string]interface{} {
	t.Helper()
	nodes, skipped := subscriptions.ParseLinks([]string{link})
	if skipped != 0 || len(nodes) != 1 {
		t.Fatalf("parse round-trip failed: link=%q skipped=%d nodes=%d", link, skipped, len(nodes))
	}
	return nodes[0].Outbound
}

func TestBuildShareLinkVlessReality(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "vless", "tag": "vless-in", "listen": "::", "listen_port": float64(443),
		"tls": map[string]interface{}{
			"enabled": true, "server_name": "www.microsoft.com",
			"reality": map[string]interface{}{
				"enabled": true, "private_key": fixturePriv, "short_id": "0123abcd",
			},
		},
	}
	user := map[string]interface{}{"name": "phone", "uuid": "11111111-2222-3333-4444-555555555555", "flow": "xtls-rprx-vision"}

	link, err := BuildShareLink(inbound, user, "vpn.example.com")
	if err != nil {
		t.Fatalf("BuildShareLink: %v", err)
	}
	if !strings.HasPrefix(link, "vless://11111111-2222-3333-4444-555555555555@vpn.example.com:443?") {
		t.Fatalf("unexpected prefix: %s", link)
	}

	ob := parseBack(t, link)
	if ob["server"] != "vpn.example.com" || ob["server_port"] != 443 {
		t.Fatalf("server/port mismatch: %v", ob)
	}
	if ob["uuid"] != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("uuid mismatch: %v", ob["uuid"])
	}
	if ob["flow"] != "xtls-rprx-vision" {
		t.Fatalf("flow mismatch: %v", ob["flow"])
	}
	tls := ob["tls"].(map[string]interface{})
	if tls["server_name"] != "www.microsoft.com" {
		t.Fatalf("sni mismatch: %v", tls["server_name"])
	}
	reality := tls["reality"].(map[string]interface{})
	if reality["public_key"] != fixturePub {
		t.Fatalf("pbk mismatch: got %v want %v", reality["public_key"], fixturePub)
	}
	if reality["short_id"] != "0123abcd" {
		t.Fatalf("sid mismatch: %v", reality["short_id"])
	}
}

func TestBuildShareLinkVlessPlainTLS(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "vless", "tag": "vless-in", "listen_port": float64(8443),
		"tls": map[string]interface{}{"enabled": true, "server_name": "vpn.example.com"},
	}
	user := map[string]interface{}{"name": "laptop", "uuid": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}

	link, err := BuildShareLink(inbound, user, "vpn.example.com")
	if err != nil {
		t.Fatalf("BuildShareLink: %v", err)
	}
	ob := parseBack(t, link)
	tls := ob["tls"].(map[string]interface{})
	if tls["server_name"] != "vpn.example.com" {
		t.Fatalf("sni mismatch: %v", tls["server_name"])
	}
	if _, hasReality := tls["reality"]; hasReality {
		t.Fatalf("plain TLS link should not carry reality params")
	}
}

func TestBuildShareLinkErrors(t *testing.T) {
	base := map[string]interface{}{"type": "vless", "listen_port": float64(443)}
	user := map[string]interface{}{"uuid": "u"}
	cases := []struct {
		name    string
		inbound map[string]interface{}
		user    map[string]interface{}
		host    string
	}{
		{"empty host", base, user, ""},
		{"no port", map[string]interface{}{"type": "vless"}, user, "h"},
		{"no uuid", base, map[string]interface{}{}, "h"},
		{"bad type", map[string]interface{}{"type": "tun", "listen_port": float64(1)}, user, "h"},
	}
	for _, c := range cases {
		if _, err := BuildShareLink(c.inbound, c.user, c.host); err == nil {
			t.Errorf("%s: expected error, got nil", c.name)
		}
	}
}

func TestBuildShareLinkDisabledReality(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "vless", "listen_port": float64(443),
		"tls": map[string]interface{}{
			"enabled":     true,
			"server_name": "vpn.example.com",
			"reality": map[string]interface{}{
				"enabled":     false,
				"private_key": fixturePriv,
				"short_id":    "0123abcd",
			},
		},
	}
	user := map[string]interface{}{"name": "test", "uuid": "11111111-2222-3333-4444-555555555555"}

	link, err := BuildShareLink(inbound, user, "vpn.example.com")
	if err != nil {
		t.Fatalf("BuildShareLink: %v", err)
	}
	if strings.Contains(link, "security=reality") {
		t.Fatalf("disabled reality must not emit security=reality: %s", link)
	}
	ob := parseBack(t, link)
	tls := ob["tls"].(map[string]interface{})
	if _, hasReality := tls["reality"]; hasReality {
		t.Fatalf("disabled reality link should not carry reality params after round-trip")
	}
}

func TestBuildShareLinkIPv6Host(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "vless", "listen_port": float64(443),
	}
	user := map[string]interface{}{"name": "ipv6user", "uuid": "11111111-2222-3333-4444-555555555555"}
	host := "2001:db8::1"

	link, err := BuildShareLink(inbound, user, host)
	if err != nil {
		t.Fatalf("BuildShareLink: %v", err)
	}
	if !strings.Contains(link, "[2001:db8::1]:443") {
		t.Fatalf("IPv6 host not bracketed in link: %s", link)
	}
	ob := parseBack(t, link)
	if ob["server"] != host {
		t.Fatalf("server mismatch after round-trip: got %v want %v", ob["server"], host)
	}
	if ob["server_port"] != 443 {
		t.Fatalf("server_port mismatch after round-trip: got %v", ob["server_port"])
	}
}
