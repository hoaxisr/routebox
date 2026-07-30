package awg

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"routebox/backend/internal/awg/cps"
)

// qUncompress mirrors the Qt reader so the round trip is a real one and not a
// restatement of qCompress. Qt ignores the length prefix, so this skips it rather
// than verifying it.
func qUncompress(t *testing.T, b []byte) []byte {
	t.Helper()
	if len(b) < 4 {
		t.Fatalf("qCompress output too short: %d bytes", len(b))
	}
	zr, err := zlib.NewReader(bytes.NewReader(b[4:]))
	if err != nil {
		t.Fatalf("body is not a zlib stream: %v", err)
	}
	defer zr.Close()
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("zlib read: %v", err)
	}
	return plain
}

func TestQCompressBodyIsZlib(t *testing.T) {
	plain := []byte(`{"containers":[{"container":"amnezia-awg"}]}`)
	got := qUncompress(t, qCompress(plain))
	if !bytes.Equal(got, plain) {
		t.Fatalf("round trip mismatch:\n got %q\nwant %q", got, plain)
	}
}

// Negative control: the same assertion must REJECT raw deflate. Without this the
// test above would pass for an implementation that fails in the real client.
func TestZlibAssertionRejectsRawDeflate(t *testing.T) {
	var body bytes.Buffer
	body.Write([]byte{0, 0, 0, 5}) // a length prefix, as qCompress would write
	fw, err := flate.NewWriter(&body, 8)
	if err != nil {
		t.Fatalf("flate writer: %v", err)
	}
	fw.Write([]byte("hello"))
	fw.Close()

	if _, err := zlib.NewReader(bytes.NewReader(body.Bytes()[4:])); err == nil {
		t.Fatal("raw deflate must not be accepted as zlib — the assertion does not discriminate")
	}
}

// The prefix is not an interop gate (real Qt ignores it), but a wrong or truncated
// one is still a bug in our own output. Cheap to pin.
func TestQCompressPrefixCarriesPlaintextLength(t *testing.T) {
	got := qCompress(bytes.Repeat([]byte("x"), 300))
	if n := binary.BigEndian.Uint32(got[:4]); n != 300 {
		t.Fatalf("prefix = %d, want 300", n)
	}
}

func TestQCompressEmptyInput(t *testing.T) {
	got := qCompress(nil)
	if n := binary.BigEndian.Uint32(got[:4]); n != 0 {
		t.Fatalf("prefix = %d, want 0", n)
	}
	if len(qUncompress(t, got)) != 0 {
		t.Fatal("empty input must round trip to empty")
	}
}

// decodeLink unwraps vpn:// -> base64url -> qCompress -> outer JSON object.
func decodeLink(t *testing.T, link string) map[string]any {
	t.Helper()
	if !strings.HasPrefix(link, "vpn://") {
		t.Fatalf("link must start with vpn://, got %.20q", link)
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(link, "vpn://"))
	if err != nil {
		t.Fatalf("payload is not unpadded base64url: %v", err)
	}
	var outer map[string]any
	if err := json.Unmarshal(qUncompress(t, raw), &outer); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	return outer
}

