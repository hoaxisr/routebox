package config

import "testing"

func TestBuildAwgServerEndpoint_AWG3(t *testing.T) {
	got := BuildAwgServerEndpoint("awg-server", AwgServerSpec{
		PrivateKey: "K", Address: "10.10.0.1/24", ListenPort: 51820, MTU: 1408,
		HeaderProtectionKey: "SFBLQkFTRTY0",
		Obf: map[string]interface{}{"s1": 8, "content_padding_addition": "64", "rekey_after_time": "120-150",
			"rekey_timeout": "5", "reject_after_time": "180", "keepalive_timeout": "25", "max_handshake_attempts": "18"},
	})
	if got["header_protection_key"] != "SFBLQkFTRTY0" ||
		got["content_padding_addition"] != "64" || got["rekey_after_time"] != "120-150" {
		t.Fatalf("awg3 fields missing: %#v", got)
	}
	for k, want := range map[string]string{
		"rekey_timeout": "5", "reject_after_time": "180", "keepalive_timeout": "25", "max_handshake_attempts": "18",
	} {
		if got[k] != want {
			t.Fatalf("device timer %s = %#v, want %s", k, got[k], want)
		}
	}
	bare := BuildAwgServerEndpoint("awg-server", AwgServerSpec{PrivateKey: "K", Address: "10.10.0.1/24", ListenPort: 51820, MTU: 1408})
	for _, k := range []string{"header_protection_key", "content_padding_addition", "rekey_after_time",
		"rekey_timeout", "reject_after_time", "keepalive_timeout", "max_handshake_attempts"} {
		if _, ok := bare[k]; ok {
			t.Fatalf("%s must be omitted when empty", k)
		}
	}
}

func TestBuildAwgServerEndpointAwg31Flags(t *testing.T) {
	base := AwgServerSpec{
		PrivateKey: "SRVPRIVKEY",
		Address:    "10.10.0.1/24",
		ListenPort: 51820,
		MTU:        1408,
	}

	t.Run("off omits both", func(t *testing.T) {
		spec := base
		spec.Obf = map[string]interface{}{"random_trailers": false, "disable_cookies": false}
		ep := BuildAwgServerEndpoint("awg-server", spec)
		for _, k := range []string{"random_trailers", "disable_cookies"} {
			if _, ok := ep[k]; ok {
				t.Fatalf("expected %q to be absent when off, got %#v", k, ep[k])
			}
		}
	})

	t.Run("absent omits both", func(t *testing.T) {
		ep := BuildAwgServerEndpoint("awg-server", base)
		for _, k := range []string{"random_trailers", "disable_cookies"} {
			if _, ok := ep[k]; ok {
				t.Fatalf("expected %q to be absent when unset", k)
			}
		}
	})

	// The literal matters: the endpoint hands the value to the engine over UAPI,
	// where amneziawg-go parses it with strconv.ParseBool — which takes
	// true/false/1/0 and does NOT take "on". The .conf side is the other way
	// round (awg's parse_bool takes on/off and rejects "true"), so the two paths
	// must not be made to share a literal.
	t.Run("on emits boolean true", func(t *testing.T) {
		spec := base
		spec.Obf = map[string]interface{}{"random_trailers": true, "disable_cookies": true}
		ep := BuildAwgServerEndpoint("awg-server", spec)
		for _, k := range []string{"random_trailers", "disable_cookies"} {
			v, ok := ep[k]
			if !ok {
				t.Fatalf("expected %q in the endpoint", k)
			}
			b, isBool := v.(bool)
			if !isBool || !b {
				t.Fatalf("%q = %#v (%T), want boolean true", k, v, v)
			}
		}
	})
}
