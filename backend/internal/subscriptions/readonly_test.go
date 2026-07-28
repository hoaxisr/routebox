package subscriptions

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"routebox/backend/internal/util"
)

func unwritableSubsPath(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}
	dir := filepath.Join(t.TempDir(), "etc")
	if err := os.Mkdir(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })
	return filepath.Join(dir, "subscriptions.toml")
}

func TestAddReportsReadOnlyWithThePath(t *testing.T) {
	path := unwritableSubsPath(t)
	m := NewManager(path)

	_, err := m.Add("Home VPN", "https://example.com/sub", 24)
	if !errors.Is(err, util.ErrReadOnly) {
		t.Fatalf("Add error = %v, want util.ErrReadOnly", err)
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

func TestWritableSubsStoreIsNotReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subscriptions.toml")
	m := NewManager(path)
	if _, err := m.Add("Home VPN", "https://example.com/sub", 24); err != nil {
		t.Fatalf("Add = %v, want nil", err)
	}
	if m.IsReadOnly() {
		t.Fatal("a save that went through must leave the store writable")
	}
}

func TestPathlessSubsStoreIsNeverReadOnly(t *testing.T) {
	if NewManager("").IsReadOnly() {
		t.Fatal("no path means no persistence, not read-only")
	}
}
