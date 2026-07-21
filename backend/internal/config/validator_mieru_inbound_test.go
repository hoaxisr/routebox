package config

import (
	"strings"
	"testing"
)

func mieruInbound() map[string]interface{} {
	return map[string]interface{}{
		"type":        "mieru",
		"tag":         "mieru-in",
		"listen":      "::",
		"listen_port": float64(2020),
		"transport":   "TCP",
		"users": []interface{}{
			map[string]interface{}{"name": "alice", "password": "pw1"},
		},
	}
}

func TestValidateMieruInbound(t *testing.T) {
	// A well-formed mieru inbound passes. validateInbound takes an INT index
	// (it builds the "inbounds[N]" prefix internally), NOT a prefix string.
	if errs := validateInbound(mieruInbound(), 0); len(errs) != 0 {
		t.Fatalf("valid mieru inbound rejected: %v", errs)
	}

	bad := map[string]func(map[string]interface{}){
		"missing listen_port":    func(m map[string]interface{}) { delete(m, "listen_port") },
		"zero listen_port":       func(m map[string]interface{}) { m["listen_port"] = float64(0) },
		"fractional listen_port": func(m map[string]interface{}) { m["listen_port"] = float64(2020.5) },
		"missing transport":      func(m map[string]interface{}) { delete(m, "transport") },
		"bad transport":          func(m map[string]interface{}) { m["transport"] = "QUIC" },
		"empty users":            func(m map[string]interface{}) { m["users"] = []interface{}{} },
		"user no name": func(m map[string]interface{}) {
			m["users"] = []interface{}{map[string]interface{}{"name": "", "password": "p"}}
		},
		"user no password": func(m map[string]interface{}) {
			m["users"] = []interface{}{map[string]interface{}{"name": "a", "password": ""}}
		},
		"duplicate user name": func(m map[string]interface{}) {
			m["users"] = []interface{}{
				map[string]interface{}{"name": "dup", "password": "p1"},
				map[string]interface{}{"name": "dup", "password": "p2"},
			}
		},
		// mieru has no TLS and no multiplexing; the raw-config PUT path can carry
		// them — reject BEFORE apply (spec: named message, not an opaque fork error).
		"stray tls block":    func(m map[string]interface{}) { m["tls"] = map[string]interface{}{"enabled": true} },
		"stray multiplexing": func(m map[string]interface{}) { m["multiplexing"] = "MULTIPLEXING_LOW" },
	}
	for label, mutate := range bad {
		m := mieruInbound()
		mutate(m)
		if errs := validateInbound(m, 0); len(errs) == 0 {
			t.Errorf("%s: expected rejection, got none", label)
		}
	}

	// RouteBox join key for mieru is the PASSWORD (CredentialKey=="password");
	// removeUserFromDraft filters by password, so two distinct-name users with
	// the same password corrupt reconcile even though the FORK allows it. The
	// error must NOT echo the secret value (security property, pinned here).
	dup := mieruInbound()
	dup["users"] = []interface{}{
		map[string]interface{}{"name": "a", "password": "same"},
		map[string]interface{}{"name": "b", "password": "same"},
	}
	dupErrs := validateInbound(dup, 0)
	if len(dupErrs) == 0 {
		t.Errorf("duplicate user password: expected rejection, got none")
	}
	if joined := strings.Join(dupErrs, ";"); strings.Contains(joined, "same") {
		t.Errorf("duplicate-password error echoed the password value: %v", dupErrs)
	}

	// mieru must NOT require TLS.
	m := mieruInbound() // no tls key at all
	if errs := validateInbound(m, 0); len(errs) != 0 {
		t.Errorf("mieru inbound without tls should pass, got: %v", errs)
	}
}
