package users

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"routebox/backend/internal/util"
)

func unwritableUsersPath(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}
	dir := filepath.Join(t.TempDir(), "etc")
	if err := os.Mkdir(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })
	return filepath.Join(dir, "users.toml")
}

// The registry is where a panel user's token lives. A raw 500 on an unwritable
// partition told the operator nothing; the 409 names the file to fix.
func TestPutReportsReadOnlyWithThePath(t *testing.T) {
	path := unwritableUsersPath(t)
	m := NewManager(path)

	err := m.Put(&PanelUser{ID: "u1", Name: "alice", Enabled: true})
	if !errors.Is(err, util.ErrReadOnly) {
		t.Fatalf("Put error = %v, want util.ErrReadOnly", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error %q must name %q", err, path)
	}
	if !m.IsReadOnly() {
		t.Fatal("the store must report the state the badge shows")
	}
	if m.GetPath() != path {
		t.Fatalf("GetPath() = %q, want %q", m.GetPath(), path)
	}
}

func TestWritableUsersStoreIsNotReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.toml")
	m := NewManager(path)
	if err := m.Put(&PanelUser{ID: "u1", Name: "alice"}); err != nil {
		t.Fatalf("Put = %v, want nil", err)
	}
	if m.IsReadOnly() {
		t.Fatal("a save that went through must leave the store writable")
	}
}

func TestPathlessUsersStoreIsNeverReadOnly(t *testing.T) {
	if NewManager("").IsReadOnly() {
		t.Fatal("no path means no persistence, not read-only")
	}
}
