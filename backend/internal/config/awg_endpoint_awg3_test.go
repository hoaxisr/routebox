package config

import "testing"

func TestBuildAwgServerEndpoint_AWG3(t *testing.T) {
	got := BuildAwgServerEndpoint("awg-server", AwgServerSpec{
		PrivateKey: "K", Address: "10.10.0.1/24", ListenPort: 51820, MTU: 1408,
		HeaderProtectionKey: "SFBLQkFTRTY0",
		Obf: map[string]interface{}{"s1": 8, "content_padding_addition": "64", "rekey_after_time": "120-150"},
	})
	if got["header_protection_key"] != "SFBLQkFTRTY0" ||
		got["content_padding_addition"] != "64" || got["rekey_after_time"] != "120-150" {
		t.Fatalf("awg3 fields missing: %#v", got)
	}
	bare := BuildAwgServerEndpoint("awg-server", AwgServerSpec{PrivateKey: "K", Address: "10.10.0.1/24", ListenPort: 51820, MTU: 1408})
	for _, k := range []string{"header_protection_key", "content_padding_addition", "rekey_after_time"} {
		if _, ok := bare[k]; ok {
			t.Fatalf("%s must be omitted when empty", k)
		}
	}
}
