package clients

import (
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
