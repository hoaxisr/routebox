package config

import (
	"strings"
	"testing"
)

func TestValidateEndpoint_AWGServerMode(t *testing.T) {
	// Server: listen_port present, peer has public_key + allowed_ips, no address/port.
	server := map[string]interface{}{
		"type": "awg", "tag": "awg-server",
		"private_key": "cGxhY2Vob2xkZXJwbGFjZWhvbGRlcnBsYWNlaG8=",
		"address":     []interface{}{"10.10.0.1/24"},
		"listen_port": float64(51820),
		"peers": []interface{}{
			map[string]interface{}{
				"public_key":  "cGxhY2Vob2xkZXJwbGFjZWhvbGRlcnBsYWNlaG8=",
				"allowed_ips": []interface{}{"10.10.0.2/32"},
			},
		},
	}
	if errs := validateEndpoint(server, 0); len(errs) != 0 {
		t.Fatalf("server endpoint should be valid, got %v", errs)
	}

	// Server with zero peers (no clients yet) is valid.
	empty := map[string]interface{}{
		"type": "awg", "tag": "awg-server",
		"private_key": "cGxhY2Vob2xkZXJwbGFjZWhvbGRlcnBsYWNlaG8=",
		"address":     []interface{}{"10.10.0.1/24"},
		"listen_port": float64(51820),
	}
	if errs := validateEndpoint(empty, 0); len(errs) != 0 {
		t.Fatalf("empty server endpoint should be valid, got %v", errs)
	}

	// Client (no listen_port) still REQUIRES peer address.
	client := map[string]interface{}{
		"type": "awg", "tag": "vpn",
		"private_key": "cGxhY2Vob2xkZXJwbGFjZWhvbGRlcnBsYWNlaG8=",
		"address":     []interface{}{"10.10.0.2/32"},
		"peers": []interface{}{
			map[string]interface{}{"public_key": "cGxhY2Vob2xkZXJwbGFjZWhvbGRlcnBsYWNlaG8="},
		},
	}
	errs := validateEndpoint(client, 0)
	found := false
	for _, e := range errs {
		if strings.Contains(e, "missing 'address'") {
			found = true
		}
	}
	if !found {
		t.Fatalf("client endpoint without peer address should error, got %v", errs)
	}
}

// The server marker must be a VALUE check, not a presence check: an explicit
// "listen_port": 0 or null is NOT a listener, so client strictness (peer
// address required) must still apply.
func TestValidateEndpoint_ListenPortValueCheck(t *testing.T) {
	for name, lp := range map[string]interface{}{"zero": float64(0), "null": nil} {
		client := map[string]interface{}{
			"type": "awg", "tag": "vpn",
			"private_key": "cGxhY2Vob2xkZXJwbGFjZWhvbGRlcnBsYWNlaG8=",
			"address":     []interface{}{"10.10.0.2/32"},
			"listen_port": lp,
			"peers": []interface{}{
				map[string]interface{}{"public_key": "cGxhY2Vob2xkZXJwbGFjZWhvbGRlcnBsYWNlaG8="},
			},
		}
		errs := validateEndpoint(client, 0)
		found := false
		for _, e := range errs {
			if strings.Contains(e, "missing 'address'") {
				found = true
			}
		}
		if !found {
			t.Fatalf("listen_port=%s: peer without address must error (client mode), got %v", name, errs)
		}
	}

	// And a client with listen_port 0 and NO peers at all must still require peers.
	noPeers := map[string]interface{}{
		"type": "awg", "tag": "vpn",
		"private_key": "cGxhY2Vob2xkZXJwbGFjZWhvbGRlcnBsYWNlaG8=",
		"address":     []interface{}{"10.10.0.2/32"},
		"listen_port": float64(0),
	}
	errs := validateEndpoint(noPeers, 0)
	found := false
	for _, e := range errs {
		if strings.Contains(e, "'peers'") {
			found = true
		}
	}
	if !found {
		t.Fatalf("listen_port=0 with no peers must error, got %v", errs)
	}
}
