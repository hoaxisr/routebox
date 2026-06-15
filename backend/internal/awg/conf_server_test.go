package awg

import (
	"strings"
	"testing"
)

func TestRenderServerNATIdempotentChains(t *testing.T) {
	s := ServerConf{
		PrivateKey: "SRVPRIV==", Address: "10.10.0.1/24", ListenPort: 51820, MTU: 1420,
		Obf: Obfuscation{Jc: 4, H1: "1"},
		WAN: "ens3", Subnet: "10.10.0.0/24", Iface: "awg-rb0",
	}
	peers := []PeerLine{{Name: "phone", PublicKey: "PUB==", PSK: "PSK==", AllowedIP: "10.10.0.2/32"}}
	out := RenderServer(s, peers)

	for _, want := range []string{
		"ListenPort = 51820",
		"# phone",
		"PublicKey = PUB==",
		"AllowedIPs = 10.10.0.2/32",
		"-N RBOX-AWG-NAT", // create dedicated chain
		"-F RBOX-AWG-NAT", // flush-stale-first (idempotent)
		"-s 10.10.0.0/24 -o ens3 -j MASQUERADE",
		"-p udp --dport 51820 -j ACCEPT", // INPUT rule
		"net.ipv4.ip_forward=1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered .conf missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "PostDown") {
		t.Fatalf("must render PostDown teardown")
	}
}

func TestParseDefaultRoute(t *testing.T) {
	sample := "default via 10.0.0.1 dev ens3 proto static metric 100"
	iface, err := parseDefaultRoute(sample)
	if err != nil || iface != "ens3" {
		t.Fatalf("got %q,%v; want ens3,nil", iface, err)
	}
	if _, err := parseDefaultRoute("blackhole default"); err == nil {
		t.Fatal("want error when no dev")
	}
}

func TestParseShowPeers(t *testing.T) {
	sample := `interface: awg-rb0
  public key: SRVPUB==
  listening port: 51820

peer: PUBONE==
  allowed ips: 10.10.0.2/32

peer: PUBTWO==
  allowed ips: 10.10.0.3/32
`
	got := parseShowPeers(sample)
	if len(got) != 2 || got[0] != "PUBONE==" || got[1] != "PUBTWO==" {
		t.Fatalf("parseShowPeers = %#v; want [PUBONE== PUBTWO==]", got)
	}
}
