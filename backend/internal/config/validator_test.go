package config

import (
	"strings"
	"testing"
)

func hasErrContaining(errs []string, sub string) bool {
	for _, e := range errs {
		if strings.Contains(e, sub) {
			return true
		}
	}
	return false
}

func TestValidateInboundServerTypes(t *testing.T) {
	tests := []struct {
		name    string
		inbound map[string]interface{}
		wantSub string // substring that must appear; "" means must be valid
	}{
		{
			name: "vless missing port",
			inbound: map[string]interface{}{
				"type": "vless", "tag": "v",
				"users": []interface{}{map[string]interface{}{"uuid": "u1"}},
			},
			wantSub: "listen_port",
		},
		{
			name: "naive without tls object",
			inbound: map[string]interface{}{
				"type": "naive", "tag": "n", "listen_port": float64(443),
				"users": []interface{}{map[string]interface{}{"username": "a", "password": "p"}},
			},
			wantSub: "TLS",
		},
		{
			// MUST-FIX 4: tls present but disabled must still fail.
			name: "naive with tls.enabled false",
			inbound: map[string]interface{}{
				"type": "naive", "tag": "n", "listen_port": float64(443),
				"tls":   map[string]interface{}{"enabled": false},
				"users": []interface{}{map[string]interface{}{"username": "a", "password": "p"}},
			},
			wantSub: "TLS",
		},
		{
			name: "hysteria2 with tls.enabled false",
			inbound: map[string]interface{}{
				"type": "hysteria2", "tag": "h", "listen_port": float64(443),
				"tls":   map[string]interface{}{"enabled": false},
				"users": []interface{}{map[string]interface{}{"password": "p"}},
			},
			wantSub: "TLS",
		},
		{
			name: "vless duplicate uuid",
			inbound: map[string]interface{}{
				"type": "vless", "tag": "v", "listen_port": float64(443),
				"tls": map[string]interface{}{"enabled": true},
				"users": []interface{}{
					map[string]interface{}{"uuid": "dup"},
					map[string]interface{}{"uuid": "dup"},
				},
			},
			wantSub: "duplicate",
		},
		{
			name: "naive duplicate username",
			inbound: map[string]interface{}{
				"type": "naive", "tag": "n", "listen_port": float64(443),
				"tls": map[string]interface{}{"enabled": true},
				"users": []interface{}{
					map[string]interface{}{"username": "same", "password": "p1"},
					map[string]interface{}{"username": "same", "password": "p2"},
				},
			},
			wantSub: "duplicate",
		},
		{
			name: "valid vless reality",
			inbound: map[string]interface{}{
				"type": "vless", "tag": "v", "listen_port": float64(443),
				"tls":   map[string]interface{}{"enabled": true, "reality": map[string]interface{}{"enabled": true}},
				"users": []interface{}{map[string]interface{}{"uuid": "u1"}},
			},
			wantSub: "",
		},
		{
			name: "valid hysteria2 with tls enabled",
			inbound: map[string]interface{}{
				"type": "hysteria2", "tag": "h", "listen_port": float64(443),
				"tls":   map[string]interface{}{"enabled": true},
				"users": []interface{}{map[string]interface{}{"password": "p1"}},
			},
			wantSub: "",
		},
		{
			// Guard: vless must NOT require TLS — plain/Reality are valid without
			// a tls object. If this fails, the validator over-restricts vless.
			name: "valid vless without any tls",
			inbound: map[string]interface{}{
				"type": "vless", "tag": "v", "listen_port": float64(443),
				"users": []interface{}{map[string]interface{}{"uuid": "u1"}},
			},
			wantSub: "",
		},
		{
			name: "hysteria2 duplicate password",
			inbound: map[string]interface{}{
				"type": "hysteria2", "tag": "h", "listen_port": float64(443),
				"tls": map[string]interface{}{"enabled": true},
				"users": []interface{}{
					map[string]interface{}{"password": "dup"},
					map[string]interface{}{"password": "dup"},
				},
			},
			wantSub: "duplicate",
		},
		{
			name: "naive missing port",
			inbound: map[string]interface{}{
				"type": "naive", "tag": "n",
				"tls":   map[string]interface{}{"enabled": true},
				"users": []interface{}{map[string]interface{}{"username": "a", "password": "p"}},
			},
			wantSub: "listen_port",
		},
		{
			name: "hysteria2 missing port",
			inbound: map[string]interface{}{
				"type": "hysteria2", "tag": "h",
				"tls":   map[string]interface{}{"enabled": true},
				"users": []interface{}{map[string]interface{}{"password": "p"}},
			},
			wantSub: "listen_port",
		},
		{
			// Guard against an over-aggressive dup check: distinct usernames are fine.
			name: "naive distinct usernames valid",
			inbound: map[string]interface{}{
				"type": "naive", "tag": "n", "listen_port": float64(443),
				"tls": map[string]interface{}{"enabled": true},
				"users": []interface{}{
					map[string]interface{}{"username": "alice", "password": "p1"},
					map[string]interface{}{"username": "bob", "password": "p2"},
				},
			},
			wantSub: "",
		},
		{
			// Empty/absent users is valid (no port/TLS issues either).
			name: "vless no users valid",
			inbound: map[string]interface{}{
				"type": "vless", "tag": "v", "listen_port": float64(443),
				"tls": map[string]interface{}{"enabled": true},
			},
			wantSub: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateInbound(tt.inbound, 0)
			if tt.wantSub == "" {
				if len(errs) != 0 {
					t.Fatalf("expected valid, got %v", errs)
				}
				return
			}
			if !hasErrContaining(errs, tt.wantSub) {
				t.Fatalf("expected error containing %q, got %v", tt.wantSub, errs)
			}
		})
	}
}

