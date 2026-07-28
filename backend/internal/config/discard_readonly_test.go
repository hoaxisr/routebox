package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Discard is the one editing action that stays available in read-only mode: it
// takes changes away, it does not put any on disk. The draft is dropped from
// memory before the file is touched, so a delete that cannot happen has already
// been made irrelevant — reporting it as a failure told the user their Discard
// did not work when it had.
func TestDiscardDraftSucceedsWhenTheDraftFileCannotBeRemoved(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"log":{"level":"info"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SetDraft(map[string]interface{}{"log": map[string]interface{}{"level": "debug"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("harness: the draft file should exist, got %v", err)
	}
	// The directory goes read-only under RouteBox's feet, the way a remount does.
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0700) })

	if err := m.DiscardDraft(); err != nil {
		t.Fatalf("DiscardDraft = %v, want nil: the draft is already gone from memory", err)
	}
	if m.HasDraft() {
		t.Fatal("the draft must be gone whether or not its file could be removed")
	}
}
