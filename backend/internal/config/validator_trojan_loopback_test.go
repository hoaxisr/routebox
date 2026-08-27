package config

import "testing"

// RouteBox refuses trojan-over-plaintext because the password would cross the
// wire in the clear. Behind the 443 front (ADR 0001) there is no wire: dest
// terminates TLS and forwards on loopback, so the rule must not fire there —
// otherwise the out-of-the-box config is one the panel cannot save.
func TestTrojanPlaintextAllowedOnLoopbackOnly(t *testing.T) {
	trojan := func(listen string) map[string]interface{} {
		return map[string]interface{}{
			"type": "trojan", "tag": "trojan-ws-in",
			"listen": listen, "listen_port": float64(8445),
			"users":     []interface{}{map[string]interface{}{"name": "owner", "password": "pw"}},
			"transport": map[string]interface{}{"type": "ws", "path": "/ws-4f2b8d"},
		}
	}
	for _, listen := range []string{"127.0.0.1", "::1", "localhost"} {
		if errs := validateInbound(trojan(listen), 0); len(errs) != 0 {
			t.Errorf("listen %q: plaintext trojan behind dest rejected: %v", listen, errs)
		}
	}
	for _, listen := range []string{"::", "0.0.0.0", "10.0.0.1", ""} {
		if errs := validateInbound(trojan(listen), 0); len(errs) == 0 {
			t.Errorf("listen %q: plaintext trojan reachable from outside must be rejected", listen)
		}
	}
}
