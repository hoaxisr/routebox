package api

import (
	"path/filepath"
	"testing"

	"routebox/backend/internal/traffic"
	"routebox/backend/internal/users"
)

func openAPITrafficStore(t *testing.T) *traffic.Store {
	t.Helper()
	s, err := traffic.OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPanelUserNames_DedupesAndSkipsBlank(t *testing.T) {
	mgr := users.NewManager("")
	_ = mgr.Put(&users.PanelUser{ID: "1", Name: "alice"})
	_ = mgr.Put(&users.PanelUser{ID: "2", Name: "bob"})
	_ = mgr.Put(&users.PanelUser{ID: "3", Name: ""}) // blank skipped

	got := panelUserNames(mgr)
	if len(got) != 2 {
		t.Fatalf("names = %v, want 2 (alice,bob)", got)
	}
	want := map[string]bool{"alice": true, "bob": true}
	for _, n := range got {
		if !want[n] {
			t.Errorf("unexpected name %q", n)
		}
	}
}

func TestPanelUserNames_NilManagerEmpty(t *testing.T) {
	if got := panelUserNames(nil); len(got) != 0 {
		t.Errorf("nil mgr → %v, want empty", got)
	}
}