func TestValidateInboundTransportVisionConflict(t *testing.T) {
	m := &Manager{}
	// vless + ws transport + a user with xtls-rprx-vision flow.
	// amnezia-box `check` PASSES this; RouteBox MUST reject it.
	cfg := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"type": "vless", "tag": "v", "listen_port": float64(443),
				"tls":       map[string]interface{}{"reality": map[string]interface{}{"enabled": true}},
				"transport": map[string]interface{}{"type": "ws", "path": "/"},
				"users":     []interface{}{map[string]interface{}{"uuid": "u1", "flow": "xtls-rprx-vision"}},
			},
		},
		"outbounds": []interface{}{},
	}
	if errs := m.Validate(cfg); !hasErrContaining(errs, "xtls-rprx-vision flow requires raw transport") {
		t.Fatalf("expected vision+transport rejection, got: %v", errs)
	}
}

func TestValidateInboundTransportRawVisionOK(t *testing.T) {
	m := &Manager{}
	cfg := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"type": "vless", "tag": "v", "listen_port": float64(443),
				"tls":   map[string]interface{}{"reality": map[string]interface{}{"enabled": true}},
				"users": []interface{}{map[string]interface{}{"uuid": "u1", "flow": "xtls-rprx-vision"}},
			},
		},
		"outbounds": []interface{}{},
	}
	if errs := m.Validate(cfg); hasErrContaining(errs, "requires raw transport") {
		t.Fatalf("raw+vision must be allowed, got: %v", errs)
	}
}

func TestValidateInboundTransportBadType(t *testing.T) {
	m := &Manager{}
	cfg := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"type": "vless", "tag": "v", "listen_port": float64(443),
				"tls":       map[string]interface{}{"reality": map[string]interface{}{"enabled": true}},
				"transport": map[string]interface{}{"type": "quic"},
				"users":     []interface{}{map[string]interface{}{"uuid": "u1"}},
			},
		},
		"outbounds": []interface{}{},
	}
	if errs := m.Validate(cfg); !hasErrContaining(errs, "transport type") {
		t.Fatalf("expected transport-type rejection, got: %v", errs)
	}
}

