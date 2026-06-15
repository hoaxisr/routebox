package config

import (
	"os"
	"reflect"
	"testing"
)

func TestBuildRejectRule(t *testing.T) {
	t.Run("empty names -> nil", func(t *testing.T) {
		if got := buildRejectRule(nil); got != nil {
			t.Fatalf("nil names -> %#v, want nil", got)
		}
		if got := buildRejectRule([]string{}); got != nil {
			t.Fatalf("empty names -> %#v, want nil", got)
		}
	})
	t.Run("names -> reject rule shape", func(t *testing.T) {
		got := buildRejectRule([]string{"alice", "bob"})
		want := map[string]interface{}{
			"auth_user": []interface{}{"alice", "bob"},
			"action":    "reject",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v\nwant %#v", got, want)
		}
		if len(got) != 2 {
			t.Fatalf("rule has %d keys, want exactly 2 (auth_user, action)", len(got))
		}
	})
	t.Run("single name", func(t *testing.T) {
		got := buildRejectRule([]string{"solo"})
		want := map[string]interface{}{
			"auth_user": []interface{}{"solo"},
			"action":    "reject",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v\nwant %#v", got, want)
		}
	})
}

func TestManagedRejectRule(t *testing.T) {
	cases := []struct {
		name string
		rule map[string]interface{}
		want bool
	}{
		{
			"managed: auth_user + action reject, no extra keys",
			map[string]interface{}{"auth_user": []interface{}{"a"}, "action": "reject"},
			true,
		},
		{
			"managed: multiple names",
			map[string]interface{}{"auth_user": []interface{}{"a", "b"}, "action": "reject"},
			true,
		},
		{
			"NOT managed: empty auth_user list",
			map[string]interface{}{"auth_user": []interface{}{}, "action": "reject"},
			false,
		},
		{
			"NOT managed: missing auth_user",
			map[string]interface{}{"action": "reject"},
			false,
		},
		{
			"NOT managed: action not reject",
			map[string]interface{}{"auth_user": []interface{}{"a"}, "action": "route"},
			false,
		},
		{
			"NOT managed: missing action",
			map[string]interface{}{"auth_user": []interface{}{"a"}},
			false,
		},
		{
			"NOT managed: extra match key (user-authored auth_user rule)",
			map[string]interface{}{"auth_user": []interface{}{"a"}, "action": "reject", "domain": []interface{}{"x.com"}},
			false,
		},
		{
			"NOT managed: outbound action with auth_user (user rule)",
			map[string]interface{}{"auth_user": []interface{}{"a"}, "outbound": "block"},
			false,
		},
		{
			"NOT managed: empty rule",
			map[string]interface{}{},
			false,
		},
		{
			"NOT managed: nil rule",
			nil,
			false,
		},
		{
			"NOT managed: auth_user wrong type",
			map[string]interface{}{"auth_user": "a", "action": "reject"},
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := managedRejectRule(tc.rule); got != tc.want {
				t.Fatalf("managedRejectRule(%#v) = %v, want %v", tc.rule, got, tc.want)
			}
		})
	}
}

// rulesOf returns route.rules of a config map as []interface{} (nil-safe).
func rulesOf(t *testing.T, cfg map[string]interface{}) []interface{} {
	t.Helper()
	route, _ := cfg["route"].(map[string]interface{})
	r, _ := route["rules"].([]interface{})
	return r
}

