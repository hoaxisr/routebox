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
