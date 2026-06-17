package config

import "testing"

// CreateInbound must reject an inbound whose (listen, port) collides with an existing
// one — including the wildcard case where the panel sets listen "::" but the existing
// inbound omits listen (both bind the same port → amnezia-box crashes on reload).
func TestCreateInboundRejectsDuplicatePort(t *testing.T) {
	m := newManagerWithInbound(t) // vless-in, listen absent, port 443

	// "::" vs absent listen on the same port must collide.
	dup := map[string]interface{}{
		"type": "naive", "tag": "naive-NL", "listen": "::", "listen_port": float64(443),
		"tls":   map[string]interface{}{"enabled": true},
		"users": []interface{}{map[string]interface{}{"username": "u", "password": "p"}},
	}
	if err := m.CreateInbound(dup); err == nil {
		t.Fatal("expected duplicate-port rejection for :: vs absent listen on 443, got nil")
	}

	// A free port is fine.
	ok := map[string]interface{}{
		"type": "naive", "tag": "naive-NL", "listen": "::", "listen_port": float64(8443),
		"tls":   map[string]interface{}{"enabled": true},
		"users": []interface{}{map[string]interface{}{"username": "u", "password": "p"}},
	}
	if err := m.CreateInbound(ok); err != nil {
		t.Fatalf("expected free port 8443 to be accepted, got %v", err)
	}
}

// UpdateInbound must reject moving an inbound onto a port another inbound holds, but
// allow a no-op update of the same inbound (self is excluded from the check).
func TestUpdateInboundPortConflict(t *testing.T) {
	m := newManagerWithInbound(t) // vless-in on 443
	second := map[string]interface{}{
		"type": "trojan", "tag": "trojan-in", "listen": "::", "listen_port": float64(8443),
		"tls":   map[string]interface{}{"enabled": true},
		"users": []interface{}{map[string]interface{}{"name": "u", "password": "p"}},
	}
	if err := m.CreateInbound(second); err != nil {
		t.Fatalf("seed second inbound: %v", err)
	}

	// Move trojan-in onto 443 (held by vless-in) → reject.
	onto443 := map[string]interface{}{
		"type": "trojan", "tag": "trojan-in", "listen": "::", "listen_port": float64(443),
		"tls":   map[string]interface{}{"enabled": true},
		"users": []interface{}{map[string]interface{}{"name": "u", "password": "p"}},
	}
	if err := m.UpdateInbound("trojan-in", onto443); err == nil {
		t.Fatal("expected rejection moving trojan-in onto 443, got nil")
	}

	// Re-saving trojan-in on its own port (8443) must be allowed (self excluded).
	if err := m.UpdateInbound("trojan-in", second); err != nil {
		t.Fatalf("expected self-update on 8443 to be allowed, got %v", err)
	}
}

func TestNormalizeListenAddr(t *testing.T) {
	for _, w := range []string{"", "::", "[::]", "::0", "0.0.0.0", "*", "  ::  "} {
		if got := normalizeListenAddr(w); got != "*" {
			t.Errorf("normalizeListenAddr(%q) = %q, want *", w, got)
		}
	}
	if got := normalizeListenAddr("10.0.0.1"); got != "10.0.0.1" {
		t.Errorf("specific IP must be preserved, got %q", got)
	}
}
