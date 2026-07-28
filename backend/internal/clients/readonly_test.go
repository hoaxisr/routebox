package clients

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"routebox/backend/internal/util"
)

// clients.toml lives on the same partition as the rest of RouteBox's state, so
// it goes read-only with it. Renaming a device then produced a raw 500 with no
// badge and no path — the exact failure the read-only mode exists to replace.

func unwritable(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}
	dir := filepath.Join(t.TempDir(), "etc")
	if err := os.Mkdir(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })
	return filepath.Join(dir, "clients.toml")
}

func TestSaveReportsReadOnlyWithThePath(t *testing.T) {
	path := unwritable(t)
	m := New(path)
	if err := m.SetName("10.0.0.5", "laptop", ""); err != nil {
		t.Fatal(err)
	}

	err := m.Save()
	if !errors.Is(err, util.ErrReadOnly) {
		t.Fatalf("Save error = %v, want util.ErrReadOnly so the API can answer 409", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error %q must name %q — the file the user has to make writable", err, path)
	}
	if !m.IsReadOnly() {
		t.Fatal("the store must report the state the badge shows")
	}
	if m.GetPath() != path {
		t.Fatalf("GetPath() = %q, want %q", m.GetPath(), path)
	}
}

func TestAWritableStoreIsNotReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clients.toml")
	m := New(path)
	if m.IsReadOnly() {
		t.Fatal("a writable path must not raise the badge")
	}
	if err := m.SetName("10.0.0.5", "laptop", ""); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("Save = %v, want nil", err)
	}
	if m.IsReadOnly() {
		t.Fatal("a save that went through must leave the store writable")
	}
}

// Persistence-disabled stores never write, so they must never claim to be
// blocked — otherwise every deployment without a state file wears the badge.
func TestAPathlessStoreIsNeverReadOnly(t *testing.T) {
	if New("").IsReadOnly() {
		t.Fatal("no path means no persistence, not read-only")
	}
}
