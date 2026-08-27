package settings

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
		{"float64 accepted (JSON decode)", "security.session_timeout_minutes", float64(1500), false, 1500,
			func(s Settings) int { return s.Security.SessionTimeoutMinutes }},
		{"int accepted", "server.public_port", 250, false, 250,
			func(s Settings) int { return s.Server.PublicPort }},
		{"json.Number accepted", "advanced.ws_ping_interval_sec", json.Number("45"), false, 45,
			func(s Settings) int { return s.Advanced.WsPingIntervalSec }},
		{"float64 accepted", "advanced.ws_pong_timeout_sec", float64(15), false, 15,
			func(s Settings) int { return s.Advanced.WsPongTimeoutSec }},
		{"fractional float rejected", "advanced.ws_ping_interval_sec", float64(1.5), true, 0, nil},
		{"string rejected", "advanced.ws_pong_timeout_sec", "fast", true, 0, nil},
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

// awg.server_host is the client-facing AWG address (router LAN/WAN IP or
// domain). It validates through SanitizePublicHost (scheme/port/path stripped,
// empty allowed) and defaults empty.
// awg.client_keepalive is the PersistentKeepalive clients get: seconds, or an
// AWG 3.0 "lo-hi" range. A value the device could not parse is rejected at save
// time — otherwise it would silently revert to 25 in every exported config.
func TestUpdateAwgClientKeepalive(t *testing.T) {
	m := &Manager{settings: Default(), path: ""}
	for _, ok := range []string{"25", "22-30", "0", "65535", "  22-30  ", ""} {
		if err := m.Update(map[string]interface{}{"awg.client_keepalive": ok}); err != nil {
			t.Fatalf("keepalive %q rejected: %v", ok, err)
		}
	}
	if got := m.Get().Awg.ClientKeepalive; got != "" {
		t.Fatalf("empty must clear the setting; got %q", got)
	}
	if err := m.Update(map[string]interface{}{"awg.client_keepalive": "22-30"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []interface{}{"abc", "30-22", "22-", "-5", "70000", "1-2-3", 25} {
		if err := m.Update(map[string]interface{}{"awg.client_keepalive": bad}); err == nil {
			t.Fatalf("keepalive %#v must be rejected", bad)
		}
	}
	if got := m.Get().Awg.ClientKeepalive; got != "22-30" {
		t.Fatalf("rejected updates must not modify the stored value; got %q", got)
	}
}

func TestUpdateAwgServerHost(t *testing.T) {
	if got := Default().Awg.ServerHost; got != "" {
		t.Fatalf("awg.server_host must default empty; got %q", got)
	}
	m := &Manager{settings: Default(), path: ""}
	if err := m.Update(map[string]interface{}{"awg.server_host": "https://192.168.1.200:8080/x"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := m.Get().Awg.ServerHost; got != "192.168.1.200" {
		t.Fatalf("got %q; want sanitized 192.168.1.200", got)
	}
	if err := m.Update(map[string]interface{}{"awg.server_host": "bad host!!"}); err == nil {
		t.Fatal("expected error for invalid awg.server_host")
	}
	// A rejected update must leave the previously stored value unchanged.
	if got := m.Get().Awg.ServerHost; got != "192.168.1.200" {
		t.Fatalf("rejected update must not modify ServerHost; got %q", got)
	}
	// Empty clears the setting (fallback to server.public_host resumes).
	if err := m.Update(map[string]interface{}{"awg.server_host": ""}); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	if got := m.Get().Awg.ServerHost; got != "" {
		t.Fatalf("clear failed; got %q", got)
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
	// Distinct values per field so a field mis-wired to its sibling is caught.
	if err := m.Update(map[string]interface{}{
		"network.acme_enabled":   true,
		"network.acme_staging":   false,
		"network.acme_email":     "ops@example.com",
		"network.acme_cache_dir": "/var/lib/routebox/acme",
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	s := m.Get()
	if !s.Network.ACMEEnabled {
		t.Error("acme_enabled not applied")
	}
	if s.Network.ACMEStaging {
		t.Error("acme_staging wrongly set (enabled↔staging swap?)")
	}
	if s.Network.ACMEEmail != "ops@example.com" {
		t.Errorf("acme_email got %q", s.Network.ACMEEmail)
	}
	if s.Network.ACMECacheDir != "/var/lib/routebox/acme" {
		t.Errorf("acme_cache_dir got %q", s.Network.ACMECacheDir)
	}
	// Inverse update proves the two bools are independently wired.
	if err := m.Update(map[string]interface{}{
		"network.acme_enabled": false,
		"network.acme_staging": true,
	}); err != nil {
		t.Fatalf("Update inverse: %v", err)
	}
	s = m.Get()
	if s.Network.ACMEEnabled {
		t.Error("acme_enabled not cleared (enabled↔staging swap?)")
	}
	if !s.Network.ACMEStaging {
		t.Error("acme_staging not applied (enabled↔staging swap?)")
	}
}

// TestACMEHTTPAddr covers the challenge listener address: it defaults to :80
// (the only port Let's Encrypt connects to for HTTP-01) and comes from the
// settings file, never from a request. A listen address does not fail the PUT
// that sets it — it fails the next startup, and keeps failing it, which under
// a restart policy is an outage recoverable only by hand-editing the file.
// network.listen is excluded for the same reason.
func TestACMEHTTPAddr(t *testing.T) {
	if got := Default().Network.ACMEHTTPAddr; got != ":80" {
		t.Errorf("network.acme_http_addr default should be :80, got %q", got)
	}
	m := &Manager{settings: Default()}
	if err := m.Update(map[string]interface{}{"network.acme_http_addr": ":0"}); err == nil {
		t.Fatal("network.acme_http_addr must be rejected by Update")
	}
	if got := m.Get().Network.ACMEHTTPAddr; got != ":80" {
		t.Fatalf("acme_http_addr changed despite the rejection: %q", got)
	}
	// What the API refuses, the file still carries.
	path := filepath.Join(t.TempDir(), "routebox.toml")
	fm, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	fm.settings.Network.ACMEHTTPAddr = ":8080"
	if err := fm.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reloaded, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Get().Network.ACMEHTTPAddr; got != ":8080" {
		t.Fatalf("acme_http_addr did not survive a save/load round trip: %q", got)
	}
}

// TestNoListenAddressIsSettableOverTheAPI keeps the class closed: every
// address the process binds at startup stays out of Update, so no single
// request can make the next boot fail.
func TestNoListenAddressIsSettableOverTheAPI(t *testing.T) {
	for _, key := range []string{"network.listen", "network.acme_http_addr"} {
		t.Run(key, func(t *testing.T) {
			m := &Manager{settings: Default()}
			if err := m.Update(map[string]interface{}{key: ":9999"}); err == nil {
				t.Fatalf("%s must not be settable through the settings API", key)
			}
		})
	}
}

// TestTrustedProxiesAreNotSettableOverTheAPI pins security.trusted_proxies out
// of the Update whitelist: it decides whose word RouteBox takes about who is
// calling, so — like the listen addresses and singbox.binary_path — it comes
// from the file only. Accepting it over PUT /api/settings would let any
// authenticated session widen the empty-by-default trust list and then forge
// rate-limit identities with a header.
func TestTrustedProxiesAreNotSettableOverTheAPI(t *testing.T) {
	m := &Manager{settings: Default()}
	if err := m.Update(map[string]interface{}{"security.trusted_proxies": []interface{}{"0.0.0.0/0"}}); err == nil {
		t.Fatal("security.trusted_proxies must be rejected by Update")
	}
	if got := m.Get().Security.TrustedProxies; len(got) != 0 {
		t.Fatalf("trusted_proxies changed despite the rejection: %v", got)
	}
}

// TestSingboxBinaryPathIsNotSettableOverTheAPI locks the one property that
// makes pinning the binary safe: singbox.binary_path is exec'd, so — like
// singbox.config_path — it comes from the file or the --binary flag only.
// Accepting it in Update would put arbitrary-command execution behind a
// PUT /api/settings from any authenticated session.
func TestSingboxBinaryPathIsNotSettableOverTheAPI(t *testing.T) {
	m := &Manager{settings: Default()}
	if err := m.Update(map[string]interface{}{"singbox.binary_path": "/tmp/evil"}); err == nil {
		t.Fatal("singbox.binary_path must be rejected by Update")
	}
	if got := m.Get().Singbox.BinaryPath; got != "" {
		t.Fatalf("binary_path changed despite the rejection: %q", got)
	}
}

// TestSingboxBinaryPathRoundTripsThroughTheFile is the other half: what the API
// refuses, the settings file must still carry.
func TestSingboxBinaryPathRoundTripsThroughTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routebox.toml")
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	m.settings.Singbox.BinaryPath = "/config/bin/amnezia-box"
	if err := m.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reloaded, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Get().Singbox.BinaryPath; got != "/config/bin/amnezia-box" {
		t.Fatalf("binary_path did not survive a save/load round trip: %q", got)
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

// TestV2RayAPIDefault verifies the loopback StatsService address default.
func TestV2RayAPIDefault(t *testing.T) {
	d := Default()
	if d.Singbox.V2RayAPI != "127.0.0.1:8081" {
		t.Fatalf("singbox.v2ray_api default should be 127.0.0.1:8081, got %q", d.Singbox.V2RayAPI)
	}
}

// TestUpdateV2RayAPI verifies the singbox.v2ray_api Update() whitelist case:
// round-trips a valid loopback value, rejects wrong types, rejects non-loopback,
// and leaves the stored value unchanged on a rejected update.
func TestUpdateV2RayAPI(t *testing.T) {
	cases := []struct {
		name    string
		value   interface{}
		wantErr bool
		want    string
	}{
		{"loopback round-trip", "127.0.0.1:9099", false, "127.0.0.1:9099"},
		{"ipv6 loopback", "[::1]:8081", false, "[::1]:8081"},
		{"int rejected", 8081, true, ""},
		{"bool rejected", true, true, ""},
		{"non-loopback rejected", "0.0.0.0:8081", true, ""},
		{"public ip rejected", "203.0.113.5:8081", true, ""},
		{"no port rejected", "127.0.0.1", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &Manager{settings: Default()}
			err := m.Update(map[string]interface{}{"singbox.v2ray_api": tc.value})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %v, got nil", tc.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := m.Get().Singbox.V2RayAPI; got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}

	// A rejected update must not mutate the stored value.
	m := &Manager{settings: Default()}
	if err := m.Update(map[string]interface{}{"singbox.v2ray_api": "127.0.0.1:9099"}); err != nil {
		t.Fatalf("valid update: %v", err)
	}
	if err := m.Update(map[string]interface{}{"singbox.v2ray_api": "0.0.0.0:8081"}); err == nil {
		t.Fatal("expected error for non-loopback address")
	}
	if got := m.Get().Singbox.V2RayAPI; got != "127.0.0.1:9099" {
		t.Fatalf("rejected update must not modify V2RayAPI; got %q", got)
	}
}

// TestLoadResetsNonLoopbackV2RayAPI (defense-in-depth): a hand-edited
// routebox.toml with a non-loopback singbox.v2ray_api must be reset to the
// loopback default on Load, while a valid loopback value survives unchanged.
func TestLoadResetsNonLoopbackV2RayAPI(t *testing.T) {
	t.Run("non-loopback reset to default", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "routebox.toml")
		if err := os.WriteFile(path, []byte("[singbox]\nv2ray_api = \"0.0.0.0:8081\"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		m, err := NewManager(path)
		if err != nil {
			t.Fatalf("NewManager: %v", err)
		}
		if got := m.Get().Singbox.V2RayAPI; got != "127.0.0.1:8081" {
			t.Fatalf("non-loopback v2ray_api must be reset to 127.0.0.1:8081; got %q", got)
		}
	})

	t.Run("public ip reset to default", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "routebox.toml")
		if err := os.WriteFile(path, []byte("[singbox]\nv2ray_api = \"203.0.113.5:8081\"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		m, err := NewManager(path)
		if err != nil {
			t.Fatalf("NewManager: %v", err)
		}
		if got := m.Get().Singbox.V2RayAPI; got != "127.0.0.1:8081" {
			t.Fatalf("public-ip v2ray_api must be reset to 127.0.0.1:8081; got %q", got)
		}
	})

	t.Run("valid loopback survives unchanged", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "routebox.toml")
		if err := os.WriteFile(path, []byte("[singbox]\nv2ray_api = \"127.0.0.1:9099\"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		m, err := NewManager(path)
		if err != nil {
			t.Fatalf("NewManager: %v", err)
		}
		if got := m.Get().Singbox.V2RayAPI; got != "127.0.0.1:9099" {
			t.Fatalf("valid loopback v2ray_api must survive Load unchanged; got %q", got)
		}
	})
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

func TestAwgSettingsObfDefaults(t *testing.T) {
	s := Default()
	if s.Awg.ObfPreset != "off" {
		t.Fatalf("default ObfPreset = %q, want off", s.Awg.ObfPreset)
	}
	if s.Awg.Obf.Jc != 0 || s.Awg.Obf.H1 != "" {
		t.Fatalf("default obf must be zero-value, got %+v", s.Awg.Obf)
	}
}

func TestUpdateAwgObf(t *testing.T) {
	m := &Manager{settings: Default()}
	err := m.Update(map[string]interface{}{
		"awg.obf_preset": "standard",
		"awg.dns":        []interface{}{"1.1.1.1", "8.8.8.8"},
		"awg.obf": map[string]interface{}{
			"jc": float64(4), "jmin": float64(40), "jmax": float64(70),
			"h1": "12345", "h2": "23456", "h3": "34567", "h4": "45678",
		},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	g := m.Get().Awg
	if g.ObfPreset != "standard" || g.Obf.Jc != 4 || g.Obf.H1 != "12345" {
		t.Fatalf("obf not applied: %+v", g)
	}
	if len(g.DNS) != 2 || g.DNS[1] != "8.8.8.8" {
		t.Fatalf("dns not applied: %v", g.DNS)
	}
}

// TestUpdateIsAtomic pins the all-or-nothing contract of Update: if ANY key in
// the payload is rejected, the in-memory settings must be exactly what they
// were before the call. Anything half-applied survives the failed request and
// is silently flushed to disk by the next successful Save().
//
// The live trigger is a browser holding a stale SPA build: it posts the whole
// settings form, including keys this build has since dropped ("ui.theme" here
// — removed because nothing read it). The user sees "Failed to save settings"
// while part of the payload has in fact been applied.
//
// The test is deterministic, not probabilistic: Update walks keys in sorted
// order, and the rejected key is chosen to sort AFTER every accepted key. So a
// partial-apply Update stages all 15 accepted keys before it reaches the bad
// one and fails this test on every single run — no repetition needed, no way
// to get lucky on map order. The guard loop below keeps that property honest
// if someone later adds a key to the payload.
func TestUpdateIsAtomic(t *testing.T) {
	// Dropped in this build; a cached SPA still sends it. Sorts last.
	const badKey = "ui.time_format"

	// Every value differs from Default() so applying any single one is visible.
	accepted := map[string]interface{}{
		"advanced.ws_ping_interval_sec":    99,
		"awg.dns":                          []interface{}{"9.9.9.9"},
		"awg.listen_port":                  51999,
		"geoip.enabled":                    false,
		"monitoring.enrichment_enabled":    false,
		"security.auth_enabled":            true,
		"security.auth_password":           "correct-horse-battery-staple",
		"security.auth_username":           "operator",
		"security.session_timeout_minutes": 60,
		"server.mode":                      "vps",
		"server.public_host":               "vpn.example.com",
		"server.public_port":               8443,
		"singbox.v2ray_api":                "127.0.0.1:9099",
		"ui.language":                      "ru",
		"ui.speed_unit":                    "bits",
	}

	// The determinism of this test rests on badKey being visited last. Assert
	// it instead of trusting it: a new payload key sorting after badKey would
	// otherwise quietly turn this back into a coin flip.
	for k := range accepted {
		if k >= badKey {
			t.Fatalf("payload key %q sorts at or after the rejected key %q; "+
				"this test needs the rejected key to come last", k, badKey)
		}
	}

	payload := map[string]interface{}{badKey: "24h"}
	maps.Copy(payload, accepted)

	m := &Manager{settings: Default()}
	err := m.Update(payload)
	if err == nil {
		t.Fatalf("unknown key %s must be rejected, got nil error", badKey)
	}
	if !strings.Contains(err.Error(), badKey) {
		t.Fatalf("error must name the offending key, got %q", err)
	}

	got, want := m.Get(), Default()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Update returned %v but left settings mutated\n got: %+v\nwant: %+v", err, got, want)
	}
	// Called out separately: bcrypt hashing is a transform Update performs on
	// the way in, and it must not run for a payload that is rejected.
	if got.Security.AuthPasswordHash != "" || got.Security.AuthPassword != "" {
		t.Fatal("password was hashed/stored despite the rejected update")
	}
}

// TestUpdateReportsLowestBadKey pins the reason Update sorts its keys: with
// several rejected keys in one payload the user must get the same message
// every time. Under map iteration order the reported key was whichever came
// up first, so the same stale form could produce a different error on refresh.
func TestUpdateReportsLowestBadKey(t *testing.T) {
	for i := 0; i < 20; i++ {
		m := &Manager{settings: Default()}
		err := m.Update(map[string]interface{}{
			"monitoring.poll_interval_ms": 1000,
			"ui.theme":                    "dark",
			"geoip.auto_reload":           true,
		})
		if err == nil {
			t.Fatal("expected an error for a payload of dropped keys")
		}
		if !strings.Contains(err.Error(), "geoip.auto_reload") {
			t.Fatalf("iteration %d: want the lowest-sorting bad key reported, got %q", i, err)
		}
	}
}

// server.front_port is the external port fronting loopback-bound inbounds. It
// validates like server.public_port but is a DIFFERENT field: the two are set
// independently (a panel on 8443 behind a front on 443 is a valid layout), so
// this pins that writing one never moves the other.
func TestUpdateServerFrontPort(t *testing.T) {
	m := &Manager{settings: Default(), path: ""}
	if err := m.Update(map[string]interface{}{"server.public_port": 8443}); err != nil {
		t.Fatalf("Update public_port: %v", err)
	}
	if err := m.Update(map[string]interface{}{"server.front_port": 443}); err != nil {
		t.Fatalf("Update front_port: %v", err)
	}
	if got := m.Get().Server.FrontPort; got != 443 {
		t.Fatalf("FrontPort = %d, want 443", got)
	}
	if got := m.Get().Server.PublicPort; got != 8443 {
		t.Fatalf("front_port must not touch PublicPort; got %d, want 8443", got)
	}

	for _, bad := range []interface{}{-1, 65536, "443"} {
		if err := m.Update(map[string]interface{}{"server.front_port": bad}); err == nil {
			t.Fatalf("expected an error for front_port %v", bad)
		}
	}
	if got := m.Get().Server.FrontPort; got != 443 {
		t.Fatalf("rejected update must not modify FrontPort; got %d", got)
	}
}
