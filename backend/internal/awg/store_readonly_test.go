package awg

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"routebox/backend/internal/util"
)

// peers.toml holds every client's private key and lives in its own directory
// (/etc/amnezia/amneziawg), which can be read-only while the sing-box config is
// not. Adding a peer then failed with a raw 500 naming nothing.
func TestStoreSaveReportsReadOnlyWithThePath(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}
	dir := filepath.Join(t.TempDir(), "amneziawg")
	if err := os.Mkdir(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })
	path := filepath.Join(dir, "peers.toml")

	s := NewStore(path)
	err := s.Put(Peer{PublicKey: "pk", Address: "10.10.0.2/32", Name: "alice"})
	if !errors.Is(err, util.ErrReadOnly) {
		t.Fatalf("Put error = %v, want util.ErrReadOnly so the API can answer 409", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error %q must name %q", err, path)
	}
	if !s.IsReadOnly() {
		t.Fatal("the store must report the state the badge shows")
	}
	if s.GetPath() != path {
		t.Fatalf("GetPath() = %q, want %q", s.GetPath(), path)
	}
}

func TestWritableStoreIsNotReadOnly(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "peers.toml"))
	if err := s.Put(Peer{PublicKey: "pk", Address: "10.10.0.2/32"}); err != nil {
		t.Fatalf("Put = %v, want nil", err)
	}
	if s.IsReadOnly() {
		t.Fatal("a save that went through must leave the store writable")
	}
}

func TestPathlessStoreIsNeverReadOnly(t *testing.T) {
	if NewStore("").IsReadOnly() {
		t.Fatal("no path means no persistence, not read-only")
	}
}
