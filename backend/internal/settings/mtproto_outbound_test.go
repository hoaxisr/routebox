package settings

import (
	"os"
	"path/filepath"
	"testing"
)

// A settings file written before the Telegram exit existed must not surface the
// new keys as zeros. socks_port renders straight into a form field, and a bare 0
// there is exactly the "looks like a real port nobody chose" problem that issue
// #62 was raised about — reintroducing it on every upgrade would be worse.
func TestUpgradeFromASettingsFileWithoutTheOutboundKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routebox.toml")

	old := `[mtproto]
  enabled = true
  listen = "0.0.0.0:9443"
  masking_domain = "www.microsoft.com"
  concurrency = 4096
  idle_timeout_sec = 300
  domain_fronting_port = 443
`

	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}

	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}

	got := m.Get().Mtproto

	// Absent means direct, which is what every install did before this existed.
	if got.Outbound != "" {
		t.Errorf("Outbound = %q, want empty (direct)", got.Outbound)
	}

	// 1080 rather than mtproto.DefaultSocksPort: this package stays protocol
	// agnostic, and the two cannot drift harmfully — a 0 here is resolved to the
	// same default by mtproto.SocksPortOrDefault at dial time.
	if got.SocksPort != 1080 {
		t.Errorf("SocksPort = %d, want the 1080 default rather than a bare 0", got.SocksPort)
	}

	// ...and nothing that was already there may be lost on the way.
	if got.MaskingDomain != "www.microsoft.com" || got.Listen != "0.0.0.0:9443" {
		t.Errorf("existing settings were disturbed: %+v", got)
	}
}

// Round-trips the new keys so a save cannot quietly drop them.
func TestOutboundSettingsSurviveASaveAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routebox.toml")

	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Update(map[string]interface{}{
		"mtproto.outbound":   "warp",
		"mtproto.socks_port": 1091,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	if err := m.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}

	got := reloaded.Get().Mtproto
	if got.Outbound != "warp" || got.SocksPort != 1091 {
		t.Errorf("after reload Outbound=%q SocksPort=%d, want warp/1091", got.Outbound, got.SocksPort)
	}
}

func TestSocksPortRejectsAnImpossibleValue(t *testing.T) {
	m, err := NewManager(filepath.Join(t.TempDir(), "routebox.toml"))
	if err != nil {
		t.Fatal(err)
	}

	for _, port := range []int{-1, 70000} {
		if err := m.Update(map[string]interface{}{"mtproto.socks_port": port}); err == nil {
			t.Errorf("port %d: want an error, got none", port)
		}
	}

	// 0 is allowed and means "use the default" — the settings file predating
	// this feature has no value at all, and that has to stay loadable.
	if err := m.Update(map[string]interface{}{"mtproto.socks_port": 0}); err != nil {
		t.Errorf("port 0 should be accepted as unset: %v", err)
	}
}
