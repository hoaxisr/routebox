package settings

import (
	"os"
	"path/filepath"
	"testing"
)

// legacyTOML is a routebox.toml as shipped before the dead-settings cleanup.
// Every key below was parsed into a struct field that nothing ever read, so the
// fields were removed. The file itself still lives on every existing install and
// MUST keep loading: toml.DecodeFile ignores keys with no matching field (Load
// discards MetaData and never inspects Undecoded()).
const legacyTOML = `
[geoip]
path = "/etc/routebox/geoip.mmdb"
enabled = true
auto_reload = true

[ui]
theme = "dark"
language = "ru"
speed_unit = "bits"
time_format = "absolute"

[monitoring]
enrichment_enabled = false
max_closed_connections = 1234
poll_interval_ms = 4321
proxies_refresh_ms = 7000

[logging]
level = "debug"
output = "/var/log/routebox.log"
timestamps = false
format = "json"

[security]
auth_enabled = true
auth_username = "admin"
session_timeout_minutes = 60

[network]
listen = "127.0.0.1:9099"
read_timeout_sec = 45
write_timeout_sec = 55
compression_enabled = false

[singbox]
config_path = "/etc/amnezia-box/config.json"
clash_api = "127.0.0.1:9090"
binary_name = "amnezia-box"
service_name = "amnezia-box.service"
v2ray_api = "127.0.0.1:8081"

[advanced]
debug_endpoints = true
pprof_enabled = true
max_body_size = 999999
ws_ping_interval_sec = 25
ws_pong_timeout_sec = 7
`

// TestLoadLegacyConfigWithRemovedKeys is the compatibility guard for the
// dead-settings removal: an on-disk config full of keys that no longer map to
// any struct field must load without error, and every surviving key must still
// be applied.
func TestLoadLegacyConfigWithRemovedKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "routebox.toml")
	if err := os.WriteFile(path, []byte(legacyTOML), 0600); err != nil {
		t.Fatal(err)
	}

	m, err := NewManager(path)
	if err != nil {
		t.Fatalf("legacy config must load without error, got: %v", err)
	}
	s := m.Get()

	// Surviving fields must still round-trip from the legacy file.
	checks := []struct {
		name      string
		got, want interface{}
	}{
		{"geoip.path", s.GeoIP.Path, "/etc/routebox/geoip.mmdb"},
		{"geoip.enabled", s.GeoIP.Enabled, true},
		{"ui.language", s.UI.Language, "ru"},
		{"ui.speed_unit", s.UI.SpeedUnit, "bits"},
		{"monitoring.enrichment_enabled", s.Monitoring.EnrichmentEnabled, false},
		{"security.auth_enabled", s.Security.AuthEnabled, true},
		{"security.auth_username", s.Security.AuthUsername, "admin"},
		{"security.session_timeout_minutes", s.Security.SessionTimeoutMinutes, 60},
		{"network.listen", s.Network.Listen, "127.0.0.1:9099"},
		{"network.read_timeout_sec", s.Network.ReadTimeoutSec, 45},
		{"network.compression_enabled", s.Network.CompressionEnabled, false},
		{"singbox.config_path", s.Singbox.ConfigPath, "/etc/amnezia-box/config.json"},
		{"singbox.clash_api", s.Singbox.ClashAPI, "127.0.0.1:9090"},
		{"singbox.v2ray_api", s.Singbox.V2RayAPI, "127.0.0.1:8081"},
		{"advanced.ws_ping_interval_sec", s.Advanced.WsPingIntervalSec, 25},
		{"advanced.ws_pong_timeout_sec", s.Advanced.WsPongTimeoutSec, 7},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}

	// Re-saving must succeed and the rewritten file must load again cleanly:
	// Save() rewrites the file with only the surviving keys, dropping the dead
	// ones for good.
	if err := m.Save(); err != nil {
		t.Fatalf("Save after legacy load: %v", err)
	}
	if err := m.Load(); err != nil {
		t.Fatalf("re-Load after Save: %v", err)
	}
	if got := m.Get().Network.Listen; got != "127.0.0.1:9099" {
		t.Errorf("network.listen after save/reload = %q, want 127.0.0.1:9099", got)
	}
}

// TestLoadIgnoresWhollyUnknownKeys pins the decoder contract itself: a config
// with keys and whole sections RouteBox has never known must not fail Load.
func TestLoadIgnoresWhollyUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "routebox.toml")
	body := "[security]\nauth_enabled = true\nfuture_field = \"whatever\"\n\n[not_a_section]\nx = 1\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path)
	if err != nil {
		t.Fatalf("unknown keys must not fail Load, got: %v", err)
	}
	if !m.Get().Security.AuthEnabled {
		t.Error("known key alongside unknown ones must still be applied")
	}
}
