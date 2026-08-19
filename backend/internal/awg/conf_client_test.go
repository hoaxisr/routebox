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
		Mimic: cps.Set{I1: "<b 0x1603>", I2: "<r 16>"},
	}
	out, err := BuildClient(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"I1 = <b 0x1603>", "I2 = <r 16>", "Jc = 5"} {
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
		Keepalive:  "25",
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

func TestBuildClientEmitsAwg3Fields(t *testing.T) {
	c := ClientConf{
		PrivateKey: "AAAA", Address: "10.10.0.2/32", ServerPub: "BBBB",
		Endpoint: "1.2.3.4:51820", AllowedIPs: []string{"0.0.0.0/0"},
		Obf: Obfuscation{Jc: 5, CPA: "200-400", RAT: "120",
			RekeyTimeout: "5", RejectAfterTime: "180", KeepaliveTimeout: "25", MaxHandshakeAttempts: "18"},
		HeaderProtectionKey: "AAAAbbbbCCCCddddEEEEffffGGGGhhhhIIIIjjjjKK==",
	}
	out, err := BuildClient(c)
	if err != nil {
		t.Fatal(err)
	}
	peerIdx := strings.Index(out, "[Peer]")
	if peerIdx < 0 {
		t.Fatalf("no [Peer] block:\n%s", out)
	}
	iface := out[:peerIdx]
	for _, want := range []string{
		"ContentPaddingAddition = 200-400",
		"RekeyAfterTime = 120",
		"RekeyTimeout = 5",
		"RejectAfterTime = 180",
		"KeepaliveTimeout = 25",
		"MaxHandshakeAttempts = 18",
		"HeaderProtectionKey = AAAAbbbbCCCCddddEEEEffffGGGGhhhhIIIIjjjjKK==",
	} {
		if !strings.Contains(iface, want+"\n") {
			t.Fatalf("[Interface] block missing %q:\n%s", want, out)
		}
	}
}

func TestBuildClientEmptyAwg3FieldsOmitted(t *testing.T) {
	c := ClientConf{
		PrivateKey: "A", Address: "10.0.0.2/32", ServerPub: "B",
		Endpoint: "h:1", AllowedIPs: []string{"0.0.0.0/0"},
	}
	out, err := BuildClient(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"ContentPaddingAddition", "RekeyAfterTime", "HeaderProtectionKey",
		"RekeyTimeout", "RejectAfterTime", "KeepaliveTimeout", "MaxHandshakeAttempts"} {
		if strings.Contains(out, absent) {
			t.Fatalf("empty awg3 field %q must be omitted:\n%s", absent, out)
		}
	}
}

func TestBuildClientDualStack(t *testing.T) {
	out, err := BuildClient(ClientConf{
		PrivateKey: "k", Address: "10.10.0.5/32", Address6: "fd00:abcd::a0a:5/128",
		ServerPub: "s", Endpoint: "vps:51820", AllowedIPs: []string{"0.0.0.0/0", "::/0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Address = 10.10.0.5/32, fd00:abcd::a0a:5/128") {
		t.Fatalf("missing dual-stack Address:\n%s", out)
	}
	if !strings.Contains(out, "AllowedIPs = 0.0.0.0/0, ::/0") {
		t.Fatalf("missing ::/0:\n%s", out)
	}
}

func TestBuildClientV4OnlyUnchanged(t *testing.T) {
	out, _ := BuildClient(ClientConf{
		PrivateKey: "k", Address: "10.10.0.5/32", ServerPub: "s", Endpoint: "vps:51820",
		AllowedIPs: []string{"0.0.0.0/0"},
	})
	if !strings.Contains(out, "Address = 10.10.0.5/32\n") {
		t.Fatalf("v4-only Address changed:\n%s", out)
	}
}

func TestBuildClientEmptyOmitAndNoPSKAndV6(t *testing.T) {
	in := ClientConf{
		PrivateKey: "P==", Address: "10.10.0.2/32", MTU: 1420,
		Obf:       Obfuscation{Jc: 4}, // S/H/others zero -> omitted
		ServerPub: "S==", Endpoint: "[2001:db8::1]:51820",
		AllowedIPs: []string{"0.0.0.0/0"}, Keepalive: "25",
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

// TestStripAwg3ClearsAwg31Flags pins that a host without AWG3 at all also loses
// the 3.1 flags: awg-quick below 3.0 hard-fails on an unknown [Interface] key,
// so leaving them in would break the whole conf rather than degrade it.
func TestStripAwg3ClearsAwg31Flags(t *testing.T) {
	o := Obfuscation{RandomTrailers: true, DisableCookies: true, CPA: "16"}
	o.stripAwg3()
	if o.RandomTrailers || o.DisableCookies {
		t.Fatalf("stripAwg3 left the 3.1 flags: %+v", o)
	}
}

// TestStripAwg31KeepsAwg3Fields is the 3.0-capable-but-not-3.1 host: the AWG 3.0
// params stay, only the two new flags go.
func TestStripAwg31KeepsAwg3Fields(t *testing.T) {
	o := Obfuscation{RandomTrailers: true, DisableCookies: true, CPA: "16", RAT: "120-150"}
	o.stripAwg31()
	if o.RandomTrailers || o.DisableCookies {
		t.Fatalf("stripAwg31 left the 3.1 flags: %+v", o)
	}
	if o.CPA != "16" || o.RAT != "120-150" {
		t.Fatalf("stripAwg31 must not touch the 3.0 params: %+v", o)
	}
}

// TestClientConfCarriesRandomTrailers pins the symmetric half of the flag. The
// server draws a random tail on handshakes and there is no negotiation on the
// wire, so a client conf without the key means the client rejects those
// handshakes on its length check and the tunnel never comes up — silently.
//
// The literal is `on`: a client .conf is read by awg's parse_bool, which takes
// on/off or a number and rejects "true". The endpoint side is the opposite.
func TestClientConfCarriesRandomTrailers(t *testing.T) {
	var b strings.Builder
	writeObf(&b, Obfuscation{RandomTrailers: true})
	if !strings.Contains(b.String(), "RandomTrailers = on") {
		t.Fatalf("expected RandomTrailers = on in:\n%s", b.String())
	}
}

func TestClientConfOmitsRandomTrailersWhenOff(t *testing.T) {
	var b strings.Builder
	writeObf(&b, Obfuscation{})
	if strings.Contains(b.String(), "RandomTrailers") {
		t.Fatalf("expected no RandomTrailers when off:\n%s", b.String())
	}
}

// TestClientConfCarriesDisableCookies pins the second AWG 3.1 flag into the
// client artifact. awg-tools 3.1 parses DisableCookies as a plain [Interface]
// device key (config.c: key_match("DisableCookies")), same as RandomTrailers —
// it is not server-only, and leaving it out gave a "3.1" .conf carrying one of
// the version's two flags.
func TestClientConfCarriesDisableCookies(t *testing.T) {
	var b strings.Builder
	writeObf(&b, Obfuscation{DisableCookies: true})
	if !strings.Contains(b.String(), "DisableCookies = on") {
		t.Fatalf("expected DisableCookies = on in:\n%s", b.String())
	}
}

func TestClientConfOmitsDisableCookiesWhenOff(t *testing.T) {
	var b strings.Builder
	writeObf(&b, Obfuscation{})
	if strings.Contains(b.String(), "DisableCookies") {
		t.Fatalf("expected no DisableCookies when off:\n%s", b.String())
	}
}
