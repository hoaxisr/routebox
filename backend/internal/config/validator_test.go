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
				"tls":  map[string]interface{}{"enabled": false},
				"users": []interface{}{map[string]interface{}{"username": "a", "password": "p"}},
			},
			wantSub: "TLS",
		},
		{
			name: "hysteria2 with tls.enabled false",
			inbound: map[string]interface{}{
				"type": "hysteria2", "tag": "h", "listen_port": float64(443),
				"tls":  map[string]interface{}{"enabled": false},
				"users": []interface{}{map[string]interface{}{"password": "p"}},
			},
			wantSub: "TLS",
		},
		{
			name: "vless duplicate uuid",
			inbound: map[string]interface{}{
				"type": "vless", "tag": "v", "listen_port": float64(443),
				"tls":  map[string]interface{}{"enabled": true},
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
				"tls":  map[string]interface{}{"enabled": true},
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
				"tls":  map[string]interface{}{"enabled": true, "reality": map[string]interface{}{"enabled": true}},
				"users": []interface{}{map[string]interface{}{"uuid": "u1"}},
			},
			wantSub: "",
		},
		{
			name: "valid hysteria2 with tls enabled",
			inbound: map[string]interface{}{
				"type": "hysteria2", "tag": "h", "listen_port": float64(443),
				"tls":  map[string]interface{}{"enabled": true},
				"users": []interface{}{map[string]interface{}{"password": "p1"}},
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
