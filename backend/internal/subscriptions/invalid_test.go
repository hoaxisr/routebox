package subscriptions

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"routebox/backend/internal/util"
)

// The store refuses for two very different reasons, and the API has to tell
// them apart: what the caller sent (a 400) versus what the disk did (never a
// 400 — the request was fine). Only the first kind carries ErrInvalid.
func TestCallerFaultsCarryErrInvalid(t *testing.T) {
	m := NewManager(filepath.Join(t.TempDir(), "subscriptions.toml"))
	if _, err := m.Add("Home", "https://example.com/sub", 12); err != nil {
		t.Fatal(err)
	}

	for name, err := range map[string]error{
		"empty name":      mustAddErr(t, m, ""),
		"no alphanumeric": mustAddErr(t, m, "···"),
		"duplicate":       mustAddErr(t, m, "Home"),
		"update unknown":  m.Update("nope", "https://example.com", 1),
		"delete unknown":  m.Delete("nope"),
	} {
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v must carry ErrInvalid, or the API cannot answer 400", name, err)
		}
		if errors.Is(err, util.ErrReadOnly) {
			t.Errorf("%s: %v must not look like a write refusal", name, err)
		}
	}
}

func mustAddErr(t *testing.T, m *Manager, name string) error {
	t.Helper()
	_, err := m.Add(name, "https://example.com/sub", 12)
	if err == nil {
		t.Fatalf("Add(%q) unexpectedly succeeded", name)
	}
	return err
}

// A file RouteBox cannot write is not the caller's fault, and must not be
// dressed up as one.
func TestAWriteRefusalIsNotACallerFault(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })

	m := NewManager(filepath.Join(dir, "subscriptions.toml"))
	_, err := m.Add("Home", "https://example.com/sub", 12)
	if !errors.Is(err, util.ErrReadOnly) {
		t.Fatalf("err = %v, want util.ErrReadOnly", err)
	}
	if errors.Is(err, ErrInvalid) {
		t.Fatal("a refused write must not be reported as a bad request")
	}
}

// The messages go to the user as written; wrapping must not prepend a sentinel's
// own words to them.
func TestErrInvalidKeepsTheMessageItWasBuiltWith(t *testing.T) {
	m := NewManager(filepath.Join(t.TempDir(), "subscriptions.toml"))
	if _, err := m.Add("", "", 0); err == nil || err.Error() != "name is required" {
		t.Fatalf("Add(\"\") = %v, want the bare message", err)
	}
}