func TestValidateInboundTransportXHTTPRejected(t *testing.T) {
	m := &Manager{}
	cfg := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"type": "vless", "tag": "v", "listen_port": float64(443),
				"tls":       map[string]interface{}{"reality": map[string]interface{}{"enabled": true}},
				"transport": map[string]interface{}{"type": "xhttp", "path": "/x", "host": "cdn.ex.com", "x_padding_bytes": XHTTPDefaultPadding},
				"users":     []interface{}{map[string]interface{}{"uuid": "u1"}},
			},
		},
		"outbounds": []interface{}{},
	}
	// This test used to assert the opposite. amnezia-box implements xhttp for
	// outbounds only — transport/v2rayxhttp has no server — so an inbound using
	// it fails at startup, and the panel has to say so before apply.
	if errs := m.Validate(cfg); !hasErrContaining(errs, "xhttp cannot be used on an inbound") {
		t.Fatalf("xhttp inbound should be rejected, got: %v", errs)
	}
}

func TestValidateTrojanInboundReality(t *testing.T) {
	m := &Manager{}
	cfg := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"type": "trojan", "tag": "tr", "listen_port": float64(443),
				"tls":   map[string]interface{}{"reality": map[string]interface{}{"enabled": true}},
				"users": []interface{}{map[string]interface{}{"name": "p", "password": "pw"}},
			},
		},
		"outbounds": []interface{}{},
	}
	if errs := m.Validate(cfg); len(errs) != 0 {
		t.Fatalf("trojan+reality should be valid, got: %v", errs)
	}
}

func TestValidateTrojanInboundAcme(t *testing.T) {
	m := &Manager{}
	cfg := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"type": "trojan", "tag": "tr", "listen_port": float64(443),
				"tls":   map[string]interface{}{"enabled": true, "acme": map[string]interface{}{"domain": "ex.com"}},
				"users": []interface{}{map[string]interface{}{"name": "p", "password": "pw"}},
			},
		},
		"outbounds": []interface{}{},
	}
	if errs := m.Validate(cfg); len(errs) != 0 {
		t.Fatalf("trojan+acme should be valid, got: %v", errs)
	}
}

func TestValidateTrojanInboundMissingPort(t *testing.T) {
	m := &Manager{}
	cfg := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"type": "trojan", "tag": "tr",
				"tls":   map[string]interface{}{"enabled": true},
				"users": []interface{}{map[string]interface{}{"name": "p", "password": "pw"}},
			},
		},
		"outbounds": []interface{}{},
	}
	if errs := m.Validate(cfg); !hasErrContaining(errs, "listen_port") {
		t.Fatalf("expected listen_port rejection, got: %v", errs)
	}
}

func TestValidateTrojanInboundNeedsTls(t *testing.T) {
	m := &Manager{}
	cfg := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"type": "trojan", "tag": "tr", "listen_port": float64(443),
				"users": []interface{}{map[string]interface{}{"name": "p", "password": "pw"}},
			},
		},
		"outbounds": []interface{}{},
	}
	if errs := m.Validate(cfg); !hasErrContaining(errs, "requires TLS") {
		t.Fatalf("trojan without tls/reality must be rejected, got: %v", errs)
	}
}

func TestValidateTrojanInboundDupPassword(t *testing.T) {
	m := &Manager{}
	cfg := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"type": "trojan", "tag": "tr", "listen_port": float64(443),
				"tls": map[string]interface{}{"reality": map[string]interface{}{"enabled": true}},
				"users": []interface{}{
					map[string]interface{}{"name": "a", "password": "dup"},
					map[string]interface{}{"name": "b", "password": "dup"},
				},
			},
		},
		"outbounds": []interface{}{},
	}
	if errs := m.Validate(cfg); !hasErrContaining(errs, "duplicate") {
		t.Fatalf("expected duplicate password rejection, got: %v", errs)
	}
}

