package config

import "testing"

func TestValidateTrustTunnelOutbound(t *testing.T) {
	ok := map[string]interface{}{"tag": "tt", "type": "trusttunnel", "server": "1.2.3.4", "server_port": float64(443),
		"username": "u", "password": "p", "tls": map[string]interface{}{"enabled": true, "server_name": "h"}}
	if errs := validateOutbound(ok, 0); len(errs) != 0 {
		t.Fatalf("valid trusttunnel rejected: %v", errs)
	}
	for _, missing := range []string{"server", "server_port", "username", "password", "tls"} {
		ob := map[string]interface{}{}
		for k, v := range ok {
			ob[k] = v
		}
		delete(ob, missing)
		if errs := validateOutbound(ob, 0); !hasErrContaining(errs, missing) {
			t.Errorf("without %s: %v", missing, errs)
		}
	}
	ob := map[string]interface{}{}
	for k, v := range ok {
		ob[k] = v
	}
	ob["tls"] = map[string]interface{}{"enabled": false}
	if errs := validateOutbound(ob, 0); !hasErrContaining(errs, "TLS") {
		t.Errorf("tls disabled: %v", errs)
	}
}
