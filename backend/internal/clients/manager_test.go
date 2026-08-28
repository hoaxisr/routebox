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

// A dual-stack inbound reports an IPv4 client as "::ffff:x". Entries written
// before #71 carry that form, which is a second entry for a device that already
// has one — the named one, usually, since the operator named it before the
// duplicate appeared.
func TestLoad_FoldsIPv4MappedDuplicates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clients.toml")
	body := `[[clients]]
ip = "192.168.1.14"
name = "laptop"
note = "mine"
first_seen = 1000
last_seen = 2000

[[clients]]
ip = "::ffff:192.168.1.14"
first_seen = 1500
last_seen = 3000
`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	m := New(path)
	if err := m.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := m.Get("::ffff:192.168.1.14"); ok {
		t.Fatal("the mapped form is still its own entry")
	}
	e, ok := m.Get("192.168.1.14")
	if !ok {
		t.Fatal("the canonical entry is gone")
	}
	if e.Name != "laptop" || e.Note != "mine" {
		t.Fatalf("the name the operator typed was lost: %+v", e)
	}
	if e.FirstSeen != 1000 || e.LastSeen != 3000 {
		t.Fatalf("merged window = %d..%d, want 1000..3000", e.FirstSeen, e.LastSeen)
	}
}

// The fold merges FIELDS, not entries. Taking one half wholesale lost the other's
// note whenever both were named, and which half that was came down to file order.
func TestLoad_FoldKeepsEveryTypedField(t *testing.T) {
	write := func(body string) *Manager {
		t.Helper()
		path := filepath.Join(t.TempDir(), "clients.toml")
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		m := New(path)
		if err := m.Load(); err != nil {
			t.Fatalf("Load: %v", err)
		}
		return m
	}

	t.Run("a note on the half with no name survives", func(t *testing.T) {
		m := write(`[[clients]]
ip = "192.168.1.14"
note = "behind the TV"

[[clients]]
ip = "::ffff:192.168.1.14"
name = "shelf"
`)
		e, _ := m.Get("192.168.1.14")
		if e.Name != "shelf" || e.Note != "behind the TV" {
			t.Fatalf("merged %+v, want both the name and the note", e)
		}
	})

	t.Run("both halves named: neither order loses the note", func(t *testing.T) {
		for _, body := range []string{
			`[[clients]]
ip = "192.168.1.14"
name = "plain"

[[clients]]
ip = "::ffff:192.168.1.14"
name = "mapped"
note = "the only note here"
`,
			`[[clients]]
ip = "::ffff:192.168.1.14"
name = "mapped"
note = "the only note here"

[[clients]]
ip = "192.168.1.14"
name = "plain"
`,
		} {
			e, _ := write(body).Get("192.168.1.14")
			if e.Note != "the only note here" {
				t.Fatalf("note lost for this file order: %+v", e)
			}
			if e.Name == "" {
				t.Fatalf("name lost: %+v", e)
			}
		}
	})

	t.Run("a lone mapped entry is rewritten and marked for saving", func(t *testing.T) {
		m := write(`[[clients]]
ip = "::ffff:10.0.0.9"
name = "solo"
`)
		if _, ok := m.Get("::ffff:10.0.0.9"); ok {
			t.Fatal("the mapped form is still its own entry")
		}
		if !m.dirty {
			t.Fatal("the file still spells it the old way but nothing will rewrite it")
		}
	})
}

// SetName has to key by the same canonical form Observe does, or a stale panel
// tab naming through the mapped address recreates the duplicate.
func TestSetName_CanonicalisesTheKey(t *testing.T) {
	m := New("")
	m.Observe("192.168.1.14", time.Unix(1000, 0))
	if err := m.SetName("::ffff:192.168.1.14", "laptop", "mine"); err != nil {
		t.Fatalf("SetName: %v", err)
	}
	if _, ok := m.Get("::ffff:192.168.1.14"); ok {
		t.Fatal("naming through the mapped form created a second entry")
	}
	e, ok := m.Get("192.168.1.14")
	if !ok || e.Name != "laptop" || e.Note != "mine" {
		t.Fatalf("the name did not land on the canonical entry: %+v (found=%v)", e, ok)
	}
}
