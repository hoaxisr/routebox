package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"routebox/backend/internal/auth"
)

func TestUpdateNumericSettings(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		value   interface{}
		wantErr bool
		want    int
		get     func(s Settings) int
	}{
		{"float64 accepted (JSON decode)", "monitoring.poll_interval_ms", float64(1500), false, 1500,
			func(s Settings) int { return s.Monitoring.PollIntervalMs }},
		{"int accepted", "monitoring.max_closed_connections", 250, false, 250,
			func(s Settings) int { return s.Monitoring.MaxClosedConnections }},
		{"json.Number accepted", "advanced.ws_ping_interval_sec", json.Number("45"), false, 45,
			func(s Settings) int { return s.Advanced.WsPingIntervalSec }},
		{"float64 accepted", "advanced.ws_pong_timeout_sec", float64(15), false, 15,
			func(s Settings) int { return s.Advanced.WsPongTimeoutSec }},
		{"fractional float rejected", "monitoring.poll_interval_ms", float64(1.5), true, 0, nil},
		{"string rejected", "monitoring.proxies_refresh_ms", "fast", true, 0, nil},
		{"bool rejected for numeric", "security.session_timeout_minutes", true, true, 0, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &Manager{settings: Default()}
			err := m.Update(map[string]interface{}{tc.key: tc.value})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s=%v, got nil", tc.key, tc.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := tc.get(m.Get()); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestUpdateBoolFieldsUnaffected(t *testing.T) {
	m := &Manager{settings: Default()}
	if err := m.Update(map[string]interface{}{"geoip.enabled": false}); err != nil {
		t.Fatalf("bool update failed: %v", err)
	}
	if m.Get().GeoIP.Enabled {
		t.Fatal("geoip.enabled not applied")
	}
}

func TestUpdatesSettings(t *testing.T) {
	def := Default()
	if !def.Updates.AutoCheck {
		t.Error("updates.auto_check must default to true")
	}
	m := &Manager{settings: Default()}
	if err := m.Update(map[string]interface{}{"updates.auto_check": false}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if m.Get().Updates.AutoCheck {
		t.Error("updates.auto_check not applied")
	}
}

func TestPasswordMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "routebox.toml")
	if err := os.WriteFile(path, []byte("[security]\nauth_enabled = true\nauth_username = \"admin\"\nauth_password = \"plain123\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	s := m.Get()
	if s.Security.AuthPassword != "" {
		t.Fatalf("plaintext password must be blanked after migration, got %q", s.Security.AuthPassword)
	}
	if s.Security.AuthPasswordHash == "" {
		t.Fatal("hash must be populated by migration")
	}
	if !auth.VerifyPassword(s.Security.AuthPasswordHash, "plain123") {
		t.Fatal("migrated hash must verify the original password")
	}
	m2, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if m2.Get().Security.AuthPasswordHash == "" || m2.Get().Security.AuthPassword != "" {
		t.Fatal("migration must persist to disk")
	}
}

func TestDefaultsPhase1(t *testing.T) {
	d := Default()
	if d.Security.SessionTimeoutMinutes != 720 {
		t.Fatalf("session timeout default should be 720, got %d", d.Security.SessionTimeoutMinutes)
	}
	if d.Server.Mode != "router" {
		t.Fatalf("default mode should be router, got %q", d.Server.Mode)
	}
}

// TestReHashOnNewPlaintext (Fix 1): after a previous migration (hash set, no
// plaintext), writing a NEW plaintext auth_password alongside the old hash
// must re-hash to the new password and discard the old hash.
func TestReHashOnNewPlaintext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "routebox.toml")

	// First load: migrate "oldpass" → hash
	if err := os.WriteFile(path, []byte("[security]\nauth_enabled = true\nauth_username = \"admin\"\nauth_password = \"oldpass\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path)
	if err != nil {
		t.Fatalf("NewManager (first load): %v", err)
	}
	oldHash := m.Get().Security.AuthPasswordHash
	if oldHash == "" {
		t.Fatal("expected hash after first migration")
	}

	// Simulate a password reset: write a new plaintext alongside the existing hash.
	toml := fmt.Sprintf("[security]\nauth_enabled = true\nauth_username = \"admin\"\nauth_password = \"newpass\"\nauth_password_hash = %q\n", oldHash)
	if err := os.WriteFile(path, []byte(toml), 0600); err != nil {
		t.Fatal(err)
	}

	// Second load: plaintext present → must win over stale hash.
	if err := m.Load(); err != nil {
		t.Fatalf("Load (second): %v", err)
	}
	s := m.Get()

	if s.Security.AuthPassword != "" {
		t.Fatalf("plaintext must be blanked after re-migration, got %q", s.Security.AuthPassword)
	}
	if !auth.VerifyPassword(s.Security.AuthPasswordHash, "newpass") {
		t.Error("new hash must verify the NEW password")
	}
	if auth.VerifyPassword(s.Security.AuthPasswordHash, "oldpass") {
		t.Error("new hash must NOT verify the old password")
	}
}

// TestMalformedReloadKeepsOldSettings (Fix 5): a decode error during Load must
// leave m.settings unchanged (auth must not silently revert to defaults).
func TestMalformedReloadKeepsOldSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "routebox.toml")

	// Write a valid config with auth enabled.
	if err := os.WriteFile(path, []byte("[security]\nauth_enabled = true\nauth_username = \"admin\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if !m.Get().Security.AuthEnabled {
		t.Fatal("auth_enabled should be true after first load")
	}

	// Overwrite with malformed TOML.
	if err := os.WriteFile(path, []byte("[[[[not valid toml"), 0600); err != nil {
		t.Fatal(err)
	}

	loadErr := m.Load()
	if loadErr == nil {
		t.Fatal("Load must return an error for malformed TOML")
	}
	if !m.Get().Security.AuthEnabled {
		t.Fatal("m.settings must remain unchanged after a failed Load (auth_enabled must still be true)")
	}
}

// TestInvalidServerModeErrors (Fix 4): Update with an invalid server.mode must
// return a non-nil error.
func TestInvalidServerModeErrors(t *testing.T) {
	m := &Manager{settings: Default()}
	err := m.Update(map[string]interface{}{"server.mode": "nope"})
	if err == nil {
		t.Fatal("expected error for invalid server.mode, got nil")
	}
}

func TestSanitizePublicHost(t *testing.T) {
	tests := []struct {
		in, want string
		wantErr  bool
	}{
		{"vpn.example.com", "vpn.example.com", false},
		{"https://vpn.example.com", "vpn.example.com", false},
		{"http://vpn.example.com/", "vpn.example.com", false},
		{"https://vpn.example.com:8443/path?x=1", "vpn.example.com", false},
		{"203.0.113.5", "203.0.113.5", false},
		{"https://203.0.113.5:443", "203.0.113.5", false},
		{"2001:db8::1", "2001:db8::1", false},
		{"[2001:db8::1]:443", "2001:db8::1", false},
		{"", "", false}, // empty clears the setting
		{"  vpn.example.com  ", "vpn.example.com", false},
		{"not a host!!", "", true}, // garbage rejected
		{"bad_host_underscore", "", true},
		// Userinfo must be stripped; the no-scheme case previously accepted
		// "user:pass@host" as "user" because SplitHostPort split on the colon.
		{"user:pass@vpn.example.com", "vpn.example.com", false}, // bug guard
		{"admin@vpn.example.com:8443", "vpn.example.com", false},
		// Hyphen-edge labels (RFC 952/1123) rejected.
		{"-leadinghyphen.com", "", true},
		{"trailinghyphen-.com", "", true},
		// Absolute FQDN trailing dot accepted.
		{"vpn.example.com.", "vpn.example.com", false},
		// Scheme-only / whitespace / spaces / bare host with port.
		{"https://", "", true}, // scheme only, no host
		{"   ", "", false},     // whitespace only clears the setting
		{"host with spaces", "", true},
		{"bare-host:8443", "bare-host", false},
		// Case is preserved (not lowercased).
		{"VPN.Example.COM", "VPN.Example.COM", false},
	}
	for _, tt := range tests {
		got, err := SanitizePublicHost(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("%q: expected error", tt.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%q: unexpected error %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("%q: got %q want %q", tt.in, got, tt.want)
		}
	}
}

func TestUpdateServerPublicHost(t *testing.T) {
	m := &Manager{settings: Default(), path: ""}
	if err := m.Update(map[string]interface{}{"server.public_host": "https://vpn.example.com:8443/x"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := m.Get().Server.PublicHost; got != "vpn.example.com" {
		t.Fatalf("got %q", got)
	}
	if err := m.Update(map[string]interface{}{"server.public_host": "bad host!!"}); err == nil {
		t.Fatalf("expected error for invalid host")
	}
	// A rejected update must leave the previously stored value unchanged.
	if got := m.Get().Server.PublicHost; got != "vpn.example.com" {
		t.Fatalf("rejected update must not modify PublicHost; got %q", got)
	}
}

func TestACMEAndPublicPortDefaults(t *testing.T) {
	d := Default()
	if d.Network.ACMEEnabled {
		t.Error("network.acme_enabled must default to false")
	}
	if d.Network.ACMEStaging {
		t.Error("network.acme_staging must default to false")
	}
	if d.Network.ACMEEmail != "" {
		t.Errorf("network.acme_email must default to empty, got %q", d.Network.ACMEEmail)
	}
	if d.Network.ACMECacheDir != "/etc/routebox/acme" {
		t.Errorf("network.acme_cache_dir default should be /etc/routebox/acme, got %q", d.Network.ACMECacheDir)
	}
	if d.Server.PublicPort != 0 {
		t.Errorf("server.public_port must default to 0 (no explicit port), got %d", d.Server.PublicPort)
	}
}

// TestSaveCreatesParentDir verifies that Save creates all intermediate
// directories so a fresh-VPS first run (no pre-existing config file) can
// persist the bootstrapped auth settings without crashing.
func TestSaveCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "nested", "routebox.toml") // parent dirs don't exist
	// NewManager with an explicit path succeeds even when the file does not yet
	// exist (it falls back to defaults and logs a notice).
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("Save must create parent dirs: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("settings file should exist after Save: %v", err)
	}
}

func TestUpdateACMEStringAndBoolFields(t *testing.T) {
	m := &Manager{settings: Default()}
	if err := m.Update(map[string]interface{}{
		"network.acme_enabled":   true,
		"network.acme_staging":   true,
		"network.acme_email":     "ops@example.com",
		"network.acme_cache_dir": "/var/lib/routebox/acme",
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	s := m.Get()
	if !s.Network.ACMEEnabled {
		t.Error("acme_enabled not applied")
	}
	if !s.Network.ACMEStaging {
		t.Error("acme_staging not applied")
	}
	if s.Network.ACMEEmail != "ops@example.com" {
		t.Errorf("acme_email got %q", s.Network.ACMEEmail)
	}
	if s.Network.ACMECacheDir != "/var/lib/routebox/acme" {
		t.Errorf("acme_cache_dir got %q", s.Network.ACMECacheDir)
	}
}

func TestUpdateACMEWrongTypesRejected(t *testing.T) {
	cases := []struct {
		key   string
		value interface{}
	}{
		{"network.acme_enabled", "yes"}, // string for bool
		{"network.acme_staging", 1},     // int for bool
		{"network.acme_email", true},    // bool for string
		{"network.acme_cache_dir", 42},  // int for string
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			m := &Manager{settings: Default()}
			if err := m.Update(map[string]interface{}{tc.key: tc.value}); err == nil {
				t.Fatalf("%s=%v: expected error, got nil", tc.key, tc.value)
			}
		})
	}
}

func TestUpdateServerPublicPort(t *testing.T) {
	cases := []struct {
		name    string
		value   interface{}
		wantErr bool
		want    int
	}{
		{"valid port", 8443, false, 8443},
		{"zero (unset)", 0, false, 0},
		{"min", 1, false, 1},
		{"max port", 65535, false, 65535},
		{"json float", float64(8443), false, 8443},
		{"negative rejected", -1, true, 0},
		{"too large rejected", 65536, true, 0},
		{"string rejected", "8443", true, 0},
		{"fractional rejected", float64(8443.5), true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &Manager{settings: Default()}
			err := m.Update(map[string]interface{}{"server.public_port": tc.value})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %v, got nil", tc.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := m.Get().Server.PublicPort; got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}

	// A rejected update must not mutate the stored value.
	m := &Manager{settings: Default()}
	_ = m.Update(map[string]interface{}{"server.public_port": 8443})
	if err := m.Update(map[string]interface{}{"server.public_port": 70000}); err == nil {
		t.Fatal("expected error for out-of-range port")
	}
	if m.Get().Server.PublicPort != 8443 {
		t.Fatalf("rejected update must not modify PublicPort; got %d", m.Get().Server.PublicPort)
	}
}
