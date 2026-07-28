package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// The read-only verdict is taken at startup (and on a path switch) and would
// otherwise stick for the life of the process: nothing re-probes it. Fixing the
// permissions then required restarting RouteBox, while the UI told the user to
// reload the page. Every write guard re-probes when — and only when — the
// standing verdict is read-only, so a repaired config is picked up by the next
// action and a writable one never pays for the probe.

// stale puts a manager whose config IS writable into the read-only state, the
// way a startup verdict outlives the permissions it was taken from.
func stale(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"log":{"level":"info"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.IsReadOnly() {
		t.Fatal("harness: a writable config must not start read-only")
	}
	m.SetReadOnly(true)
	return m
}

func TestSaveRechecksAStaleReadOnlyVerdict(t *testing.T) {
	m := stale(t)
	if err := m.Save(map[string]interface{}{"log": map[string]interface{}{"level": "debug"}}); err != nil {
		t.Fatalf("Save on a writable config must succeed, got %v", err)
	}
	if m.IsReadOnly() {
		t.Fatal("the verdict must be lifted once the write goes through")
	}
}

func TestApplyDraftRechecksAStaleReadOnlyVerdict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"log":{"level":"info"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SetDraft(map[string]interface{}{"log": map[string]interface{}{"level": "debug"}}); err != nil {
		t.Fatal(err)
	}
	m.SetReadOnly(true)
	if err := m.ApplyDraft(); err != nil {
		t.Fatalf("ApplyDraft on a writable config must succeed, got %v", err)
	}
	if m.IsReadOnly() {
		t.Fatal("the verdict must be lifted once the write goes through")
	}
}

func TestSetDraftRechecksAStaleReadOnlyVerdict(t *testing.T) {
	m := stale(t)
	if err := m.SetDraft(map[string]interface{}{"log": map[string]interface{}{"level": "debug"}}); err != nil {
		t.Fatalf("SetDraft on a writable config must succeed, got %v", err)
	}
	if m.IsReadOnly() {
		t.Fatal("the verdict must be lifted once the draft is written")
	}
	if !m.HasDraft() {
		t.Fatal("the draft was dropped by a verdict that no longer holds")
	}
}

func TestSyncAwgEndpointActiveRechecksAStaleReadOnlyVerdict(t *testing.T) {
	m := stale(t)
	changed, err := m.SyncAwgEndpointActive("awg-server", &AwgServerSpec{
		PrivateKey: "aGVsbG8gd29ybGQgaGVsbG8gd29ybGQgaGVsbG8h",
		Address:    "10.10.0.1/24",
		ListenPort: 51820,
		MTU:        1408,
	})
	if err != nil {
		t.Fatalf("sync on a writable config must succeed, got %v", err)
	}
	if !changed {
		t.Fatal("the endpoint was not written")
	}
	if m.IsReadOnly() {
		t.Fatal("the verdict must be lifted once the write goes through")
	}
}

// The re-probe must not turn a genuinely unwritable config into a writable one:
// the verdict stands and the caller still gets ErrReadOnly, not a raw
// permission error from somewhere deeper.
func TestWriteGuardsKeepRefusingWhenTheConfigIsStillUnwritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"log":{"level":"info"}}`), 0400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0700) })
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if !m.IsReadOnly() {
		t.Fatal("harness: an unwritable config must start read-only")
	}
	err = m.Save(map[string]interface{}{"log": map[string]interface{}{"level": "debug"}})
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Save error = %v, want ErrReadOnly", err)
	}
	if !m.IsReadOnly() {
		t.Fatal("the verdict must stand while the config is still unwritable")
	}
}
