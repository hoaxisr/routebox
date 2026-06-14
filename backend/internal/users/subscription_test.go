package users

import (
	"encoding/base64"
	"strings"
	"testing"

	"routebox/backend/internal/subscriptions"
)

// activeMultiInbound builds an active config with a vless and a hysteria2 inbound,
// each carrying one user keyed by the credentials we bind below.
func activeMultiInbound() map[string]interface{} {
	return map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"type": "vless", "tag": "vless-in", "listen_port": float64(443),
				"tls": map[string]interface{}{"enabled": true},
				"users": []interface{}{
					map[string]interface{}{"name": "alice", "uuid": "uuid-1",
						"flow": "xtls-rprx-vision"},
				},
			},
			map[string]interface{}{
				"type": "hysteria2", "tag": "hy2-in", "listen_port": float64(8443),
				"tls": map[string]interface{}{"enabled": true},
				"users": []interface{}{
					map[string]interface{}{"name": "alice", "password": "pw-1"},
				},
			},
		},
	}
}

func userBoundTo() *PanelUser {
	return &PanelUser{
		ID: "u1", Name: "alice", Token: "tok",
		Bindings: []Binding{
			{InboundTag: "vless-in", Credential: "uuid-1", Protocol: "vless"},
			{InboundTag: "hy2-in", Credential: "pw-1", Protocol: "hysteria2"},
		},
	}
}

func TestBuildSubscription_RoundTripMultiBinding(t *testing.T) {
	out, err := BuildSubscription(userBoundTo(), activeMultiInbound(), "vpn.example.com")
	if err != nil {
		t.Fatalf("BuildSubscription: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(out)
	if err != nil {
		t.Fatalf("output is not std base64: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 links, got %d: %q", len(lines), string(raw))
	}
	nodes, skipped := subscriptions.ParseLinks(lines)
	if skipped != 0 {
		t.Fatalf("client parser skipped %d of our own links", skipped)
	}
	if len(nodes) != 2 {
		t.Fatalf("parser produced %d nodes, want 2", len(nodes))
	}
	// Every node must point at the public host with the bound credential.
	for _, n := range nodes {
		if n.Outbound["server"] != "vpn.example.com" {
			t.Fatalf("node server = %v, want vpn.example.com", n.Outbound["server"])
		}
	}
}

func TestBuildSubscription_SkipsBindingMissingFromActive(t *testing.T) {
	u := userBoundTo()
	// Add a binding whose inbound is NOT in active — must be skipped, not fatal.
	u.Bindings = append(u.Bindings, Binding{
		InboundTag: "ghost-in", Credential: "x", Protocol: "vless"})
	out, err := BuildSubscription(u, activeMultiInbound(), "vpn.example.com")
	if err != nil {
		t.Fatalf("BuildSubscription must not fail on a missing binding: %v", err)
	}
	raw, _ := base64.StdEncoding.DecodeString(out)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("missing binding not skipped; got %d lines: %q", len(lines), string(raw))
	}
}

func TestBuildSubscription_EmptyWhenNoActiveNodes(t *testing.T) {
	u := &PanelUser{ID: "u1", Name: "alice", Token: "tok",
		Bindings: []Binding{{InboundTag: "vless-in", Credential: "uuid-1", Protocol: "vless"}}}
	out, err := BuildSubscription(u, map[string]interface{}{"inbounds": []interface{}{}}, "vpn.example.com")
	if err != nil {
		t.Fatalf("BuildSubscription: %v", err)
	}
	// Empty list => base64.StdEncoding of "" => "".
	if out != "" {
		t.Fatalf("expected empty base64 for no resolvable nodes, got %q", out)
	}
}

// Host-agnostic: an empty host is NOT a builder error (the handler owns the 503
// policy). With no host, BuildShareLink errors per binding and every binding is
// skipped, yielding base64 of "" (i.e. ""). The builder must NOT hard-error.
func TestBuildSubscription_EmptyHostIsNotAnError(t *testing.T) {
	out, err := BuildSubscription(userBoundTo(), activeMultiInbound(), "")
	if err != nil {
		t.Fatalf("empty host must not error the pure builder: %v", err)
	}
	if out != "" {
		t.Fatalf("empty host should skip all bindings -> empty base64, got %q", out)
	}
}
