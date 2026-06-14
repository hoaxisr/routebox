package api

import (
	"testing"

	"routebox/backend/internal/users"
)

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

func TestUserTrafficNames_NameUnionBindings(t *testing.T) {
	u := users.PanelUser{
		Name: "alice",
		Bindings: []users.Binding{
			{Name: "alice"},       // dup of Name
			{Name: "alice-phone"}, // extra
			{Name: ""},            // skipped
		},
	}
	got := userTrafficNames(u)
	if len(got) != 2 {
		t.Fatalf("names = %v, want [alice alice-phone]", got)
	}
	seen := map[string]bool{}
	for _, n := range got {
		seen[n] = true
	}
	if !seen["alice"] || !seen["alice-phone"] {
		t.Errorf("got %v, want alice + alice-phone", got)
	}
}
