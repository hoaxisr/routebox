package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The panel writes hysteria2 congestion-control keys (#59) that upstream sing-box
// does not have — `bbr_profile` on both sides and `ignore_client_bandwidth` on the
// inbound are fork additions. Nothing in the panel can tell whether the binary
// still takes them, and an unknown key is a decode FATAL: the VPN would not come
// back up. So pin the contract against the real binary. Skips without one; CI
// installs a published amnezia-box before the tests run.
func TestHysteria2CongestionKeys_AcceptedByBinary(t *testing.T) {
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
	// hysteria2 inbounds refuse to start without a usable certificate, so the
	// fixture carries a throwaway one — this test is about the congestion keys,
	// not about TLS.
	certPath, keyPath := filepath.Join(dir, "c.pem"), filepath.Join(dir, "k.pem")
	gen := exec.Command("openssl", "req", "-x509", "-newkey", "rsa:2048",
		"-keyout", keyPath, "-out", certPath, "-days", "1", "-nodes", "-subj", "/CN=t.example")
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("no openssl to build the fixture certificate: %v: %s", err, out)
	}

	cfg := func(profile string) string {
		return `{
		  "log": {"level": "error"},
		  "inbounds": [{
		    "type": "hysteria2", "tag": "hy2-in", "listen": "::", "listen_port": 8443,
		    "up_mbps": 100, "down_mbps": 200,
		    "ignore_client_bandwidth": true,
		    "bbr_profile": "` + profile + `",
		    "users": [{"name": "p", "password": "pw"}],
		    "tls": {"enabled": true, "certificate_path": "` + certPath + `", "key_path": "` + keyPath + `"}
		  }],
		  "outbounds": [
		    {"type": "hysteria2", "tag": "hy2-out", "server": "1.2.3.4", "server_port": 443,
		     "password": "pw", "bbr_profile": "conservative",
		     "tls": {"enabled": true, "server_name": "t.example"}},
		    {"type": "direct", "tag": "direct"}
		  ]
		}`
	}
	write := func(body string) string {
		p := filepath.Join(dir, "config.json")
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	if ok, errs := CheckConfigWith(binary, write(cfg("aggressive"))); !ok {
		t.Fatalf("this build rejects the congestion-control keys the panel writes: %v", errs)
	}

	// The other half of the contract: the value set is closed, so a profile the
	// panel does not offer must NOT quietly pass. If this ever stops failing, the
	// buttons are no longer the only thing keeping bad values out.
	ok, errs := CheckConfigWith(binary, write(cfg("turbo")))
	if ok {
		t.Fatal("an unknown BBR profile was accepted; the panel's fixed button set is no longer a guarantee")
	}
	if !strings.Contains(strings.Join(errs, "\n"), "BBR profile") {
		t.Fatalf("rejected for some other reason than the profile, fixture may be stale: %v", errs)
	}
}
