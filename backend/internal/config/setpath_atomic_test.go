package config

import (
	"os"
	"path/filepath"
	"testing"
)

// SetPath must be all-or-nothing. It is reachable from the panel (adopt the
// detected config path) with a path that does not exist — the unit's ExecStart
// can name a file nobody ever created. A half-applied switch leaves the manager
// pointing at the new path while still holding the OLD config in memory and the
// OLD read-only verdict, so the next Save writes the old contents to the new
// place.
func TestSetPathLeavesNothingBehindOnFailure(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "config.json")
	if err := os.WriteFile(good, []byte(`{"log":{"level":"info"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(good)
	if err != nil {
		t.Fatal(err)
	}

	missing := filepath.Join(dir, "elsewhere", "config.json")
	if err := m.SetPath(missing); err == nil {
		t.Fatal("SetPath must fail when the new config cannot be read")
	}

	if got := m.GetPath(); got != good {
		t.Fatalf("path = %q, want the old %q", got, good)
	}
	if got := m.draftPath; got != good+".bak" {
		t.Fatalf("draft path = %q, want the old %q", got, good+".bak")
	}
	if lvl, _ := m.GetActive()["log"].(map[string]interface{}); lvl == nil || lvl["level"] != "info" {
		t.Fatalf("active config was disturbed by a failed switch: %#v", m.GetActive())
	}

	// The real damage: a save after the failed switch must land on the old file.
	if err := m.Save(map[string]interface{}{"log": map[string]interface{}{"level": "debug"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(missing); err == nil {
		t.Fatalf("save went to %s — the failed switch was applied anyway", missing)
	}
	data, err := os.ReadFile(good)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" || !contains(string(data), "debug") {
		t.Fatalf("old config file did not receive the save:\n%s", data)
	}
}

// A successful switch still moves everything: path, draft path, contents.
func TestSetPathSwitchesOnSuccess(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "one.json")
	second := filepath.Join(dir, "two.json")
	if err := os.WriteFile(first, []byte(`{"log":{"level":"info"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte(`{"log":{"level":"warn"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SetPath(second); err != nil {
		t.Fatal(err)
	}
	if m.GetPath() != second || m.draftPath != second+".bak" {
		t.Fatalf("path=%q draft=%q, want %q", m.GetPath(), m.draftPath, second)
	}
	lvl, _ := m.GetActive()["log"].(map[string]interface{})
	if lvl == nil || lvl["level"] != "warn" {
		t.Fatalf("active config not loaded from the new path: %#v", m.GetActive())
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
