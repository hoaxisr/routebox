//go:build qtoracle

// Verifies a generated link against the Amnezia client's OWN parser rather than a
// hand transcription of its schema. Run with:
//
//	AMNEZIA_CLIENT=... ./scripts/qtoracle/build.sh
//	go test -tags qtoracle ./backend/internal/awg/ -run TestOracle -v
//
// Kept behind a build tag so the default suite has no Qt dependency. The oracle
// consumes the whole vpn:// string, so the envelope is verified by Qt too.
package awg

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"routebox/backend/internal/awg/cps"
)

const oraclePath = "../../../scripts/qtoracle/oracle"

// runOracle returns the outer object the client's parser produces, plus the parsed
// last_config it carries.
func runOracle(t *testing.T, link string) (outer, last map[string]any) {
	t.Helper()
	if _, err := os.Stat(oraclePath); err != nil {
		t.Fatalf("oracle not built — run scripts/qtoracle/build.sh: %v", err)
	}
	cmd := exec.Command(oraclePath)
	cmd.Stdin = bytes.NewReader([]byte(link))
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("oracle failed: %v\nstderr: %s", err, errb.String())
	}
	if err := json.Unmarshal(out.Bytes(), &outer); err != nil {
		t.Fatalf("oracle output is not JSON: %v\n%s", err, out.String())
	}
	s, ok := outer["last_config"].(string)
	if !ok {
		t.Fatalf("client produced no last_config string: %s", out.String())
	}
	if err := json.Unmarshal([]byte(s), &last); err != nil {
		t.Fatalf("client's last_config is not JSON: %v", err)
	}
	return outer, last
}

// Every field we care about must survive the client's real fromJson -> toJson. A key
// the client does not know is dropped here, which is exactly the failure a
// hand-written schema table cannot catch.
func TestOracleRoundTripsAwg3Fields(t *testing.T) {
	outer, last := runOracle(t, mustLink(t, awg3Conf()))

	for k, want := range map[string]any{
		"port":               "51820", // a QString on the server side
		"transport_proto":    "udp",
		"protocol_version":   "2",
		"isThirdPartyConfig": true,
	} {
		if outer[k] != want {
			t.Errorf("client sees outer %s = %#v, want %#v", k, outer[k], want)
		}
	}

	for k, want := range map[string]any{
		"client_ip":              "10.10.0.2/32",
		"client_priv_key":        "cpriv",
		"server_pub_key":         validPub,
		"psk_key":                "psk",
		"Jc":                     "3",
		"Jmin":                   "10",
		"Jmax":                   "30",
		"S1":                     "15",
		"S2":                     "18",
		"S3":                     "20",
		"S4":                     "23",
		"H1":                     "1020325451-1020326451",
		"H2":                     "3288052141-3288053141",
		"H3":                     "1766607858-1766608858",
		"H4":                     "2528465083-2528466083",
		"HeaderProtectionKey":    "hpk==",
		"ContentPaddingAddition": "0-64",
		"RekeyAfterTime":         "120-150",
		"RekeyTimeout":           "5",
		"RejectAfterTime":        "180-210",
		"KeepaliveTimeout":       "10-25",
		"MaxHandshakeAttempts":   "18",
	} {
		if last[k] != want {
			t.Errorf("client sees %s = %#v, want %#v", k, last[k], want)
		}
	}
	if p, ok := last["port"].(float64); !ok || p != 51820 {
		t.Errorf("client sees last_config.port = %#v, want the number 51820", last["port"])
	}

	// These round-trip through the same AwgClientConfig::fromJson/toJson pair the
	// oracle exists to exercise, but nobody listed them above — a key-name slip in
	// any of them (mtu vs MTU, etc.) would otherwise pass both the untagged suite
	// and this test.
	if last["hostName"] != "vpn.example.com" {
		t.Errorf("client sees hostName = %#v, want vpn.example.com", last["hostName"])
	}
	if last["mtu"] != "1420" {
		t.Errorf("client sees mtu = %#v, want the string \"1420\"", last["mtu"])
	}
	if last["persistent_keep_alive"] != "25" {
		t.Errorf("client sees persistent_keep_alive = %#v, want \"25\"", last["persistent_keep_alive"])
	}
	if got, _ := json.Marshal(last["allowed_ips"]); string(got) != `["0.0.0.0/0"]` {
		t.Errorf("client sees allowed_ips = %s, want [\"0.0.0.0/0\"]", got)
	}
	conf, ok := last["config"].(string)
	if !ok || conf == "" || !strings.Contains(conf, "[Interface]") {
		t.Errorf("client sees config = %#v, want a non-empty .conf containing [Interface]", last["config"])
	}
}

// awg3Conf() does not set Mimic, so I1-I5 round-tripping needs its own peer. Every
// shipped CPS profile populates all five fields, so all five must be checked with
// distinct values — I1/I3 alone cannot catch the client silently dropping I2/I4/I5.
func TestOracleRoundTripsMimicry(t *testing.T) {
	c := awg3Conf()
	c.Mimic = cps.Set{I1: "<b 0xaa>", I2: "<r 32>", I3: "<t>", I4: "<r 40>", I5: "<t><r 12>"}
	_, last := runOracle(t, mustLink(t, c))
	for k, want := range map[string]string{
		"I1": "<b 0xaa>", "I2": "<r 32>", "I3": "<t>", "I4": "<r 40>", "I5": "<t><r 12>",
	} {
		if last[k] != want {
			t.Errorf("client sees %s = %#v, want %#v", k, last[k], want)
		}
	}
}

// A field left unset on the peer must reach the client as a genuinely absent key.
func TestOracleOmitsUnsetMimicryFields(t *testing.T) {
	c := awg3Conf()
	c.Mimic = cps.Set{I1: "<b 0xaa>", I3: "<t>"}
	_, last := runOracle(t, mustLink(t, c))
	if last["I1"] != "<b 0xaa>" || last["I3"] != "<t>" {
		t.Fatalf("client sees I1 = %#v, I3 = %#v", last["I1"], last["I3"])
	}
	for _, k := range []string{"I2", "I4", "I5"} {
		if _, ok := last[k]; ok {
			t.Errorf("client sees %s = %#v, want absent", k, last[k])
		}
	}
}

// A zeroed junk count must reach the client as "0", not as a missing key.
func TestOracleKeepsZeroJunkCount(t *testing.T) {
	c := awg3Conf()
	c.Obf.Jc = 0
	_, last := runOracle(t, mustLink(t, c))
	if last["Jc"] != "0" {
		t.Fatalf("client sees Jc = %#v, want \"0\"", last["Jc"])
	}
}