func TestValidateInboundRejectsUtls(t *testing.T) {
	m := &Manager{}
	cfg := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"type": "trojan", "tag": "tr", "listen_port": float64(443),
				"tls": map[string]interface{}{
					"enabled": true, "acme": map[string]interface{}{"domain": "ex.com"},
					"utls": map[string]interface{}{"enabled": true, "fingerprint": "chrome"},
				},
				"users": []interface{}{map[string]interface{}{"name": "p", "password": "pw"}},
			},
		},
		"outbounds": []interface{}{},
	}
	if errs := m.Validate(cfg); !hasErrContaining(errs, "utls") {
		t.Fatalf("inbound tls.utls must be rejected (outbound-only), got: %v", errs)
	}
}

func TestValidateOutboundTrojan(t *testing.T) {
	tests := []struct {
		name    string
		ob      map[string]interface{}
		wantSub string
	}{
		{"valid trojan tls", map[string]interface{}{"type": "trojan", "tag": "t", "server": "ex.com", "server_port": float64(443), "password": "pw", "tls": map[string]interface{}{"enabled": true}}, ""},
		{"valid trojan no tls (plaintext allowed on outbound)", map[string]interface{}{"type": "trojan", "tag": "t", "server": "ex.com", "server_port": float64(443), "password": "pw"}, ""},
		{"trojan missing server", map[string]interface{}{"type": "trojan", "tag": "t", "server_port": float64(443), "password": "pw"}, "server"},
		{"trojan missing port", map[string]interface{}{"type": "trojan", "tag": "t", "server": "ex.com", "password": "pw"}, "server_port"},
		{"trojan missing password", map[string]interface{}{"type": "trojan", "tag": "t", "server": "ex.com", "server_port": float64(443)}, "password"},
		{"trojan httpupgrade transport accepted", map[string]interface{}{"type": "trojan", "tag": "t", "server": "ex.com", "server_port": float64(443), "password": "pw", "transport": map[string]interface{}{"type": "httpupgrade", "path": "/x", "host": "cdn.ex.com"}}, ""},
		{"vless httpupgrade transport accepted", map[string]interface{}{"type": "vless", "tag": "v", "server": "ex.com", "server_port": float64(443), "uuid": "u1", "transport": map[string]interface{}{"type": "httpupgrade", "path": "/x", "host": "cdn.ex.com"}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateOutbound(tt.ob, 0)
			if tt.wantSub == "" {
				if len(errs) != 0 {
					t.Fatalf("expected valid, got %v", errs)
				}
				return
			}
			if !hasErrContaining(errs, tt.wantSub) {
				t.Fatalf("expected error containing %q, got %v", tt.wantSub, errs)
			}
		})
	}
}

// TestValidateOutboundVlessVisionConflict mirrors the inbound Vision guard onto
// the client outbound side. A vless outbound with a non-raw transport plus
// flow == xtls-rprx-vision PASSES amnezia-box `check` but fails per-connection at
// runtime ("vision: not a valid supported TLS connection") — RouteBox is the only
// config-time guard.
func TestValidateOutboundVlessVisionConflict(t *testing.T) {
	tests := []struct {
		name    string
		ob      map[string]interface{}
		wantSub string
	}{
		{"vless ws + vision rejected", map[string]interface{}{"type": "vless", "tag": "v", "server": "ex.com", "server_port": float64(443), "uuid": "u1", "flow": "xtls-rprx-vision", "transport": map[string]interface{}{"type": "ws", "path": "/"}}, "xtls-rprx-vision flow requires raw transport"},
		{"vless raw + vision ok", map[string]interface{}{"type": "vless", "tag": "v", "server": "ex.com", "server_port": float64(443), "uuid": "u1", "flow": "xtls-rprx-vision"}, ""},
		{"vless tcp transport + vision ok", map[string]interface{}{"type": "vless", "tag": "v", "server": "ex.com", "server_port": float64(443), "uuid": "u1", "flow": "xtls-rprx-vision", "transport": map[string]interface{}{"type": "tcp"}}, ""},
		{"vless ws + no flow ok", map[string]interface{}{"type": "vless", "tag": "v", "server": "ex.com", "server_port": float64(443), "uuid": "u1", "transport": map[string]interface{}{"type": "ws", "path": "/"}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateOutbound(tt.ob, 0)
			if tt.wantSub == "" {
				if hasErrContaining(errs, "requires raw transport") {
					t.Fatalf("expected no vision conflict, got %v", errs)
				}
				return
			}
			if !hasErrContaining(errs, tt.wantSub) {
				t.Fatalf("expected error containing %q, got %v", tt.wantSub, errs)
			}
		})
	}
}

