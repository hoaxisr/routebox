package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"routebox/backend/internal/config"
	"routebox/backend/internal/process"
	"routebox/backend/internal/settings"
	"routebox/backend/internal/users"
)

// newApplyHandler builds a Handler wired the way main.go wires it for the apply
// path: config + panel-user registry + settings + a (non-running) process
// manager. The temp config has one vless inbound with the named users. Returns
// the handler and the on-disk config path so tests can inspect what apply wrote.
func newApplyHandler(t *testing.T, names ...string) (*Handler, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	var u []interface{}
	for i, n := range names {
		u = append(u, map[string]interface{}{"name": n, "uuid": string(rune('a' + i))})
	}
	cfg := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{"type": "vless", "tag": "vless-in", "users": u},
		},
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	m, err := config.NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	reg := users.NewManager(filepath.Join(dir, "users.toml"))
	if _, err := reg.Reconcile(m.GetActive()); err != nil {
		t.Fatal(err)
	}
	sm, err := settings.NewManager(filepath.Join(dir, "routebox.toml"))
	if err != nil {
		t.Fatal(err)
	}

	h := &Handler{config: m, settings: sm, process: process.NewManager()}
	h.SetUsers(reg)
	return h, path
}

// readDiskV2RayAPI loads the on-disk config and returns the
// experimental.v2ray_api block (or nil if absent).
func readDiskV2RayAPI(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	d, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var disk map[string]interface{}
	if err := json.Unmarshal(d, &disk); err != nil {
		t.Fatal(err)
	}
	exp, _ := disk["experimental"].(map[string]interface{})
	if exp == nil {
		return nil
	}
	v, _ := exp["v2ray_api"].(map[string]interface{})
	return v
}

// TestApplyConfig_SyncsV2RayAPI_WithUsers proves ApplyConfig writes
// experimental.v2ray_api with stats.users mirroring the registry, and that the
// apply still succeeds (the sync is wired at the reconcile point, change-gated,
// non-fatal).
func TestApplyConfig_SyncsV2RayAPI_WithUsers(t *testing.T) {
	h, path := newApplyHandler(t, "alice", "bob")

	req := httptest.NewRequest(http.MethodPost, "/api/config/apply", nil)
	w := httptest.NewRecorder()
	h.ApplyConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("apply status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	v := readDiskV2RayAPI(t, path)
	if v == nil {
		t.Fatal("expected experimental.v2ray_api block after apply with users")
	}
	if listen, _ := v["listen"].(string); listen != "127.0.0.1:8081" {
		t.Errorf("listen = %q, want 127.0.0.1:8081 (settings default)", listen)
	}
	stats, _ := v["stats"].(map[string]interface{})
	got, _ := stats["users"].([]interface{})
	if len(got) != 2 || got[0] != "alice" || got[1] != "bob" {
		t.Errorf("stats.users = %v, want [alice bob]", got)
	}

	// Active config (in-memory) must match disk so the apply's reload re-reads it.
	active := readDiskV2RayAPI(t, path)
	if active == nil {
		t.Error("active/disk diverged: no v2ray_api on disk")
	}
}

// TestApplyConfig_SyncsV2RayAPI_NoUsers proves router mode / empty registry
// leaves no experimental.v2ray_api block, while the apply still succeeds.
func TestApplyConfig_SyncsV2RayAPI_NoUsers(t *testing.T) {
	h, path := newApplyHandler(t) // no users

	req := httptest.NewRequest(http.MethodPost, "/api/config/apply", nil)
	w := httptest.NewRecorder()
	h.ApplyConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("apply status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	if v := readDiskV2RayAPI(t, path); v != nil {
		t.Errorf("no users: expected no v2ray_api block, got %v", v)
	}
}