func TestSyncRejectRuleActive_InsertPrependUpdateRemove(t *testing.T) {
	// Active has ONE pre-existing user rule that must be preserved at the tail.
	p := writeV2Cfg(t, `{"route":{"rules":[{"domain":["x.com"],"action":"reject"}]}}`)
	m, err := NewManager(p)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Insert: reject rule PREPENDED at index 0, user rule preserved after it.
	changed, err := m.SyncRejectRuleActive([]string{"alice", "bob"})
	if err != nil || !changed {
		t.Fatalf("insert changed=%v err=%v, want true/nil", changed, err)
	}
	rules := rulesOf(t, m.GetActive())
	if len(rules) != 2 {
		t.Fatalf("want 2 rules (reject + user), got %d: %#v", len(rules), rules)
	}
	if !managedRejectRule(rules[0].(map[string]interface{})) {
		t.Fatalf("reject rule must be at index 0, got %#v", rules[0])
	}
	au := rules[0].(map[string]interface{})["auth_user"].([]interface{})
	if len(au) != 2 || au[0] != "alice" || au[1] != "bob" {
		t.Fatalf("auth_user = %#v, want [alice bob]", au)
	}
	if dm, _ := rules[1].(map[string]interface{}); dm["domain"] == nil {
		t.Fatalf("user domain rule must survive at index 1, got %#v", rules[1])
	}

	// Persisted: re-read from disk.
	d, _ := os.ReadFile(p)
	diskRules := rulesOf(t, mustJSON(t, d))
	if len(diskRules) != 2 || !managedRejectRule(diskRules[0].(map[string]interface{})) {
		t.Fatalf("disk does not have managed reject rule at index 0: %#v", diskRules)
	}

	// Idempotent: same names -> no change.
	if changed, _ := m.SyncRejectRuleActive([]string{"alice", "bob"}); changed {
		t.Fatal("re-sync with same names reported changed=true, want false")
	}

	// Update: different names -> changed, still ONE managed rule at index 0.
	changed, err = m.SyncRejectRuleActive([]string{"carol"})
	if err != nil || !changed {
		t.Fatalf("update changed=%v err=%v, want true/nil", changed, err)
	}
	rules = rulesOf(t, m.GetActive())
	if len(rules) != 2 {
		t.Fatalf("update must keep exactly 2 rules, got %d: %#v", len(rules), rules)
	}
	au = rules[0].(map[string]interface{})["auth_user"].([]interface{})
	if len(au) != 1 || au[0] != "carol" {
		t.Fatalf("after update auth_user = %#v, want [carol]", au)
	}

	// Remove: empty names -> managed rule gone, user rule survives.
	changed, err = m.SyncRejectRuleActive(nil)
	if err != nil || !changed {
		t.Fatalf("remove changed=%v err=%v, want true/nil", changed, err)
	}
	rules = rulesOf(t, m.GetActive())
	if len(rules) != 1 {
		t.Fatalf("after remove want 1 user rule, got %d: %#v", len(rules), rules)
	}
	if managedRejectRule(rules[0].(map[string]interface{})) {
		t.Fatal("managed rule was not removed")
	}

	// Remove again (already absent) -> no change.
	if changed, _ := m.SyncRejectRuleActive(nil); changed {
		t.Fatal("remove on absent managed rule reported changed=true, want false")
	}
}

func TestSyncRejectRuleActive_EmptyNoRuleNoOp(t *testing.T) {
	p := writeV2Cfg(t, `{"inbounds":[]}`)
	m, _ := NewManager(p)
	if changed, _ := m.SyncRejectRuleActive(nil); changed {
		t.Fatal("empty names + no existing rule must be a no-op (changed=false)")
	}
}

func TestSyncRejectRuleActive_EmptyRulesArrayEmptyNamesNoOp(t *testing.T) {
	// A non-nil empty route.rules array + empty names must be a true no-op: there
	// is no managed rule to remove, so no rewrite/reload (which would drop live
	// VPN connections). Guards the empty-vs-empty DeepEqual normalization.
	p := writeV2Cfg(t, `{"route":{"rules":[]}}`)
	m, _ := NewManager(p)
	if changed, err := m.SyncRejectRuleActive(nil); changed || err != nil {
		t.Fatalf("empty rules array + empty names must no-op: changed=%v err=%v, want false/nil", changed, err)
	}
}

func TestSyncRejectRuleActive_CollapsesDuplicateManagedRules(t *testing.T) {
	// A corrupted/double-written active config with TWO managed reject rules must
	// collapse to exactly ONE on sync (self-healing), and be idempotent after.
	p := writeV2Cfg(t, `{"route":{"rules":[`+
		`{"auth_user":["alice"],"action":"reject"},`+
		`{"auth_user":["alice"],"action":"reject"},`+
		`{"domain":["x.com"],"action":"reject"}]}}`)
	m, _ := NewManager(p)
	changed, err := m.SyncRejectRuleActive([]string{"alice"})
	if err != nil || !changed {
		t.Fatalf("collapse changed=%v err=%v, want true/nil", changed, err)
	}
	rules := rulesOf(t, m.GetActive())
	if len(rules) != 2 {
		t.Fatalf("want 2 rules (one managed + one user), got %d: %#v", len(rules), rules)
	}
	if !managedRejectRule(rules[0].(map[string]interface{})) {
		t.Fatalf("collapsed managed rule must be at index 0, got %#v", rules[0])
	}
	if managedRejectRule(rules[1].(map[string]interface{})) {
		t.Fatalf("only ONE managed rule must remain, index 1 is also managed: %#v", rules[1])
	}
	// Idempotent after self-heal.
	if changed, _ := m.SyncRejectRuleActive([]string{"alice"}); changed {
		t.Fatal("re-sync after collapse reported changed=true, want false")
	}
}

