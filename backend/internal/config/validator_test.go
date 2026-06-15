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
