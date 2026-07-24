package awg

import (
	"testing"

	"routebox/backend/internal/config"
)

func TestBuildClientEndpoint_AWG3Fields(t *testing.T) {
	ep := BuildClientEndpoint(ClientEndpointSpec{
		Tag: "awg-alice", PrivateKey: "PRIV", Address: "10.10.0.2/32",
		Obf: Obfuscation{Jc: 4, S1: 30, H1: "5", CPA: "64", RAT: "120-150",
			RekeyTimeout: "5", RejectAfterTime: "180", KeepaliveTimeout: "25", MaxHandshakeAttempts: "18"},
		HeaderProtectionKey: "SFBLQkFTRTY0",
		ServerPub:           "SRVPUB", Host: "vpn.example.com", Port: 51820,
	})
	if ep["header_protection_key"] != "SFBLQkFTRTY0" {
		t.Fatalf("header_protection_key = %#v, want SFBLQkFTRTY0", ep["header_protection_key"])
	}
	if ep["content_padding_addition"] != "64" {
		t.Fatalf("content_padding_addition = %#v, want 64", ep["content_padding_addition"])
	}
	if ep["rekey_after_time"] != "120-150" {
		t.Fatalf("rekey_after_time = %#v, want 120-150", ep["rekey_after_time"])
	}
	for k, want := range map[string]string{
		"rekey_timeout": "5", "reject_after_time": "180", "keepalive_timeout": "25", "max_handshake_attempts": "18",
	} {
		if ep[k] != want {
			t.Fatalf("%s = %#v, want %s", k, ep[k], want)
		}
	}
	// The exported client endpoint must still pass the config validator.
	if errs := config.ValidateEndpointExported(ep); len(errs) != 0 {
		t.Fatalf("exported client endpoint invalid: %v", errs)
	}
}

func TestBuildClientEndpoint_AWG3FieldsOmittedWhenEmpty(t *testing.T) {
	ep := BuildClientEndpoint(ClientEndpointSpec{
		Tag: "awg-alice", PrivateKey: "PRIV", Address: "10.10.0.2/32",
		ServerPub: "SRVPUB", Host: "vpn.example.com", Port: 51820,
	})
	for _, k := range []string{"header_protection_key", "content_padding_addition", "rekey_after_time",
		"rekey_timeout", "reject_after_time", "keepalive_timeout", "max_handshake_attempts"} {
		if _, ok := ep[k]; ok {
			t.Fatalf("%s must be omitted when empty, got %#v", k, ep[k])
		}
	}
}
