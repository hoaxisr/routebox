package users

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreEmptyPathNoOp(t *testing.T) {
	m := NewManager("")
	if err := m.Load(); err != nil {
		t.Fatalf("Load on empty path: %v", err)
	}
	if len(m.List()) != 0 {
		t.Fatalf("expected empty list")
	}
}

func TestStoreLoadMissingFileNoError(t *testing.T) {
	m := NewManager(filepath.Join(t.TempDir(), "users.toml"))
	if err := m.Load(); err != nil {
		t.Fatalf("Load of missing file must be nil error, got %v", err)
	}
}

func TestStoreRoundTripAndPerm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "users.toml")
	m := NewManager(path)

	u := &PanelUser{
		ID: "abc123", Name: "alice", Enabled: true,
		Bindings: []Binding{{InboundTag: "vless-in", Credential: "u-1", Protocol: "vless", Name: "alice", Flow: "xtls-rprx-vision"}},
	}
	if err := m.Put(u); err != nil {
		t.Fatalf("Put: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("perm = %o, want 0600", perm)
	}

	m2 := NewManager(path)
	if err := m2.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	got, ok := m2.Get("abc123")
	if !ok {
		t.Fatalf("user not found after reload")
	}
	if got.Name != "alice" || len(got.Bindings) != 1 || got.Bindings[0].Credential != "u-1" {
		t.Fatalf("unexpected reload value: %#v", got)
	}
}

func TestStoreDeleteAndListSorted(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(filepath.Join(dir, "users.toml"))
	_ = m.Put(&PanelUser{ID: "x1", Name: "B"})
	_ = m.Put(&PanelUser{ID: "x2", Name: "A"})
	if got := m.List(); len(got) != 2 || got[0].Name != "A" {
		t.Fatalf("List must be sorted by name: %#v", got)
	}
	if err := m.Delete("x1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := m.Get("x1"); ok {
		t.Fatalf("expected user gone")
	}
}

// MUST-FIX 7 (graft from B/C): Get/List must deep-copy the Bindings slice so a
// caller cannot mutate the registry's backing array through the returned value.
func TestStoreGetCopyIsolation(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(filepath.Join(dir, "users.toml"))
	_ = m.Put(&PanelUser{ID: "c1", Name: "Orig", Bindings: []Binding{{InboundTag: "v", Credential: "u"}}})

	got, _ := m.Get("c1")
	got.Name = "Mutated"
	got.Bindings[0].Credential = "hacked"

	again, _ := m.Get("c1")
	if again.Name != "Orig" || again.Bindings[0].Credential != "u" {
		t.Fatalf("Get must return a deep copy; store was mutated: %#v", again)
	}

	// Same guarantee for List.
	lst := m.List()
	lst[0].Bindings[0].Credential = "hacked-too"
	after, _ := m.Get("c1")
	if after.Bindings[0].Credential != "u" {
		t.Fatalf("List must return deep copies; store was mutated: %#v", after)
	}
}
