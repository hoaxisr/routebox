package settings

import (
	"path/filepath"
	"testing"
)

func TestDefaultMtprotoIsOffAndSafe(t *testing.T) {
	d := Default().Mtproto

	// It opens a public port and hands out credentials; opt-in only.
	if d.Enabled {
		t.Error("MTProto must default to off")
	}

	// 443 is usually the reverse proxy's and 8443 is the panel's own port in
	// the Docker image, so neither can be the default here.
	if d.Listen != "0.0.0.0:9443" {
		t.Errorf("Listen = %q, want 0.0.0.0:9443", d.Listen)
	}

	if d.MaskingDomain != "" {
		t.Errorf("MaskingDomain = %q, want empty — there is no sensible default host to impersonate", d.MaskingDomain)
	}

	if d.Concurrency == 0 {
		t.Error("Concurrency needs a non-zero default")
	}

	if d.IdleTimeoutSec == 0 {
		t.Error("IdleTimeoutSec needs a non-zero default")
	}

	if d.DomainFrontingPort == 0 {
		t.Error("DomainFrontingPort needs a non-zero default")
	}
}

func mtprotoManager(t *testing.T) *Manager {
	t.Helper()

	m, err := NewManager(filepath.Join(t.TempDir(), "routebox.toml"))
	if err != nil {
		t.Fatal(err)
	}

	return m
}

func TestMtprotoRuntimeKeysApply(t *testing.T) {
	m := mtprotoManager(t)

	if err := m.Update(map[string]any{
		"mtproto.enabled":              true,
		"mtproto.listen":               "0.0.0.0:9443",
		"mtproto.masking_domain":       "storage.googleapis.com",
		"mtproto.public_host":          "panel.example.com",
		"mtproto.public_port":          int64(443),
		"mtproto.concurrency":          int64(1024),
		"mtproto.idle_timeout_sec":     int64(60),
		"mtproto.prefer_ip":            "prefer-ipv4",
		"mtproto.domain_fronting_port": int64(8443),
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got := m.Get().Mtproto

	if !got.Enabled {
		t.Error("enabled did not apply")
	}

	if got.Listen != "0.0.0.0:9443" {
		t.Errorf("Listen = %q", got.Listen)
	}

	if got.MaskingDomain != "storage.googleapis.com" {
		t.Errorf("MaskingDomain = %q", got.MaskingDomain)
	}

	if got.PublicHost != "panel.example.com" || got.PublicPort != 443 {
		t.Errorf("public = %q:%d", got.PublicHost, got.PublicPort)
	}

	if got.Concurrency != 1024 || got.IdleTimeoutSec != 60 {
		t.Errorf("Concurrency = %d, IdleTimeoutSec = %d", got.Concurrency, got.IdleTimeoutSec)
	}

	if got.PreferIP != "prefer-ipv4" || got.DomainFrontingPort != 8443 {
		t.Errorf("PreferIP = %q, DomainFrontingPort = %d", got.PreferIP, got.DomainFrontingPort)
	}
}

func TestMtprotoSettingsPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routebox.toml")

	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Update(map[string]any{
		"mtproto.enabled":        true,
		"mtproto.masking_domain": "example.com",
	}); err != nil {
		t.Fatal(err)
	}

	// Update only stages; persistence is a separate step, as everywhere else.
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}

	got := reloaded.Get().Mtproto
	if !got.Enabled || got.MaskingDomain != "example.com" {
		t.Errorf("got %+v, want the values to survive a reload", got)
	}
}

func TestMtprotoRejectsAWrongTypedValue(t *testing.T) {
	m := mtprotoManager(t)

	// The API decodes JSON, so a client can send anything. Rejecting loudly
	// beats silently ignoring: the panel would otherwise report a successful
	// save for a setting that never changed.
	if err := m.Update(map[string]any{"mtproto.listen": 42}); err == nil {
		t.Error("a non-string listen must be rejected")
	}

	if got := m.Get().Mtproto.Listen; got != Default().Mtproto.Listen {
		t.Errorf("Listen = %q, want the default left untouched", got)
	}
}

func TestMtprotoRejectsAWrongTypedNumber(t *testing.T) {
	m := mtprotoManager(t)

	if err := m.Update(map[string]any{"mtproto.public_port": "443"}); err == nil {
		t.Error("a string port must be rejected")
	}
}

func TestARejectedMtprotoKeyLeavesTheWholeUpdateUnapplied(t *testing.T) {
	m := mtprotoManager(t)

	// Updates are staged and committed as one assignment, so a bad key in the
	// batch must not leave the good ones half-applied.
	_ = m.Update(map[string]any{
		"mtproto.masking_domain": "example.com",
		"mtproto.public_port":    "not a number",
	})

	if got := m.Get().Mtproto.MaskingDomain; got != "" {
		t.Errorf("MaskingDomain = %q, want it unapplied alongside the rejected key", got)
	}
}
