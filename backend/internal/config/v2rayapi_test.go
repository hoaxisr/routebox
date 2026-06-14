package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeV2Cfg(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBuildV2RayAPIBlock_SortedDeduped(t *testing.T) {
	got := BuildV2RayAPIBlock("127.0.0.1:8081", []string{"bob", "alice", "bob", "", "carol"})
	want := map[string]interface{}{
		"listen": "127.0.0.1:8081",
		"stats": map[string]interface{}{
			"enabled": true,
			"users":   []interface{}{"alice", "bob", "carol"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v\nwant %#v", got, want)
	}
}

func TestBuildV2RayAPIBlock_EmptyIsNil(t *testing.T) {
	if got := BuildV2RayAPIBlock("127.0.0.1:8081", nil); got != nil {
		t.Errorf("nil names → %#v, want nil", got)
	}
	if got := BuildV2RayAPIBlock("127.0.0.1:8081", []string{"", "  "}); got != nil {
		t.Errorf("blank-only names → %#v, want nil", got)
	}
}

func TestSyncV2RayAPI_WriteIdempotentRemove(t *testing.T) {
	p := writeV2Cfg(t, `{"experimental":{"clash_api":{"external_controller":"127.0.0.1:9090"}}}`)
	m, err := NewManager(p)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Write: block appears, clash_api untouched.
	changed, err := m.SyncV2RayAPI("127.0.0.1:8081", []string{"alice", "bob"})
	if err != nil || !changed {
		t.Fatalf("first sync changed=%v err=%v, want true/nil", changed, err)
	}
	exp := m.GetActive()["experimental"].(map[string]interface{})
	if _, ok := exp["clash_api"]; !ok {
		t.Error("clash_api was clobbered")
	}
	stats := exp["v2ray_api"].(map[string]interface{})["stats"].(map[string]interface{})
	if got := stats["users"].([]interface{}); len(got) != 2 || got[0] != "alice" || got[1] != "bob" {
		t.Errorf("users = %#v, want [alice bob]", got)
	}

	// Re-read from disk to prove persistence.
	d, _ := os.ReadFile(p)
	if !reflect.DeepEqual(mustJSON(t, d)["experimental"].(map[string]interface{})["v2ray_api"],
		map[string]interface{}{
			"listen": "127.0.0.1:8081",
			"stats":  map[string]interface{}{"enabled": true, "users": []interface{}{"alice", "bob"}},
		}) {
		t.Error("disk block does not match")
	}

	// Idempotent: same names → no change.
	if changed, _ := m.SyncV2RayAPI("127.0.0.1:8081", []string{"bob", "alice"}); changed {
		t.Error("re-sync with same names reported changed=true, want false")
	}

	// Empty → block removed, clash_api survives.
	changed, err = m.SyncV2RayAPI("127.0.0.1:8081", nil)
	if err != nil || !changed {
		t.Fatalf("remove sync changed=%v err=%v, want true/nil", changed, err)
	}
	exp = m.GetActive()["experimental"].(map[string]interface{})
	if _, ok := exp["v2ray_api"]; ok {
		t.Error("v2ray_api not removed for empty names")
	}
	if _, ok := exp["clash_api"]; !ok {
		t.Error("clash_api removed by mistake")
	}
}

func TestSyncV2RayAPI_EmptyNoBlockNoOp(t *testing.T) {
	p := writeV2Cfg(t, `{"inbounds":[]}`)
	m, _ := NewManager(p)
	if changed, _ := m.SyncV2RayAPI("127.0.0.1:8081", nil); changed {
		t.Error("empty names + no existing block should be a no-op")
	}
}

func mustJSON(t *testing.T, b []byte) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}
