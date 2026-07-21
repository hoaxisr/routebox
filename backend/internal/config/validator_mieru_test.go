package config

import (
	"strings"
	"testing"
)

func mieruOB(mut func(m map[string]interface{})) map[string]interface{} {
	m := map[string]interface{}{
		"type": "mieru", "tag": "m", "server": "h", "server_port": float64(443),
		"transport": "TCP", "username": "u", "password": "p",
	}
	if mut != nil {
		mut(m)
	}
	return m
}

func TestValidateOutbound_Mieru(t *testing.T) {
	if errs := validateOutbound(mieruOB(nil), 0); len(errs) != 0 {
		t.Fatalf("valid mieru rejected: %v", errs)
	}
	// server_ports dash range accepted (server_port absent).
	if errs := validateOutbound(mieruOB(func(m map[string]interface{}) {
		delete(m, "server_port")
		m["server_ports"] = []interface{}{"9000-9010"}
	}), 0); len(errs) != 0 {
		t.Fatalf("dash range rejected: %v", errs)
	}
	// server_port 0 + non-empty server_ports is fork-valid (0 = unset).
	if errs := validateOutbound(mieruOB(func(m map[string]interface{}) {
		m["server_port"] = float64(0)
		m["server_ports"] = []interface{}{"9000-9010"}
	}), 0); len(errs) != 0 {
		t.Fatalf("zero server_port with ranges rejected: %v", errs)
	}
	// Valid traffic_pattern accepted.
	if errs := validateOutbound(mieruOB(func(m map[string]interface{}) {
		m["traffic_pattern"] = "YQ+b"
	}), 0); len(errs) != 0 {
		t.Fatalf("valid traffic_pattern rejected: %v", errs)
	}

	manyRanges := make([]interface{}, 65)
	for i := range manyRanges {
		manyRanges[i] = "9000-9010"
	}

	bad := map[string]func(map[string]interface{}){
		"missing username":    func(m map[string]interface{}) { delete(m, "username") },
		"missing password":    func(m map[string]interface{}) { delete(m, "password") },
		"missing server":      func(m map[string]interface{}) { delete(m, "server") },
		"no ports":            func(m map[string]interface{}) { delete(m, "server_port") },
		"server_port 70000":   func(m map[string]interface{}) { m["server_port"] = float64(70000) },
		"bad transport":       func(m map[string]interface{}) { m["transport"] = "SCTP" },
		"missing transport":   func(m map[string]interface{}) { delete(m, "transport") },
		"bare server_ports":   func(m map[string]interface{}) { delete(m, "server_port"); m["server_ports"] = []interface{}{"8443"} },
		"reversed range":      func(m map[string]interface{}) { delete(m, "server_port"); m["server_ports"] = []interface{}{"9010-9000"} },
		"out-of-bounds range": func(m map[string]interface{}) { delete(m, "server_port"); m["server_ports"] = []interface{}{"0-99999"} },
		"too many ranges":     func(m map[string]interface{}) { delete(m, "server_port"); m["server_ports"] = manyRanges },
		"bad multiplexing":    func(m map[string]interface{}) { m["multiplexing"] = "BOGUS" },
		"bad traffic":         func(m map[string]interface{}) { m["traffic_pattern"] = "not base64 !!!" },
		"unpadded traffic":    func(m map[string]interface{}) { m["traffic_pattern"] = "YQ" },
		"oversize traffic":    func(m map[string]interface{}) { m["traffic_pattern"] = strings.Repeat("A", 90000) },
	}
	for name, mut := range bad {
		if errs := validateOutbound(mieruOB(mut), 0); len(errs) == 0 {
			t.Errorf("%s: expected rejection", name)
		}
	}
}