// container returns the single container object and its protocol key.
func container(t *testing.T, outer map[string]any) (map[string]any, string) {
	t.Helper()
	arr, ok := outer["containers"].([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("containers must be a 1-element array, got %#v", outer["containers"])
	}
	c, ok := arr[0].(map[string]any)
	if !ok {
		t.Fatalf("container entry must be an object, got %#v", arr[0])
	}
	switch name, _ := c["container"].(string); name {
	case "amnezia-awg":
		return c, "awg"
	case "amnezia-wireguard":
		return c, "wireguard"
	default:
		t.Fatalf("unexpected container %q", name)
		return nil, ""
	}
}

// lastConfig parses the inner JSON *string*. Asserting on the outer JSON by
// substring would pass a double-encoding bug, so always go through this.
func lastConfig(t *testing.T, outer map[string]any) map[string]any {
	t.Helper()
	c, proto := container(t, outer)
	inner, ok := c[proto].(map[string]any)
	if !ok {
		t.Fatalf("container has no %q object", proto)
	}
	s, ok := inner["last_config"].(string)
	if !ok {
		t.Fatalf("last_config must be a JSON string, got %T", inner["last_config"])
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("last_config is not JSON: %v", err)
	}
	return m
}

// fullObf is a peer with every obfuscation and awg3 field set.
func fullObf() Obfuscation {
	return Obfuscation{
		Jc: 3, Jmin: 10, Jmax: 30, S1: 15, S2: 18, S3: 20, S4: 23,
		H1: "1020325451-1020326451", H2: "3288052141-3288053141",
		H3: "1766607858-1766608858", H4: "2528465083-2528466083",
		CPA: "0-64", RAT: "120-150", RekeyTimeout: "5",
		RejectAfterTime: "180-210", KeepaliveTimeout: "10-25", MaxHandshakeAttempts: "18",
	}
}

func awg3Conf() ClientConf {
	return ClientConf{
		PrivateKey: "cpriv", Address: "10.10.0.2/32",
		MTU: 1420, Obf: fullObf(), ServerPub: validPub,
		Endpoint: "vpn.example.com:51820", AllowedIPs: []string{"0.0.0.0/0"},
		Keepalive: 25, PSK: "psk", HeaderProtectionKey: "hpk==",
	}
}

func mustLink(t *testing.T, c ClientConf) string {
	t.Helper()
	link, err := AmneziaLink(c, "phone")
	if err != nil {
		t.Fatalf("AmneziaLink: %v", err)
	}
	return link
}

func TestAmneziaLinkAwgContainerAndTypes(t *testing.T) {
	outer := decodeLink(t, mustLink(t, awg3Conf()))

	c, proto := container(t, outer)
	if proto != "awg" {
		t.Fatalf("proto = %q, want awg", proto)
	}
	if got := outer["defaultContainer"]; got != "amnezia-awg" {
		t.Fatalf("defaultContainer = %v, want amnezia-awg", got)
	}
	if got := outer["description"]; got != "phone — vpn.example.com" {
		t.Fatalf("description = %v", got)
	}
	if got := outer["hostName"]; got != "vpn.example.com" {
		t.Fatalf("hostName = %v", got)
	}

	inner := c["awg"].(map[string]any)
	if got := inner["port"]; got != "51820" {
		t.Fatalf("outer port = %#v, want the string \"51820\"", got)
	}
	if got := inner["protocol_version"]; got != "2" {
		t.Fatalf("protocol_version = %v, want \"2\"", got)
	}
	if got := inner["isThirdPartyConfig"]; got != true {
		t.Fatalf("isThirdPartyConfig = %#v, want true", got)
	}
	if got := inner["transport_proto"]; got != "udp" {
		t.Fatalf("transport_proto = %v", got)
	}

	last := lastConfig(t, outer)
	if _, ok := last["port"].(float64); !ok {
		t.Fatalf("last_config.port must be a JSON number, got %T", last["port"])
	}
	if got := last["port"]; got != float64(51820) {
		t.Fatalf("last_config.port = %v", got)
	}
	for _, k := range []string{"Jc", "Jmin", "Jmax", "S1", "S2", "S3", "S4", "H1", "H2", "H3", "H4"} {
		if _, ok := last[k].(string); !ok {
			t.Fatalf("%s must be a JSON string, got %T", k, last[k])
		}
	}
	for k, want := range map[string]string{
		"Jc": "3", "Jmin": "10", "Jmax": "30", "S1": "15", "S2": "18", "S3": "20", "S4": "23",
		"HeaderProtectionKey": "hpk==", "ContentPaddingAddition": "0-64",
		"RekeyAfterTime": "120-150", "RekeyTimeout": "5",
		"RejectAfterTime": "180-210", "KeepaliveTimeout": "10-25",
		"MaxHandshakeAttempts": "18",
		"client_priv_key":      "cpriv", "server_pub_key": validPub,
		"psk_key": "psk", "mtu": "1420", "persistent_keep_alive": "25",
		"client_ip": "10.10.0.2/32",
	} {
		if last[k] != want {
			t.Errorf("%s = %v, want %q", k, last[k], want)
		}
	}
	// Not the ["0.0.0.0/0","::/0"] literal: matching it routes IPv6 into a tunnel
	// with no IPv6 address.
	if got, _ := json.Marshal(last["allowed_ips"]); string(got) != `["0.0.0.0/0"]` {
		t.Errorf("allowed_ips = %s", got)
	}
	if !strings.Contains(last["config"].(string), "[Interface]") {
		t.Error("last_config.config must embed the .conf text")
	}
}

// The off preset empties H1-H4 and zeroes the numerics; such a peer is plain
// WireGuard and must not be labelled amnezia-awg.
func TestAmneziaLinkUnobfuscatedIsWireguard(t *testing.T) {
	c := awg3Conf()
	c.Obf = Obfuscation{}
	c.HeaderProtectionKey = ""

	outer := decodeLink(t, mustLink(t, c))
	cont, proto := container(t, outer)
	if proto != "wireguard" {
		t.Fatalf("proto = %q, want wireguard", proto)
	}
	if got := outer["defaultContainer"]; got != "amnezia-wireguard" {
		t.Fatalf("defaultContainer = %v", got)
	}
	if _, ok := cont["wireguard"].(map[string]any)["protocol_version"]; ok {
		t.Error("protocol_version must be absent on the wireguard container")
	}
	last := lastConfig(t, outer)
	for _, k := range []string{"Jc", "Jmin", "Jmax", "S1", "S2", "S3", "S4",
		"H1", "H2", "H3", "H4", "HeaderProtectionKey", "ContentPaddingAddition"} {
		if _, ok := last[k]; ok {
			t.Errorf("%s must be absent for an unobfuscated peer", k)
		}
	}
}

// A partially obfuscated peer cannot be expressed either way: amnezia-wireguard
// silently connects as plain WireGuard to an AWG server (no handshake, no error),
// and amnezia-awg crashes the Android import.
func TestAmneziaLinkRejectsPartialObfuscation(t *testing.T) {
	cases := map[string]struct {
		mutate  func(*ClientConf)
		wantMsg string
	}{
		"junk set, no header magic": {mutate: func(c *ClientConf) {
			c.HeaderProtectionKey = ""
			c.Obf = Obfuscation{Jc: 5, Jmin: 10, Jmax: 50, S1: 30, S2: 40}
		}},
		"one header magic missing": {mutate: func(c *ClientConf) {
			c.HeaderProtectionKey = ""
			c.Obf = fullObf()
			c.Obf.CPA, c.Obf.RAT = "", ""
			c.Obf.RekeyTimeout, c.Obf.RejectAfterTime = "", ""
			c.Obf.KeepaliveTimeout, c.Obf.MaxHandshakeAttempts = "", ""
			c.Obf.H2 = ""
		}},
		"header key with no header magic": {mutate: func(c *ClientConf) {
			c.Obf = Obfuscation{S1: 15, S2: 18, S3: 20, S4: 23}
		}},
		// partial == false here (obfuscation is entirely off); only the awg3
		// disjunct (a lone HeaderProtectionKey) makes this unrepresentable. Without
		// that disjunct in the guard, this case would wrongly succeed.
		"header key with obfuscation entirely off": {
			mutate: func(c *ClientConf) {
				c.Obf = Obfuscation{}
				c.HeaderProtectionKey = "hpk=="
			},
			wantMsg: "a header protection key is set but obfuscation is off",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := awg3Conf()
			tc.mutate(&c)
			_, err := AmneziaLink(c, "phone")
			if !errors.Is(err, ErrLinkUnrepresentable) {
				t.Fatalf("err = %v, want ErrLinkUnrepresentable", err)
			}
			if tc.wantMsg != "" && !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("err = %v, want message containing %q", err, tc.wantMsg)
			}
		})
	}
}

