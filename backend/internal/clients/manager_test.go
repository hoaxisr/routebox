package clients

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestObserve_NewIPCreatesEntry(t *testing.T) {
	m := New("")
	now := time.Unix(1000, 0)
	m.Observe("192.168.1.14", now)

	e, ok := m.Get("192.168.1.14")
	if !ok {
		t.Fatal("expected entry to exist")
	}
	if e.FirstSeen != now.Unix() {
		t.Errorf("FirstSeen = %d, want %d", e.FirstSeen, now.Unix())
	}
	if e.LastSeen != now.Unix() {
		t.Errorf("LastSeen = %d, want %d", e.LastSeen, now.Unix())
	}
	if e.Name != "" {
		t.Errorf("Name = %q, want empty", e.Name)
	}
}

func TestObserve_RepeatedIPUpdatesLastSeenOnly(t *testing.T) {
	m := New("")
	first := time.Unix(1000, 0)
	later := time.Unix(2000, 0)
	m.Observe("10.0.0.1", first)
	m.Observe("10.0.0.1", later)

	e, _ := m.Get("10.0.0.1")
	if e.FirstSeen != first.Unix() {
		t.Errorf("FirstSeen changed to %d, want %d", e.FirstSeen, first.Unix())
	}
	if e.LastSeen != later.Unix() {
		t.Errorf("LastSeen = %d, want %d", e.LastSeen, later.Unix())
	}
}

func TestSetName_PreservesTimestamps(t *testing.T) {
	m := New("")
	now := time.Unix(1000, 0)
	m.Observe("10.0.0.1", now)
	if err := m.SetName("10.0.0.1", "MacBook", "work"); err != nil {
		t.Fatalf("SetName: %v", err)
	}
	e, _ := m.Get("10.0.0.1")
	if e.Name != "MacBook" {
		t.Errorf("Name = %q, want MacBook", e.Name)
	}
	if e.Note != "work" {
		t.Errorf("Note = %q, want work", e.Note)
	}
	if e.FirstSeen != now.Unix() {
		t.Errorf("FirstSeen overwritten")
	}
}

func TestSetName_AutocreatesIfMissing(t *testing.T) {
	m := New("")
	if err := m.SetName("10.0.0.1", "Server", ""); err != nil {
		t.Fatalf("SetName: %v", err)
	}
	e, ok := m.Get("10.0.0.1")
	if !ok || e.Name != "Server" {
		t.Errorf("expected entry with name Server, got %+v ok=%v", e, ok)
	}
	if e.FirstSeen == 0 || e.LastSeen == 0 {
		t.Errorf("expected timestamps to be set, got %+v", e)
	}
	if e.FirstSeen != e.LastSeen {
		t.Errorf("FirstSeen and LastSeen should match on autocreate, got %d vs %d", e.FirstSeen, e.LastSeen)
	}
}

func TestForget_RemovesEntry(t *testing.T) {
	m := New("")
	m.Observe("10.0.0.1", time.Unix(1000, 0))
	m.Forget("10.0.0.1")
	if _, ok := m.Get("10.0.0.1"); ok {
		t.Errorf("expected entry to be removed")
	}
}

func TestList_SortsByLastSeenDesc(t *testing.T) {
	m := New("")
	m.Observe("a", time.Unix(1000, 0))
	m.Observe("b", time.Unix(2000, 0))
	m.Observe("c", time.Unix(1500, 0))
	got := m.List()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	wantOrder := []string{"b", "c", "a"}
	for i, ip := range wantOrder {
		if got[i].IP != ip {
			t.Errorf("List()[%d].IP = %q, want %q", i, got[i].IP, ip)
		}
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "clients.toml")

	m1 := New(p)
	m1.Observe("10.0.0.1", time.Unix(1000, 0))
	if err := m1.SetName("10.0.0.1", "Mac", "work"); err != nil {
		t.Fatalf("SetName: %v", err)
	}
	m1.Observe("10.0.0.2", time.Unix(2000, 0))
	if err := m1.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	m2 := New(p)
	if err := m2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	e, ok := m2.Get("10.0.0.1")
	if !ok {
		t.Fatal("expected 10.0.0.1 to be loaded")
	}
	if e.Name != "Mac" || e.Note != "work" {
		t.Errorf("Mac entry: got name=%q note=%q, want Mac/work", e.Name, e.Note)
	}
	if e.FirstSeen != 1000 || e.LastSeen != 1000 {
		t.Errorf("Mac entry timestamps: %d/%d, want 1000/1000", e.FirstSeen, e.LastSeen)
	}

	if _, ok := m2.Get("10.0.0.2"); !ok {
		t.Error("expected 10.0.0.2 to be loaded")
	}
}

func TestSave_NoOpWhenNotDirty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "clients.toml")
	m := New(p)
	// Observe -> dirty -> save -> file exists -> dirty=false
	m.Observe("1.1.1.1", time.Unix(0, 0))
	if err := m.Save(); err != nil {
		t.Fatalf("first save: %v", err)
	}
	info1, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// Save again with no changes — file should not be rewritten.
	if err := m.Save(); err != nil {
		t.Fatalf("second save: %v", err)
	}
	info2, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info1.ModTime() != info2.ModTime() {
		t.Errorf("expected no rewrite; got mtime change: %v -> %v", info1.ModTime(), info2.ModTime())
	}
}
