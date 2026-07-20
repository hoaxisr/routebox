package awg

import (
	"reflect"
	"testing"

	"routebox/backend/internal/awg/cps"
	"routebox/backend/internal/config"
)

func TestBuildClientEndpoint_RoundTripsValidator(t *testing.T) {
	ep := BuildClientEndpoint(ClientEndpointSpec{
		Tag: "awg-alice", PrivateKey: "PRIV", Address: "10.10.0.2/32", MTU: 1408,
		Obf:       Obfuscation{Jc: 4, S1: 30, H1: "5"},
		Mimic:     cps.Set{I1: "<b 0xde>", I2: "<r 20>"},
		ServerPub: "SRVPUB", PSK: "PSK", Host: "vpn.example.com", Port: 51820,
	})
	// Client-side markers.
	if ep["listen_port"] != nil {
		t.Fatal("client endpoint must NOT have listen_port")
	}
	peers := ep["peers"].([]interface{})
	p := peers[0].(map[string]interface{})
	if p["address"] != "vpn.example.com" || p["port"] != 51820 {
		t.Fatalf("peer endpoint wrong: %#v", p)
	}
	if p["preshared_key"] != "PSK" {
		t.Fatalf("preshared_key missing/renamed: %#v", p)
	}
	if !reflect.DeepEqual(p["allowed_ips"], []interface{}{"0.0.0.0/0"}) {
		t.Fatalf("allowed_ips = %#v", p["allowed_ips"])
	}
	if ep["i1"] != "<b 0xde>" || ep["i2"] != "<r 20>" {
		t.Fatalf("mimic i-fields missing: %#v", ep)
	}
	// The exported client endpoint must pass the config validator.
	if errs := config.ValidateEndpointExported(ep); len(errs) != 0 {
		t.Fatalf("exported client endpoint invalid: %v", errs)
	}
}