// Each empty header magic must be named, so the operator knows what to set. H1/H4
// stay set here (and must NOT appear in the error) precisely because the static
// wording of the partial-obfuscation message must not itself contain any H-literal
// — otherwise this assertion would pass regardless of what missingHeaderFields does.
func TestAmneziaLinkUnrepresentableNamesEveryEmptyField(t *testing.T) {
	c := awg3Conf()
	c.Obf.H2, c.Obf.H3 = "", ""
	_, err := AmneziaLink(c, "phone")
	if !errors.Is(err, ErrLinkUnrepresentable) {
		t.Fatalf("err = %v", err)
	}
	for _, want := range []string{"H2", "H3"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must name %s, got %q", want, err)
		}
	}
	for _, unwanted := range []string{"H1", "H4"} {
		if strings.Contains(err.Error(), unwanted) {
			t.Errorf("error must not name the set field %s, got %q", unwanted, err)
		}
	}
}

// Every required numeric may legitimately be zero. Omitting one reaches the client
// as a missing key, which is not the same as zero.
func TestAmneziaLinkZeroNumericsAreSpelledOut(t *testing.T) {
	c := awg3Conf()
	c.Obf.Jc, c.Obf.Jmin, c.Obf.Jmax, c.Obf.S1, c.Obf.S2 = 0, 0, 0, 0, 0
	c.Obf.S3, c.Obf.S4 = 0, 0

	last := lastConfig(t, decodeLink(t, mustLink(t, c)))
	for _, k := range []string{"Jc", "Jmin", "Jmax", "S1", "S2"} {
		if last[k] != "0" {
			t.Errorf("%s = %#v, want the string \"0\"", k, last[k])
		}
	}
	// S3/S4 are optional on their side, so a zero means "not configured".
	for _, k := range []string{"S3", "S4"} {
		if _, ok := last[k]; ok {
			t.Errorf("%s must be omitted when zero, got %#v", k, last[k])
		}
	}
}

