package settings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"routebox/backend/internal/util"
)

func unwritableSettingsPath(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}
	dir := filepath.Join(t.TempDir(), "etc")
	if err := os.Mkdir(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })
	return filepath.Join(dir, "routebox.toml")
}

// routebox.toml is RouteBox's own settings file — the one place where "the panel
// cannot save" is most confusing without a reason attached.
func TestSaveReportsReadOnlyWithThePath(t *testing.T) {
	path := unwritableSettingsPath(t)
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}

	err = m.Save()
	if !errors.Is(err, util.ErrReadOnly) {
		t.Fatalf("Save error = %v, want util.ErrReadOnly", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error %q must name %q", err, path)
	}
	if !m.IsReadOnly() {
		t.Fatal("the manager must report the state the badge shows")
	}
}

func TestWritableSettingsAreNotReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routebox.toml")
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.IsReadOnly() {
		t.Fatal("a writable path must not raise the badge")
	}
	if err := m.Save(); err != nil {
		t.Fatalf("Save = %v, want nil", err)
	}
	if m.IsReadOnly() {
		t.Fatal("a save that went through must leave the manager writable")
	}
}
