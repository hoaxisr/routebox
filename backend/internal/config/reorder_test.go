package config

import (
	"os"
	"path/filepath"
	"testing"
)

// newManagerWithRules loads a Manager over a config whose route.rules are
// identifiable by outbound tag: "A", "B", "C", "D" in that order.
func newManagerWithRules(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := `{
	  "outbounds": [{"type":"direct","tag":"direct"}],
	  "route": {"rules": [
	    {"domain":["a"],"outbound":"A"},
	    {"domain":["b"],"outbound":"B"},
	    {"domain":["c"],"outbound":"C"},
	    {"domain":["d"],"outbound":"D"}
	  ]}
	}`
	if err := os.WriteFile(path, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// ruleOrder reads back the outbound tags of route.rules, in order.
func ruleOrder(t *testing.T, m *Manager) string {
	t.Helper()
	out := ""
	for _, r := range m.ListRules() {
		tag, _ := r["outbound"].(string)
		out += tag
	}
	return out
}

// `to` is the index the moved rule ENDS UP at. The panel drags a row onto a
// position and expects it to land there; the previous contract treated `to` as
// an index in the pre-removal array and decremented it for downward moves,
// which made "drag one slot down" a silent no-op and made the LAST position
// unreachable by any input at all — while both the panel's optimistic update
// and its drop indicator meant the plain reading. They disagreed on every
// downward drag, so the saved order differed from the one on screen and every
// later index-addressed edit then hit the wrong rule.
func TestReorderRulesUsesDestinationIndex(t *testing.T) {
	cases := []struct {
		name     string
		from, to int
		want     string
	}{
		{"down one is a swap, not a no-op", 0, 1, "BACD"},
		{"down two", 0, 2, "BCAD"},
		{"down to the last position", 0, 3, "BCDA"},
		{"up one", 1, 0, "BACD"},
		{"up from the end to the front", 3, 0, "DABC"},
		{"middle down one", 1, 2, "ACBD"},
		{"same index is a no-op", 2, 2, "ABCD"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newManagerWithRules(t)
			if err := m.ReorderRules(tc.from, tc.to); err != nil {
				t.Fatalf("ReorderRules(%d, %d): %v", tc.from, tc.to, err)
			}
			if got := ruleOrder(t, m); got != tc.want {
				t.Fatalf("ReorderRules(%d, %d) = %s, want %s", tc.from, tc.to, got, tc.want)
			}
		})
	}
}

func TestReorderRulesRejectsOutOfRange(t *testing.T) {
	for _, tc := range []struct{ from, to int }{{-1, 0}, {0, -1}, {4, 0}, {0, 4}} {
		m := newManagerWithRules(t)
		if err := m.ReorderRules(tc.from, tc.to); err == nil {
			t.Fatalf("ReorderRules(%d, %d) = nil, want an out-of-range error", tc.from, tc.to)
		}
		if got := ruleOrder(t, m); got != "ABCD" {
			t.Fatalf("a rejected reorder changed the order: %s", got)
		}
	}
}
