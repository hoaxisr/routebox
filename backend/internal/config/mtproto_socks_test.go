package config

import (
	"os"
	"reflect"
	"testing"
)

func TestBuildMtprotoSocksInbound(t *testing.T) {
	got := BuildMtprotoSocksInbound(1080)
	want := map[string]interface{}{
		"type":        "socks",
		"tag":         "mtproto-socks",
		"listen":      "127.0.0.1",
		"listen_port": 1080,
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestBuildMtprotoSocksRule(t *testing.T) {
	if got := buildMtprotoSocksRule(""); got != nil {
		t.Fatalf("empty outbound -> %#v, want nil", got)
	}

	got := buildMtprotoSocksRule("warp")
	want := map[string]interface{}{
		"inbound":  []interface{}{"mtproto-socks"},
		"outbound": "warp",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestManagedMtprotoSocksRule(t *testing.T) {
	cases := []struct {
		name string
		rule map[string]interface{}
		want bool
	}{
		{
			"managed",
			map[string]interface{}{"inbound": []interface{}{"mtproto-socks"}, "outbound": "warp"},
			true,
		},
		{
			// An operator who added their own condition owns the rule now.
			"NOT managed: an extra match key",
			map[string]interface{}{"inbound": []interface{}{"mtproto-socks"}, "outbound": "warp", "domain": []interface{}{"x.com"}},
			false,
		},
		{
			"NOT managed: another inbound rides along",
			map[string]interface{}{"inbound": []interface{}{"mtproto-socks", "vless-in"}, "outbound": "warp"},
			false,
		},
		{
			"NOT managed: a different inbound",
			map[string]interface{}{"inbound": []interface{}{"vless-in"}, "outbound": "warp"},
			false,
		},
		{
			"NOT managed: no outbound",
			map[string]interface{}{"inbound": []interface{}{"mtproto-socks"}, "action": "reject"},
			false,
		},
		{
			"NOT managed: inbound is not a list",
			map[string]interface{}{"inbound": "mtproto-socks", "outbound": "warp"},
			false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := managedMtprotoSocksRule(tc.rule); got != tc.want {
				t.Fatalf("managedMtprotoSocksRule(%#v) = %v, want %v", tc.rule, got, tc.want)
			}
		})
	}
}

// inboundsOf returns the config's inbounds as []interface{} (nil-safe).
func inboundsOf(cfg map[string]interface{}) []interface{} {
	arr, _ := cfg["inbounds"].([]interface{})

	return arr
}

func findInbound(cfg map[string]interface{}, tag string) map[string]interface{} {
	for _, ib := range inboundsOf(cfg) {
		if obj, ok := ib.(map[string]interface{}); ok {
			if t, _ := obj["tag"].(string); t == tag {
				return obj
			}
		}
	}

	return nil
}

func TestSyncMtprotoSocksActive_InsertUpdateRemove(t *testing.T) {
	// A user inbound and a user rule that must both survive untouched.
	p := writeV2Cfg(t, `{
		"inbounds":[{"type":"vless","tag":"vless-in","listen":"::","listen_port":443}],
		"outbounds":[{"type":"direct","tag":"direct"},{"type":"wireguard","tag":"warp"}],
		"route":{"rules":[{"domain":["x.com"],"action":"reject"}]}
	}`)

	m, err := NewManager(p)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// --- insert ---
	changed, err := m.SyncMtprotoSocksActive(1080, "warp")
	if err != nil || !changed {
		t.Fatalf("insert changed=%v err=%v, want true/nil", changed, err)
	}

	cfg := m.GetActive()

	ib := findInbound(cfg, "mtproto-socks")
	if ib == nil {
		t.Fatal("managed inbound was not written")
	}

	if ib["listen"] != "127.0.0.1" {
		t.Errorf("listen = %v, want 127.0.0.1 — this must never be reachable off-host", ib["listen"])
	}

	if port, _ := ib["listen_port"].(float64); port != 1080 {
		t.Errorf("listen_port = %v, want 1080", ib["listen_port"])
	}

	if findInbound(cfg, "vless-in") == nil {
		t.Error("the user's inbound was dropped")
	}

	rules := rulesOf(t, cfg)
	if len(rules) != 2 {
		t.Fatalf("want 2 rules (managed + user), got %d: %#v", len(rules), rules)
	}

	if !managedMtprotoSocksRule(rules[0].(map[string]interface{})) {
		t.Fatalf("the managed rule must be prepended at index 0, got %#v", rules[0])
	}

	if dm, _ := rules[1].(map[string]interface{}); dm["domain"] == nil {
		t.Fatalf("the user's rule must survive at index 1, got %#v", rules[1])
	}

	// Persisted, not just held in memory.
	d, _ := os.ReadFile(p)
	if findInbound(mustJSON(t, d), "mtproto-socks") == nil {
		t.Fatal("the managed inbound never reached disk")
	}

	// --- idempotent ---
	// This is the one that matters operationally: a save with nothing altered
	// must not reload sing-box and drop every live tunnel.
	if changed, err := m.SyncMtprotoSocksActive(1080, "warp"); changed || err != nil {
		t.Fatalf("re-sync changed=%v err=%v, want false/nil", changed, err)
	}

	// --- update the outbound ---
	changed, err = m.SyncMtprotoSocksActive(1080, "direct")
	if err != nil || !changed {
		t.Fatalf("update changed=%v err=%v, want true/nil", changed, err)
	}

	rules = rulesOf(t, m.GetActive())
	if len(rules) != 2 {
		t.Fatalf("update must keep exactly 2 rules, got %d", len(rules))
	}

	if got := rules[0].(map[string]interface{})["outbound"]; got != "direct" {
		t.Errorf("outbound = %v, want direct", got)
	}

	// --- update the port ---
	changed, err = m.SyncMtprotoSocksActive(1081, "direct")
	if err != nil || !changed {
		t.Fatalf("port change changed=%v err=%v, want true/nil", changed, err)
	}

	if port, _ := findInbound(m.GetActive(), "mtproto-socks")["listen_port"].(float64); port != 1081 {
		t.Errorf("listen_port = %v, want 1081", port)
	}

	// --- remove ---
	changed, err = m.SyncMtprotoSocksActive(1081, "")
	if err != nil || !changed {
		t.Fatalf("remove changed=%v err=%v, want true/nil", changed, err)
	}

	cfg = m.GetActive()

	if findInbound(cfg, "mtproto-socks") != nil {
		t.Error("the managed inbound survived a switch back to direct")
	}

	rules = rulesOf(t, cfg)
	if len(rules) != 1 {
		t.Fatalf("want only the user rule left, got %#v", rules)
	}

	if findInbound(cfg, "vless-in") == nil {
		t.Error("the user's inbound was dropped on removal")
	}

	// Removing when already absent is a no-op, not a spurious reload.
	if changed, err := m.SyncMtprotoSocksActive(1081, ""); changed || err != nil {
		t.Fatalf("re-remove changed=%v err=%v, want false/nil", changed, err)
	}
}

// Two inbounds on one port crash amnezia-box at the next reload, which would
// strand the operator with a dead VPN over a Telegram setting. Refuse instead,
// and leave the config exactly as it was.
func TestSyncMtprotoSocksActive_RefusesAPortCollision(t *testing.T) {
	p := writeV2Cfg(t, `{
		"inbounds":[{"type":"socks","tag":"local-socks","listen":"127.0.0.1","listen_port":1080}],
		"outbounds":[{"type":"direct","tag":"direct"}]
	}`)

	m, err := NewManager(p)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	changed, err := m.SyncMtprotoSocksActive(1080, "direct")
	if err == nil {
		t.Fatal("want an error for a colliding port, got none")
	}

	if changed {
		t.Error("a refused sync must not report a change")
	}

	if findInbound(m.GetActive(), "mtproto-socks") != nil {
		t.Error("a refused sync wrote the inbound anyway")
	}
}

func TestSyncMtprotoSocksActive_RejectsAnImpossiblePort(t *testing.T) {
	p := writeV2Cfg(t, `{"outbounds":[{"type":"direct","tag":"direct"}]}`)

	m, err := NewManager(p)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	for _, port := range []int{0, -1, 70000} {
		if _, err := m.SyncMtprotoSocksActive(port, "direct"); err == nil {
			t.Errorf("port %d: want an error, got none", port)
		}
	}

	// ...but going back to direct must work regardless of the port value, or an
	// operator who saved a bad port could never undo it.
	if _, err := m.SyncMtprotoSocksActive(0, ""); err != nil {
		t.Errorf("removing with a zero port: %v", err)
	}
}

// Never write the active config mid-edit: the pending Apply re-renders it.
func TestSyncMtprotoSocksActive_DefersWhileADraftIsPending(t *testing.T) {
	p := writeV2Cfg(t, `{"outbounds":[{"type":"direct","tag":"direct"}]}`)

	m, err := NewManager(p)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if err := m.CreateOutbound(map[string]interface{}{"type": "direct", "tag": "staged"}); err != nil {
		t.Fatalf("CreateOutbound: %v", err)
	}

	if !m.HasDraft() {
		t.Fatal("a draft was expected to be pending")
	}

	changed, err := m.SyncMtprotoSocksActive(1080, "direct")
	if changed || err != nil {
		t.Fatalf("changed=%v err=%v, want false/nil (a silent deferral)", changed, err)
	}

	if findInbound(m.GetActive(), "mtproto-socks") != nil {
		t.Error("active was written while a draft was pending")
	}
}

// A rule an operator wrote against the managed inbound is theirs, and a sync
// must neither remove it nor treat it as its own.
func TestSyncMtprotoSocksActive_LeavesUserAuthoredRulesAlone(t *testing.T) {
	p := writeV2Cfg(t, `{
		"outbounds":[{"type":"direct","tag":"direct"},{"type":"wireguard","tag":"warp"}],
		"route":{"rules":[{"inbound":["mtproto-socks"],"outbound":"warp","domain":["x.com"]}]}
	}`)

	m, err := NewManager(p)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if _, err := m.SyncMtprotoSocksActive(1080, "direct"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	rules := rulesOf(t, m.GetActive())
	if len(rules) != 2 {
		t.Fatalf("want the managed rule plus the user's, got %d: %#v", len(rules), rules)
	}

	user, _ := rules[1].(map[string]interface{})
	if user["domain"] == nil {
		t.Errorf("the user's rule was replaced: %#v", rules[1])
	}
}

func TestListRoutableTags(t *testing.T) {
	p := writeV2Cfg(t, `{
		"outbounds":[{"type":"direct","tag":"direct"},{"type":"selector","tag":"pick"}],
		"endpoints":[{"type":"wireguard","tag":"warp"},{"type":"awg","tag":"awg-server"}]
	}`)

	m, err := NewManager(p)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	got := m.ListRoutableTags()

	want := []RoutableTag{
		{Tag: "direct", Type: "direct", Kind: "outbound"},
		{Tag: "pick", Type: "selector", Kind: "outbound"},
		{Tag: "warp", Type: "wireguard", Kind: "endpoint"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}

	// awg-server is a listener for inbound peers; routing Telegram into it would
	// black-hole the proxy, so it must never be offered.
	for _, r := range got {
		if r.Tag == ManagedAwgServerTag {
			t.Error("the managed AWG server endpoint was offered as an exit")
		}
	}
}