// With the broker on, client_ip stays IPv4-only and allowed_ips drops ::/0, while
// the embedded .conf keeps both for wg-quick users.
func TestAmneziaLinkDualStackKeepsClientIPv4Only(t *testing.T) {
	c := awg3Conf()
	c.Address6 = "fd00::2/128"
	c.AllowedIPs = []string{"0.0.0.0/0", "::/0"}

	last := lastConfig(t, decodeLink(t, mustLink(t, c)))
	if last["client_ip"] != "10.10.0.2/32" {
		t.Fatalf("client_ip = %v, want the IPv4 prefix alone", last["client_ip"])
	}
	got, _ := json.Marshal(last["allowed_ips"])
	if string(got) != `["0.0.0.0/0"]` {
		t.Fatalf("allowed_ips = %s, want IPv4 only — ::/0 would blackhole IPv6", got)
	}
	if !strings.Contains(last["config"].(string), "Address = 10.10.0.2/32, fd00::2/128") {
		t.Error("the embedded .conf must keep the full dual-stack Address line")
	}
}

// Every shipped CPS profile (dns/web/stealth) populates all five I1-I5 fields, so
// each must survive with its own distinct value — a test that only ever sets I1
// and I3 cannot tell "I2/I4/I5 emitted" from "I2/I4/I5 dropped".
func TestAmneziaLinkMimicry(t *testing.T) {
	c := awg3Conf()
	c.Mimic = cps.Set{I1: "<b 0xaa>", I2: "<r 32>", I3: "<t>", I4: "<r 40>", I5: "<t><r 12>"}

	last := lastConfig(t, decodeLink(t, mustLink(t, c)))
	for k, want := range map[string]string{
		"I1": "<b 0xaa>", "I2": "<r 32>", "I3": "<t>", "I4": "<r 40>", "I5": "<t><r 12>",
	} {
		if last[k] != want {
			t.Errorf("%s = %v, want %q", k, last[k], want)
		}
	}
}

// A field left unset on the peer must be a genuinely absent key, not an empty
// string — kept as a separate case from TestAmneziaLinkMimicry so that test can
// set all five fields without losing this coverage.
func TestAmneziaLinkMimicryOmitsUnsetFields(t *testing.T) {
	c := awg3Conf()
	c.Mimic = cps.Set{I1: "<b 0xaa>", I3: "<t>"}

	last := lastConfig(t, decodeLink(t, mustLink(t, c)))
	if last["I1"] != "<b 0xaa>" || last["I3"] != "<t>" {
		t.Fatalf("I1 = %v, I3 = %v", last["I1"], last["I3"])
	}
	for _, k := range []string{"I2", "I4", "I5"} {
		if _, ok := last[k]; ok {
			t.Errorf("%s must be omitted when unset", k)
		}
	}
}

