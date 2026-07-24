package config

import (
	"reflect"
	"testing"
)

func TestBuildAwgServerEndpoint(t *testing.T) {
	spec := AwgServerSpec{
		PrivateKey: "SRVPRIVKEY",
		Address:    "10.10.0.1/24",
		ListenPort: 51820,
		MTU:        1408,
		Obf: map[string]interface{}{
			"jc": 4, "jmin": 40, "jmax": 70, "s1": 30, "s2": 0,
			"h1": "5", "h2": "", // empty h2 omitted
		},
		Peers: []AwgServerPeer{
			{PublicKey: "PUBA", PresharedKey: "PSKA", AllowedIP: "10.10.0.2/32"},
		},
	}
	got := BuildAwgServerEndpoint("awg-server", spec)
	want := map[string]interface{}{
		"type":             "awg",
		"tag":              "awg-server",
		"useIntegratedTun": false,
		"private_key":      "SRVPRIVKEY",
		"address":          []interface{}{"10.10.0.1/24"},
		"listen_port":      51820,
		"mtu":              1408,
		"jc":               4, "jmin": 40, "jmax": 70, "s1": 30, "h1": "5",
		"peers": []interface{}{
			map[string]interface{}{
				"public_key":    "PUBA",
				"preshared_key": "PSKA",
				"allowed_ips":   []interface{}{"10.10.0.2/32"},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mismatch\n got=%#v\nwant=%#v", got, want)
	}
}

func TestBuildAwgServerEndpoint_NoPeers(t *testing.T) {
	got := BuildAwgServerEndpoint("awg-server", AwgServerSpec{
		PrivateKey: "K", Address: "10.10.0.1/24", ListenPort: 51820, MTU: 1408,
	})
	if _, ok := got["peers"]; ok {
		t.Fatalf("no peers => 'peers' key must be absent, got %#v", got["peers"])
	}
}

func TestBuildAwgServerEndpointDualStack(t *testing.T) {
	ep := BuildAwgServerEndpoint("awg-x", AwgServerSpec{
		PrivateKey: "k", Address: "10.10.0.1/24", Address6: "fd00:abcd::a0a:1/64", ListenPort: 51820, MTU: 1420,
		Peers: []AwgServerPeer{{PublicKey: "p", AllowedIP: "10.10.0.5/32", AllowedIP6: "fd00:abcd::a0a:5/128"}},
	})
	addr := ep["address"].([]interface{})
	if len(addr) != 2 || addr[1] != "fd00:abcd::a0a:1/64" {
		t.Fatalf("address = %v", addr)
	}
	peers := ep["peers"].([]interface{})
	aip := peers[0].(map[string]interface{})["allowed_ips"].([]interface{})
	if len(aip) != 2 || aip[1] != "fd00:abcd::a0a:5/128" {
		t.Fatalf("allowed_ips = %v", aip)
	}
}

func TestBuildAwgServerEndpointV4OnlyUnchanged(t *testing.T) {
	ep := BuildAwgServerEndpoint("awg-x", AwgServerSpec{
		PrivateKey: "k", Address: "10.10.0.1/24", ListenPort: 51820, MTU: 1420,
		Peers: []AwgServerPeer{{PublicKey: "p", AllowedIP: "10.10.0.5/32"}},
	})
	if addr := ep["address"].([]interface{}); len(addr) != 1 {
		t.Fatalf("v4-only address must stay single: %v", addr)
	}
	aip := ep["peers"].([]interface{})[0].(map[string]interface{})["allowed_ips"].([]interface{})
	if len(aip) != 1 {
		t.Fatalf("v4-only allowed_ips must stay single: %v", aip)
	}
}
