package settings

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultMtprotoWebIsOffOnLoopback(t *testing.T) {
	d := Default().Mtproto

	if d.WebEnabled {
		t.Error("WEB proxy must default to off")
	}
	if d.WebListen != "127.0.0.1:9446" {
		t.Errorf("WebListen = %q, want 127.0.0.1:9446", d.WebListen)
	}
	if d.WebHostname != "" || d.WebPublicDir != "" {
		t.Errorf("WebHostname = %q, WebPublicDir = %q, want both empty", d.WebHostname, d.WebPublicDir)
	}
}

func TestMtprotoWebKeysApply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routebox.toml")
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Update(map[string]any{
		"mtproto.web_enabled":    true,
		"mtproto.web_hostname":   "proxy.example.com",
		"mtproto.web_listen":     "127.0.0.1:9555",
		"mtproto.web_public_dir": "/srv/web",
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Update only stages; persistence is a separate step.
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	check := func(m *Manager) {
		t.Helper()
		got := m.Get().Mtproto
		if !got.WebEnabled || got.WebHostname != "proxy.example.com" ||
			got.WebListen != "127.0.0.1:9555" || got.WebPublicDir != "/srv/web" {
			t.Errorf("web settings = %+v", got)
		}
	}
	check(m)

	m2, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	check(m2)
}

func TestMtprotoWebHostnameIsValidated(t *testing.T) {
	m := mtprotoManager(t)

	for _, bad := range []string{"Proxy.Example.com", "proxy.example.com.", "proxy.example.com:443", "203.0.113.9"} {
		if err := m.Update(map[string]any{"mtproto.web_hostname": bad}); err == nil {
			t.Errorf("%q accepted, want error", bad)
		}
	}
	for _, good := range []string{"", "proxy.example.com"} {
		if err := m.Update(map[string]any{"mtproto.web_hostname": good}); err != nil {
			t.Errorf("%q rejected: %v", good, err)
		}
	}
}

func TestMtprotoWebListenIsValidated(t *testing.T) {
	m := mtprotoManager(t)

	for _, bad := range []string{"9446", "127.0.0.1:", "0.0.0.0:70000", "0.0.0.0:0"} {
		if err := m.Update(map[string]any{"mtproto.web_listen": bad}); err == nil {
			t.Errorf("%q accepted, want error", bad)
		}
	}
	for _, good := range []string{"0.0.0.0:9446", "[::1]:9446"} {
		if err := m.Update(map[string]any{"mtproto.web_listen": good}); err != nil {
			t.Errorf("%q rejected: %v", good, err)
		}
	}
}

// A bootstrapped install's Caddyfile dials the front at 127.0.0.1:<port>, so
// a front bound to any other host there would be one dest cannot reach.
func TestMtprotoWebListenOnABootstrappedInstallMustIncludeLoopback(t *testing.T) {
	m := mtprotoManager(t)
	if err := m.SetBootstrap("/panel-secret", "127.0.0.1:9443", "", 443); err != nil {
		t.Fatal(err)
	}

	for _, bad := range []string{"10.0.0.5:9446", "[::1]:9446", "localhost:9446", "127.0.0.2:9446"} {
		err := m.Update(map[string]any{"mtproto.web_listen": bad})
		if err == nil || !strings.Contains(err.Error(), "127.0.0.1") {
			t.Errorf("%q: err = %v, want a refusal naming 127.0.0.1", bad, err)
		}
	}
	for _, good := range []string{"127.0.0.1:9500", ":9501", "0.0.0.0:9502", "[::]:9503"} {
		if err := m.Update(map[string]any{"mtproto.web_listen": good}); err != nil {
			t.Errorf("%q rejected: %v", good, err)
		}
	}

	// A PUT echoing the current value back is not refused over it. (Boot's
	// migrateBootstrapMtproto moves such a value to 127.0.0.1, so it is not
	// the current one on a running install.)
	m.settings.Mtproto.WebListen = "[::1]:9446"
	if err := m.Update(map[string]any{"mtproto.web_listen": "[::1]:9446"}); err != nil {
		t.Errorf("the unchanged value was refused: %v", err)
	}
}

func TestEffectiveWebHostnameFollowsBootstrap(t *testing.T) {
	s := Default()
	s.Server.PublicHost = "panel.example.com"
	s.Mtproto.WebHostname = "proxy.example.com"

	if got := s.EffectiveWebHostname(); got != "proxy.example.com" {
		t.Errorf("not bootstrapped: got %q, want the web hostname", got)
	}

	s.Server.Bootstrapped = true
	if got := s.EffectiveWebHostname(); got != "panel.example.com" {
		t.Errorf("bootstrapped: got %q, want Server.PublicHost", got)
	}

	// The WEB handler takes only the canonical form; the public host is
	// validated for a URL, which allows more.
	s.Server.PublicHost = "Panel.Example.COM."
	if got := s.EffectiveWebHostname(); got != "panel.example.com" {
		t.Errorf("bootstrapped: got %q, want the public host lowercased without the trailing dot", got)
	}
}
