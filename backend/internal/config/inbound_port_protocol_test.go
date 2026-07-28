package config

import (
	"strings"
	"testing"
)

// Issue #37: 443 is the port that gets through everywhere, so operators want a
// QUIC/UDP inbound there ALONGSIDE the TCP one. The conflict check only compared
// address and port, so the panel refused — even though the two bind different
// sockets and coexist fine. It must still refuse a genuine double bind.
func TestListenPortConflictIsProtocolAware(t *testing.T) {
	inbound := func(typ, tag string, port float64, extra map[string]interface{}) map[string]interface{} {
		m := map[string]interface{}{"type": typ, "tag": tag, "listen": "::", "listen_port": port}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	udpMieru := map[string]interface{}{"transport": "UDP"}
	tcpMieru := map[string]interface{}{"transport": "TCP"}

	cases := []struct {
		name       string
		existing   map[string]interface{}
		candidate  map[string]interface{}
		wantErrror bool
	}{
		{
			name:      "UDP mieru may share 443 with TCP vless",
			existing:  inbound("vless", "vless-in", 443, nil),
			candidate: inbound("mieru", "mieru-in", 443, udpMieru),
		},
		{
			name:      "hysteria2 may share 443 with TCP trojan",
			existing:  inbound("trojan", "trojan-in", 443, nil),
			candidate: inbound("hysteria2", "hy2-in", 443, nil),
		},
		{
			name:       "two UDP inbounds on one port still collide",
			existing:   inbound("hysteria2", "hy2-in", 443, nil),
			candidate:  inbound("mieru", "mieru-in", 443, udpMieru),
			wantErrror: true,
		},
		{
			name:       "two TCP inbounds on one port still collide",
			existing:   inbound("vless", "vless-in", 443, nil),
			candidate:  inbound("trojan", "trojan-in", 443, nil),
			wantErrror: true,
		},
		{
			name:       "TCP mieru collides with a TCP inbound",
			existing:   inbound("vless", "vless-in", 443, nil),
			candidate:  inbound("mieru", "mieru-in", 443, tcpMieru),
			wantErrror: true,
		},
		{
			name:      "TCP mieru may share a port with a UDP inbound",
			existing:  inbound("hysteria2", "hy2-in", 443, nil),
			candidate: inbound("mieru", "mieru-in", 443, tcpMieru),
		},
		{
			// Nothing is known about a type the table has not been taught, and a
			// wrong "no conflict" here is a crash on reload — so it conflicts.
			name:       "an unknown type is assumed to bind both",
			existing:   inbound("vless", "vless-in", 443, nil),
			candidate:  inbound("some-future-protocol", "x-in", 443, nil),
			wantErrror: true,
		},
		{
			name:       "mieru with no transport is assumed to bind both",
			existing:   inbound("vless", "vless-in", 443, nil),
			candidate:  inbound("mieru", "mieru-in", 443, nil),
			wantErrror: true,
		},
		{
			name:      "different ports never collide",
			existing:  inbound("vless", "vless-in", 443, nil),
			candidate: inbound("trojan", "trojan-in", 8443, nil),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := listenPortConflict([]interface{}{tc.existing}, tc.candidate, "")
			if tc.wantErrror && err == nil {
				t.Fatalf("expected a conflict, got none")
			}
			if !tc.wantErrror && err != nil {
				t.Fatalf("expected no conflict, got %v", err)
			}
		})
	}
}

// The same rule must hold on the full-config path (raw PUT /api/config), which
// runs its own cross-inbound scan rather than listenPortConflict.
func TestValidateConfigPortConflictIsProtocolAware(t *testing.T) {
	cfg := func(inbounds ...map[string]interface{}) map[string]interface{} {
		arr := make([]interface{}, len(inbounds))
		for i, ib := range inbounds {
			arr[i] = ib
		}
		return map[string]interface{}{
			"inbounds":  arr,
			"outbounds": []interface{}{map[string]interface{}{"type": "direct", "tag": "direct"}},
		}
	}
	vless := map[string]interface{}{
		"type": "vless", "tag": "vless-in", "listen": "::", "listen_port": float64(443),
		"users": []interface{}{map[string]interface{}{"name": "a", "uuid": "11111111-1111-1111-1111-111111111111"}},
	}
	hy2 := map[string]interface{}{
		"type": "hysteria2", "tag": "hy2-in", "listen": "::", "listen_port": float64(443),
		"users": []interface{}{map[string]interface{}{"name": "a", "password": "p"}},
	}
	tuic := map[string]interface{}{
		"type": "tuic", "tag": "tuic-in", "listen": "::", "listen_port": float64(443),
		"users": []interface{}{map[string]interface{}{"name": "a", "uuid": "11111111-1111-1111-1111-111111111111"}},
	}

	m := newManagerWithRules(t) // any loaded Manager: Validate is config-driven
	shareErr := func(errs []string) string {
		for _, e := range errs {
			if strings.Contains(e, "share listen") {
				return e
			}
		}
		return ""
	}

	if got := shareErr(m.Validate(cfg(vless, hy2))); got != "" {
		t.Fatalf("TCP + QUIC on 443 must be allowed, got %q", got)
	}
	if got := shareErr(m.Validate(cfg(hy2, tuic))); got == "" {
		t.Fatal("two QUIC inbounds on 443 must still conflict")
	}
}

func TestInboundNetworks(t *testing.T) {
	both := map[string]bool{"tcp": true, "udp": true}
	cases := []struct {
		inbound map[string]interface{}
		want    map[string]bool
	}{
		{map[string]interface{}{"type": "vless"}, map[string]bool{"tcp": true}},
		{map[string]interface{}{"type": "trojan"}, map[string]bool{"tcp": true}},
		{map[string]interface{}{"type": "naive"}, map[string]bool{"tcp": true}},
		{map[string]interface{}{"type": "hysteria2"}, map[string]bool{"udp": true}},
		{map[string]interface{}{"type": "tuic"}, map[string]bool{"udp": true}},
		{map[string]interface{}{"type": "mieru", "transport": "UDP"}, map[string]bool{"udp": true}},
		{map[string]interface{}{"type": "mieru", "transport": "TCP"}, map[string]bool{"tcp": true}},
		{map[string]interface{}{"type": "mieru"}, both},
		{map[string]interface{}{"type": "shadowsocks"}, both},
		{map[string]interface{}{"type": "whatever"}, both},
	}
	for _, tc := range cases {
		got := inboundNetworks(tc.inbound)
		if len(got) != len(tc.want) {
			t.Fatalf("inboundNetworks(%v) = %v, want %v", tc.inbound, got, tc.want)
		}
		for k := range tc.want {
			if !got[k] {
				t.Fatalf("inboundNetworks(%v) = %v, want %v", tc.inbound, got, tc.want)
			}
		}
	}
}
