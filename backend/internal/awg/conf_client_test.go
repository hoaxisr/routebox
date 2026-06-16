package awg

import (
	"strings"
	"testing"

	"routebox/backend/internal/awg/cps"
)

func TestBuildClientEmitsMimic(t *testing.T) {
	c := ClientConf{
		PrivateKey: "AAAA", Address: "10.10.0.2/32", ServerPub: "BBBB",
		Endpoint: "1.2.3.4:51820", AllowedIPs: []string{"0.0.0.0/0"},
		Obf:   Obfuscation{Jc: 5, H1: "100"},
		Mimic: cps.Set{Itime: 7, I1: "<b 0x1603>", I2: "<r 16>"},
	}
	out, err := BuildClient(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Itime = 7", "I1 = <b 0x1603>", "I2 = <r 16>", "Jc = 5"} {
		if !strings.Contains(out, want) {
			t.Fatalf("client conf missing %q:\n%s", want, out)
		}
	}
}

func TestBuildClientNoMimicNoIFields(t *testing.T) {
	c := ClientConf{PrivateKey: "A", Address: "10.0.0.2/32", ServerPub: "B", Endpoint: "h:1", AllowedIPs: []string{"0.0.0.0/0"}}
	out, _ := BuildClient(c)
	if strings.Contains(out, "I1 =") || strings.Contains(out, "Itime") {
		t.Fatalf("empty Mimic must emit no I fields:\n%s", out)
	}
}

func TestRenderServerNoIFields(t *testing.T) {
	sc := ServerConf{PrivateKey: "k", Address: "10.10.0.1/24", ListenPort: 51820, MTU: 1420, Subnet: "10.10.0.0/24", WAN: "ens3", Iface: "awg-rb0", Obf: Obfuscation{Jc: 5, H1: "100"}}
	out := RenderServer(sc, nil)
	if strings.Contains(out, "I1 =") || strings.Contains(out, "Itime") {
		t.Fatalf("server.conf must never contain I/Itime:\n%s", out)
	}
}

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