func TestSyncRejectRuleActive_PrunesEmptyRouteOnRemove(t *testing.T) {
	// route.rules contains ONLY the managed rule; removing it must drop route.rules
	// AND the now-empty route key (twin of SyncV2RayAPI's experimental cleanup).
	p := writeV2Cfg(t, `{"inbounds":[]}`)
	m, _ := NewManager(p)
	if _, err := m.SyncRejectRuleActive([]string{"alice"}); err != nil {
		t.Fatal(err)
	}
	if changed, err := m.SyncRejectRuleActive(nil); err != nil || !changed {
		t.Fatalf("remove changed=%v err=%v, want true/nil", changed, err)
	}
	cfg := m.GetActive()
	if _, ok := cfg["route"]; ok {
		t.Fatalf("empty route must be pruned, got route=%#v", cfg["route"])
	}
}

func TestSyncRejectRuleActive_PreservesSiblingRouteKeysOnRemove(t *testing.T) {
	// route has a sibling key (final) besides rules; removing the only managed
	// rule must drop route.rules but KEEP route (it is not empty).
	p := writeV2Cfg(t, `{"route":{"final":"direct"}}`)
	m, _ := NewManager(p)
	if _, err := m.SyncRejectRuleActive([]string{"alice"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SyncRejectRuleActive(nil); err != nil {
		t.Fatal(err)
	}
	cfg := m.GetActive()
	route, ok := cfg["route"].(map[string]interface{})
	if !ok {
		t.Fatalf("route with sibling key must survive, got %#v", cfg["route"])
	}
	if route["final"] != "direct" {
		t.Fatalf("sibling route.final must survive, got %#v", route)
	}
	if _, ok := route["rules"]; ok {
		t.Fatalf("empty route.rules must be pruned, got %#v", route["rules"])
	}
}

func TestSyncRejectRuleActive_NoRouteSectionInsert(t *testing.T) {
	// No route section at all -> insert creates route + rules.
	p := writeV2Cfg(t, `{"inbounds":[]}`)
	m, _ := NewManager(p)
	changed, err := m.SyncRejectRuleActive([]string{"a"})
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v, want true/nil", changed, err)
	}
	rules := rulesOf(t, m.GetActive())
	if len(rules) != 1 || !managedRejectRule(rules[0].(map[string]interface{})) {
		t.Fatalf("expected one managed rule, got %#v", rules)
	}
}

func TestSyncRejectRuleActive_DeferWhenDraft(t *testing.T) {
	p := writeV2Cfg(t, `{"route":{"rules":[]}}`)
	m, _ := NewManager(p)
	if err := m.EnsureDraft(); err != nil { // hasDraft = true
		t.Fatal(err)
	}
	changed, err := m.SyncRejectRuleActive([]string{"alice"})
	if changed || err != nil {
		t.Fatalf("with pending draft must defer: changed=%v err=%v, want false/nil", changed, err)
	}
	// Active must be UNTOUCHED while a draft is pending.
	if r := rulesOf(t, m.GetActive()); len(r) != 0 {
		t.Fatalf("active rules must be untouched while draft pending, got %#v", r)
	}
}

func TestSyncRejectRuleActive_ReadOnlyNoOp(t *testing.T) {
	p := writeV2Cfg(t, `{"route":{"rules":[]}}`)
	m, err := NewReadOnlyManager(p)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := m.SyncRejectRuleActive([]string{"alice"}); changed || err != nil {
		t.Fatalf("read-only must no-op: changed=%v err=%v, want false/nil", changed, err)
	}
}
