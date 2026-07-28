package config

import (
	"strings"
	"testing"
)

// The fork decodes x_padding_bytes into a plain Range with no omitempty and no
// default, then rejects From<=0||To<=0 with "x_padding_bytes cannot be
// disabled". An xhttp transport without the field therefore fails to LOAD —
// reported as an opaque FATAL at apply time, long after the panel said the
// config was fine. Catch it here, by name.
func xhttpOutbound(transport map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type": "vless", "tag": "vless-out", "server": "example.com",
		"server_port": float64(443), "uuid": "11111111-1111-1111-1111-111111111111",
		"transport": transport,
	}
}

func TestValidateXHTTPPadding(t *testing.T) {
	cases := []struct {
		name      string
		transport map[string]interface{}
		wantErr   bool
	}{
		{
			name:      "xhttp without x_padding_bytes is rejected",
			transport: map[string]interface{}{"type": "xhttp", "path": "/"},
			wantErr:   true,
		},
		{
			name:      "a range string is accepted",
			transport: map[string]interface{}{"type": "xhttp", "path": "/", "x_padding_bytes": "100-1000"},
		},
		{
			name:      "a single number is accepted",
			transport: map[string]interface{}{"type": "xhttp", "path": "/", "x_padding_bytes": float64(1000)},
		},
		{
			name:      "a from/to object is accepted",
			transport: map[string]interface{}{"type": "xhttp", "path": "/", "x_padding_bytes": map[string]interface{}{"from": float64(100), "to": float64(1000)}},
		},
		{
			name:      "an explicit zero is rejected — that is what 'disabled' means",
			transport: map[string]interface{}{"type": "xhttp", "path": "/", "x_padding_bytes": float64(0)},
			wantErr:   true,
		},
		{
			name:      "a zero range is rejected",
			transport: map[string]interface{}{"type": "xhttp", "path": "/", "x_padding_bytes": "0-0"},
			wantErr:   true,
		},
		{
			name:      "a range starting at zero is rejected",
			transport: map[string]interface{}{"type": "xhttp", "path": "/", "x_padding_bytes": "0-1000"},
			wantErr:   true,
		},
		{
			// Other transports have no such field and must not be touched.
			name:      "ws needs nothing",
			transport: map[string]interface{}{"type": "ws", "path": "/"},
		},
		{
			name:      "httpupgrade needs nothing",
			transport: map[string]interface{}{"type": "httpupgrade", "path": "/"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateOutbound(xhttpOutbound(tc.transport), 1)
			joined := strings.Join(errs, "; ")
			has := strings.Contains(joined, "x_padding_bytes")
			if tc.wantErr && !has {
				t.Fatalf("expected an x_padding_bytes error, got %q", joined)
			}
			if !tc.wantErr && has {
				t.Fatalf("expected no x_padding_bytes error, got %q", joined)
			}
		})
	}
}

// xhttp cannot SERVE. transport/v2rayxhttp holds a client and nothing else, and
// NewServerTransport has no xhttp case at all — an inbound configured with it
// dies at startup with "create server transport: xhttp: unknown transport type".
// The panel used to offer it anyway, so the choice could only ever fail.
func TestValidateXHTTPInboundIsRejected(t *testing.T) {
	ib := func(transport map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{
			"type": "vless", "tag": "vless-in", "listen": "::", "listen_port": float64(8443),
			"users":     []interface{}{map[string]interface{}{"name": "a", "uuid": "11111111-1111-1111-1111-111111111111"}},
			"transport": transport,
		}
	}
	for _, tr := range []map[string]interface{}{
		{"type": "xhttp", "path": "/"},
		{"type": "xhttp", "path": "/", "x_padding_bytes": "100-1000"}, // padding does not save it
	} {
		errs := validateInbound(ib(tr), 0)
		if !strings.Contains(strings.Join(errs, "; "), "xhttp") {
			t.Fatalf("an xhttp inbound must be rejected, got %v", errs)
		}
	}
	// The transports that DO serve keep working.
	for _, tr := range []map[string]interface{}{
		{"type": "ws", "path": "/"},
		{"type": "httpupgrade", "path": "/"},
		{"type": "grpc", "service_name": "gun"},
	} {
		if errs := validateInbound(ib(tr), 0); len(errs) != 0 {
			t.Fatalf("transport %v must validate, got %v", tr["type"], errs)
		}
	}
}