func TestAmneziaLinkDNS(t *testing.T) {
	c := awg3Conf()

	c.DNS = []string{"10.10.0.1", "9.9.9.9", "8.8.8.8"}
	outer := decodeLink(t, mustLink(t, c))
	if outer["dns1"] != "10.10.0.1" || outer["dns2"] != "9.9.9.9" {
		t.Fatalf("dns pair = %v / %v", outer["dns1"], outer["dns2"])
	}

	c.DNS = []string{"10.10.0.1"}
	outer = decodeLink(t, mustLink(t, c))
	if outer["dns1"] != "10.10.0.1" {
		t.Fatalf("dns1 = %v", outer["dns1"])
	}
	if _, ok := outer["dns2"]; ok {
		t.Error("dns2 must be absent when only one resolver is configured")
	}

	c.DNS = nil
	outer = decodeLink(t, mustLink(t, c))
	for _, k := range []string{"dns1", "dns2"} {
		if _, ok := outer[k]; ok {
			t.Errorf("%s must be absent when no resolver is configured", k)
		}
	}
}

func TestAmneziaLinkEmptyPeerNameFallsBackToHost(t *testing.T) {
	link, err := AmneziaLink(awg3Conf(), "")
	if err != nil {
		t.Fatalf("AmneziaLink: %v", err)
	}
	if got := decodeLink(t, link)["description"]; got != "vpn.example.com" {
		t.Fatalf("description = %v, want the bare host", got)
	}
}

// The Amnezia client writes an unbracketed endpoint, so an IPv6 literal cannot work.
// Refusing beats emitting a link that fails silently on the user's device.
func TestAmneziaLinkRejectsIPv6LiteralHost(t *testing.T) {
	cases := map[string]string{
		"plain IPv6": "[fd00::1]:51820",
		// ::ffff:1.2.3.4 is Is6()==true AND Is4In6()==true: the guard must still
		// refuse it. netip.ParseAddr("1.2.3.4") — the dotted form actually stored
		// on a real deployment — has Is6()==false, so this is not the same case
		// as the dotted-IPv4 test below; it is the mapped form that slips through
		// an "!Is4In6()" carve-out.
		"IPv4-mapped IPv6": "[::ffff:1.2.3.4]:51820",
	}
	for name, endpoint := range cases {
		t.Run(name, func(t *testing.T) {
			c := awg3Conf()
			c.Endpoint = endpoint
			_, err := AmneziaLink(c, "phone")
			if !errors.Is(err, ErrLinkUnrepresentable) {
				t.Fatalf("err = %v, want ErrLinkUnrepresentable", err)
			}
		})
	}
}

// An IPv4-literal server host — the common bare-VPS deployment — must still work.
// Every other success-path test uses a hostname; without this, an implementation
// that rejected any IP literal (not just IPv6) would pass the whole file.
func TestAmneziaLinkAcceptsIPv4LiteralHost(t *testing.T) {
	c := awg3Conf()
	c.Endpoint = "203.0.113.5:51820"
	outer := decodeLink(t, mustLink(t, c))
	if outer["hostName"] != "203.0.113.5" {
		t.Fatalf("hostName = %v", outer["hostName"])
	}
}

// A malformed ClientConf must fail as an ordinary error, not as an unrepresentable
// peer — the handler maps the two to different status codes.
func TestAmneziaLinkIncompleteConfIsNotUnrepresentable(t *testing.T) {
	_, err := AmneziaLink(ClientConf{}, "phone")
	if err == nil {
		t.Fatal("an incomplete ClientConf must error")
	}
	if errors.Is(err, ErrLinkUnrepresentable) {
		t.Fatalf("err = %v, want a plain error", err)
	}
}
