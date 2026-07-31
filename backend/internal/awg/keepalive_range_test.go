package awg

import (
	"strings"
	"testing"
)

// AWG 3.0 lets PersistentKeepalive be a "lo-hi" range the device redraws on every
// timer arm (fork commit "persistent_keepalive_interval принимает диапазон").
// Both client exports have to carry it verbatim instead of an integer.
func TestClientExportsCarryKeepaliveRange(t *testing.T) {
	conf, err := BuildClient(ClientConf{
		PrivateKey: "priv==", Address: "10.0.0.2/32", ServerPub: "spub==",
		Endpoint: "vpn.example.com:51820", AllowedIPs: []string{"0.0.0.0/0"},
		Keepalive: "22-30",
	})
	if err != nil {
		t.Fatalf("BuildClient: %v", err)
	}
	if !strings.Contains(conf, "PersistentKeepalive = 22-30\n") {
		t.Fatalf("client .conf lost the keepalive range:\n%s", conf)
	}

	ep := BuildClientEndpoint(ClientEndpointSpec{
		Tag: "t", PrivateKey: "priv==", Address: "10.0.0.2/32",
		ServerPub: "spub==", Host: "vpn.example.com", Port: 51820,
		Keepalive: "22-30",
	})
	peer := ep["peers"].([]interface{})[0].(map[string]interface{})
	// A range can only be a string; the fork's option type reads both shapes.
	if got := peer["persistent_keepalive_interval"]; got != "22-30" {
		t.Fatalf("endpoint peer keepalive = %#v, want \"22-30\"", got)
	}

	// Plain seconds stay a NUMBER — this export gets pasted into another box that
	// may still run a pre-3.0 binary, where the field is a uint16.
	ep = BuildClientEndpoint(ClientEndpointSpec{
		Tag: "t", PrivateKey: "priv==", Address: "10.0.0.2/32",
		ServerPub: "spub==", Host: "vpn.example.com", Port: 51820,
		Keepalive: "15",
	})
	peer = ep["peers"].([]interface{})[0].(map[string]interface{})
	if got := peer["persistent_keepalive_interval"]; got != 15 {
		t.Fatalf("endpoint peer keepalive = %#v (%T), want 15 (int)", got, got)
	}

	// Unset stays the historical 25 rather than dropping the line.
	ep = BuildClientEndpoint(ClientEndpointSpec{
		Tag: "t", PrivateKey: "priv==", Address: "10.0.0.2/32",
		ServerPub: "spub==", Host: "vpn.example.com", Port: 51820,
	})
	peer = ep["peers"].([]interface{})[0].(map[string]interface{})
	if got := peer["persistent_keepalive_interval"]; got != 25 {
		t.Fatalf("endpoint peer keepalive = %#v, want the 25 default", got)
	}
}

// clientKeepalive reads the LIVE settings (no re-enable needed) and refuses to
// hand a client something the device cannot parse.
func TestManagerClientKeepalive(t *testing.T) {
	m := &Manager{}
	if got := m.clientKeepalive(); got != DefaultClientKeepalive {
		t.Fatalf("no desired getter: got %q, want %q", got, DefaultClientKeepalive)
	}
	for _, tc := range []struct{ set, want string }{
		{"", DefaultClientKeepalive},
		{"15", "15"},
		{"22-30", "22-30"},
		{"30-22", DefaultClientKeepalive}, // lo > hi is unusable, not fatal
		{"abc", DefaultClientKeepalive},
	} {
		set := tc.set
		m.SetDesired(func() EnableInput { return EnableInput{ClientKeepalive: set} })
		if got := m.clientKeepalive(); got != tc.want {
			t.Fatalf("ClientKeepalive %q: got %q, want %q", tc.set, got, tc.want)
		}
	}
}
