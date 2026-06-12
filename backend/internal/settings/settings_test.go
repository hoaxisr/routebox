package settings

import (
	"encoding/json"
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
