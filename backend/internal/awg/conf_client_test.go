package awg

import (
	"strings"
	"testing"
)

func TestBuildClientGolden(t *testing.T) {
	in := ClientConf{
		PrivateKey: "CLIENTPRIV==",
		Address:    "10.10.0.2/32",
		DNS:        []string{"1.1.1.1"},
		MTU:        1420,
		Obf:        Obfuscation{Jc: 4, Jmin: 40, Jmax: 70, S1: 50, S2: 50, H1: "1", H2: "2", H3: "3", H4: "4"},
		ServerPub:  "SERVERPUB==",
		PSK:        "PSK==",
		Endpoint:   "vpn.example.com:51820",
		AllowedIPs: []string{"0.0.0.0/0"},
		Keepalive:  25,
	}
	want := `[Interface]
PrivateKey = CLIENTPRIV==
Address = 10.10.0.2/32
DNS = 1.1.1.1
MTU = 1420
Jc = 4
Jmin = 40
Jmax = 70
S1 = 50
S2 = 50
H1 = 1
H2 = 2
H3 = 3
H4 = 4

[Peer]
PublicKey = SERVERPUB==
PresharedKey = PSK==
Endpoint = vpn.example.com:51820
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25
`
	got, err := BuildClient(in)
	if err != nil {
		t.Fatalf("BuildClient: %v", err)
	}
	if got != want {
		t.Fatalf("golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestBuildClientEmptyOmitAndNoPSKAndV6(t *testing.T) {
	in := ClientConf{
		PrivateKey: "P==", Address: "10.10.0.2/32", MTU: 1420,
		Obf:       Obfuscation{Jc: 4}, // S/H/others zero -> omitted
		ServerPub: "S==", Endpoint: "[2001:db8::1]:51820",
		AllowedIPs: []string{"0.0.0.0/0"}, Keepalive: 25,
	}
	got, err := BuildClient(in)
	if err != nil {
		t.Fatalf("BuildClient: %v", err)
	}
	if want := "Jc = 4\n"; !strings.Contains(got, want) {
		t.Fatalf("Jc should render")
	}
	if strings.Contains(got, "Jmin") || strings.Contains(got, "S1") || strings.Contains(got, "H1") {
		t.Fatalf("zero/absent obfuscation fields must be omitted:\n%s", got)
	}
	if strings.Contains(got, "PresharedKey") {
		t.Fatalf("no PSK -> no PresharedKey line")
	}
	if !strings.Contains(got, "Endpoint = [2001:db8::1]:51820") {
		t.Fatalf("IPv6 endpoint must pass through bracketed")
	}
}
