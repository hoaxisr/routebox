package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The naive INBOUND takes quic_congestion_control (#73) with a wider value set
// than the outbound: bbr_standard and bbr2_variant exist only on the listener
// side. Both the key and that asymmetry are fork territory, and an unknown key
// or value is a decode FATAL — the VPN would not come back up. Pin the contract
// against the real binary, the same way the hysteria2 keys are pinned. Skips
// without one; CI installs a published amnezia-box before the tests run.
func TestNaiveInboundCongestionKeys_AcceptedByBinary(t *testing.T) {
	binary := ""
	for _, b := range []string{"amnezia-box", "sing-box"} {
		if _, err := exec.LookPath(b); err == nil {
			binary = b
			break
		}
	}
	if binary == "" {
		t.Skip("no amnezia-box/sing-box binary on PATH")
	}

	dir := t.TempDir()
	// A naive inbound refuses to start without a usable certificate; the fixture
	// carries a throwaway one, this test is about the congestion key.
	certPath, keyPath := filepath.Join(dir, "c.pem"), filepath.Join(dir, "k.pem")
	gen := exec.Command("openssl", "req", "-x509", "-newkey", "rsa:2048",
		"-keyout", keyPath, "-out", certPath, "-days", "1", "-nodes", "-subj", "/CN=t.example")
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("no openssl to build the fixture certificate: %v: %s", err, out)
	}

	cfg := func(cc string) string {
		return `{
		  "log": {"level": "error"},
		  "inbounds": [{
		    "type": "naive", "tag": "naive-in", "listen": "::", "listen_port": 8443,
		    "quic_congestion_control": "` + cc + `",
		    "users": [{"username": "u", "password": "pw"}],
		    "tls": {"enabled": true, "certificate_path": "` + certPath + `", "key_path": "` + keyPath + `"}
		  }],
		  "outbounds": [{"type": "direct", "tag": "direct"}]
		}`
	}
	write := func(body string) string {
		p := filepath.Join(dir, "config.json")
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	// Every value the panel offers, including the two the outbound form has no
	// button for — those are the ones most likely to be lost in a rebase.
	for _, cc := range []string{"bbr", "bbr_standard", "bbr2", "bbr2_variant", "cubic", "reno"} {
		if ok, errs := CheckConfigWith(binary, write(cfg(cc))); !ok {
			t.Fatalf("this build rejects quic_congestion_control %q on a naive inbound: %v", cc, errs)
		}
	}

	// The other half of the contract is NOT symmetric with hysteria2's, and that
	// is a measured fact rather than an oversight: an out-of-set value ("turbo")
	// passes `check` on 1.14.0-rc.1-awgm.14 and the inbound still comes up. The
	// binary is no safety net here — the panel's fixed button set is the only
	// thing keeping bad values out. What CAN be pinned is that the key is real:
	// decoding is strict about unknown fields, so a typo is a hard failure, and
	// that is what proves the option exists rather than being swallowed.
	bogus := strings.Replace(cfg("bbr"), `"quic_congestion_control"`, `"quic_congestion_controlx"`, 1)
	ok, errs := CheckConfigWith(binary, write(bogus))
	if ok {
		t.Fatal("an unknown inbound field was accepted; this build no longer decodes strictly, so the positive half above proves nothing")
	}
	if !strings.Contains(strings.Join(errs, "\n"), "quic_congestion_controlx") {
		t.Fatalf("rejected for some other reason than the unknown field, fixture may be stale: %v", errs)
	}
}
