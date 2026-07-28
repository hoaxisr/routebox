package config

import (
	"strings"
	"testing"
)

// Issue #37: a mieru server can bind port ranges (the fork's inbound gained
// listen_ports, mirroring the outbound's server_ports). The panel must accept
// them — including the "ranges only, no single port" shape the client needs when
// it rotates across a range.
func mieruInboundWith(extra map[string]interface{}) map[string]interface{} {
	ib := map[string]interface{}{
		"type": "mieru", "tag": "mieru-in", "listen": "::",
		"transport": "TCP",
		"users":     []interface{}{map[string]interface{}{"name": "a", "password": "p"}},
	}
	for k, v := range extra {
		ib[k] = v
	}
	return ib
}

func TestValidateMieruInboundListenPorts(t *testing.T) {
	cases := []struct {
		name    string
		extra   map[string]interface{}
		wantErr string // substring; "" = must validate cleanly
	}{
		{
			name:  "single port only, as before",
			extra: map[string]interface{}{"listen_port": float64(2020)},
		},
		{
			name:  "single port plus a range",
			extra: map[string]interface{}{"listen_port": float64(2020), "listen_ports": []interface{}{"25010-25012"}},
		},
		{
			name:  "ranges only, no single port",
			extra: map[string]interface{}{"listen_ports": []interface{}{"25010-25012"}},
		},
		{
			name:  "several ranges",
			extra: map[string]interface{}{"listen_ports": []interface{}{"2000-2100", "3000-3100"}},
		},
		{
			name:    "neither port nor ranges",
			extra:   map[string]interface{}{},
			wantErr: "listen_port",
		},
		{
			name:    "a bare port is not a range",
			extra:   map[string]interface{}{"listen_ports": []interface{}{"8443"}},
			wantErr: "listen_ports",
		},
		{
			name:    "reversed range",
			extra:   map[string]interface{}{"listen_ports": []interface{}{"25012-25010"}},
			wantErr: "listen_ports",
		},
		{
			name:    "out of range",
			extra:   map[string]interface{}{"listen_ports": []interface{}{"1-70000"}},
			wantErr: "listen_ports",
		},
		{
			name:    "malformed",
			extra:   map[string]interface{}{"listen_ports": []interface{}{"abc"}},
			wantErr: "listen_ports",
		},
		{
			name:    "listen_ports must be a list",
			extra:   map[string]interface{}{"listen_ports": "25010-25012"},
			wantErr: "listen_ports",
		},
		{
			name:    "an out-of-range single port is still rejected",
			extra:   map[string]interface{}{"listen_port": float64(70000)},
			wantErr: "listen_port",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateMieruInbound(mieruInboundWith(tc.extra), "inbounds[0]")
			joined := strings.Join(errs, "; ")
			if tc.wantErr == "" {
				if len(errs) != 0 {
					t.Fatalf("expected no errors, got %s", joined)
				}
				return
			}
			if !strings.Contains(joined, tc.wantErr) {
				t.Fatalf("expected an error mentioning %q, got %q", tc.wantErr, joined)
			}
		})
	}
}

// Too many ranges is the same hostile-input guard the outbound already has.
func TestValidateMieruInboundListenPortsCap(t *testing.T) {
	many := make([]interface{}, mieruMaxPortRanges+1)
	for i := range many {
		many[i] = "2000-2001"
	}
	errs := validateMieruInbound(mieruInboundWith(map[string]interface{}{"listen_ports": many}), "inbounds[0]")
	if !strings.Contains(strings.Join(errs, "; "), "too many") {
		t.Fatalf("expected a cap error, got %v", errs)
	}
}