// TestValidateInboundMalformedUsers ensures a malformed users slice — a non-map
// element alongside a blank-credential user — does not panic. Errors may or may
// not be present; the point is panic-safety (non-map entries are skipped).
func TestValidateInboundMalformedUsers(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "vless", "tag": "v", "listen_port": float64(443),
		"tls": map[string]interface{}{"enabled": true},
		"users": []interface{}{
			"not-a-map",
			map[string]interface{}{"uuid": ""},
		},
	}
	// validateInbound must return without crashing.
	_ = validateInbound(inbound, 0)
}

func TestValidateRejectsDuplicateListenPort(t *testing.T) {
	m := &Manager{}
	cfg := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{"type": "naive", "tag": "naive-in", "listen_port": float64(443),
				"users": []interface{}{map[string]interface{}{"username": "a", "password": "p"}},
				"tls":   map[string]interface{}{"enabled": true, "acme": map[string]interface{}{"domain": "x.example.com", "email": "a@b.c"}}},
			map[string]interface{}{"type": "vless", "tag": "vless-in", "listen_port": float64(443),
				"users": []interface{}{map[string]interface{}{"name": "a", "uuid": "11111111-1111-1111-1111-111111111111"}},
				"tls":   map[string]interface{}{"enabled": true, "reality": map[string]interface{}{"enabled": true, "handshake": map[string]interface{}{"server": "g.com", "server_port": float64(443)}, "private_key": "k", "short_id": []interface{}{"00"}}}},
		},
	}
	errs := m.Validate(cfg)
	if !hasErrContaining(errs, "443") || !hasErrContaining(errs, "ports must be unique") {
		t.Fatalf("want duplicate-port error mentioning 443; got %v", errs)
	}
}

func TestValidateAllowsDistinctPortsAndListens(t *testing.T) {
	m := &Manager{}
	mk := func(tag string, port float64, listen string) map[string]interface{} {
		o := map[string]interface{}{"type": "vless", "tag": tag, "listen_port": port,
			"users": []interface{}{map[string]interface{}{"name": "a", "uuid": "11111111-1111-1111-1111-111111111111"}},
			"tls":   map[string]interface{}{"enabled": true, "reality": map[string]interface{}{"enabled": true, "handshake": map[string]interface{}{"server": "g.com", "server_port": float64(443)}, "private_key": "k", "short_id": []interface{}{"00"}}}}
		if listen != "" {
			o["listen"] = listen
		}
		return o
	}
	if errs := m.Validate(map[string]interface{}{"inbounds": []interface{}{mk("a", 443, ""), mk("b", 8443, "")}}); hasErrContaining(errs, "ports must be unique") {
		t.Fatalf("distinct ports must not conflict: %v", errs)
	}
	if errs := m.Validate(map[string]interface{}{"inbounds": []interface{}{mk("a", 443, "10.0.0.1"), mk("b", 443, "10.0.0.2")}}); hasErrContaining(errs, "ports must be unique") {
		t.Fatalf("same port on distinct listens must not conflict: %v", errs)
	}
}
