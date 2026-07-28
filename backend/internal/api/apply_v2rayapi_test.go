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

	// The process manager MUST be the cut-off one. process.NewManager() adopts
	// this machine's systemd unit and finds this machine's running amnezia-box,
	// and ApplyConfig calls Reload() (SIGHUP) with Restart() as its fallback when
	// the status says running — so on a host that actually runs amnezia-box this
	// helper reaches the real process. Today a config-path mismatch happens to
	// refuse both calls, but that guard is accidental: it disappears the moment
	// this manager is given a config path.
	h := &Handler{config: m, settings: sm, process: process.NewManagerForTest("", dir)}
	// Force the additivity guard to report "supported" so these tests exercise
	// the sync logic on machines that have no amnezia-box binary installed (the
	// real SupportsV2RayAPI fail-closes to false there). The unsupported branch
	// is covered separately in TestApplyConfig_V2RayAPI_UnsupportedBinary.
	h.v2rayAPISupported = func() bool { return true }
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

// TestApplyConfig_V2RayAPI_UnsupportedBinary proves the additivity guard: when
// the running binary does NOT support with_v2ray_api, apply writes NO v2ray_api
// block even though panel users exist — so an old/forked binary isn't fed a
// config block it would reject.
func TestApplyConfig_V2RayAPI_UnsupportedBinary(t *testing.T) {
	h, path := newApplyHandler(t, "alice", "bob")
	h.v2rayAPISupported = func() bool { return false } // binary lacks with_v2ray_api

	req := httptest.NewRequest(http.MethodPost, "/api/config/apply", nil)
	w := httptest.NewRecorder()
	h.ApplyConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("apply status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if v := readDiskV2RayAPI(t, path); v != nil {
		t.Errorf("unsupported binary: expected no v2ray_api block, got %v", v)
	}
}

// TestApplyConfig_V2RayAPI_UnsupportedBinary_RemovesStaleBlock proves a
// downgrade self-heals: a config that ALREADY has a v2ray_api block has it
// REMOVED on apply when the (now unsupported) binary cannot accept it.
func TestApplyConfig_V2RayAPI_UnsupportedBinary_RemovesStaleBlock(t *testing.T) {
	h, path := newApplyHandler(t, "alice")

	// Seed a stale block via a "supported" apply, then verify it's present.
	h.v2rayAPISupported = func() bool { return true }
	seedReq := httptest.NewRequest(http.MethodPost, "/api/config/apply", nil)
	seedW := httptest.NewRecorder()
	h.ApplyConfig(seedW, seedReq)
	if seedW.Code != http.StatusOK {
		t.Fatalf("setup apply status = %d, want 200; body=%s", seedW.Code, seedW.Body.String())
	}
	if v := readDiskV2RayAPI(t, path); v == nil {
		t.Fatal("setup: expected v2ray_api block after supported apply")
	}

	// Now the binary no longer supports it: apply must remove the block.
	h.v2rayAPISupported = func() bool { return false }
	req := httptest.NewRequest(http.MethodPost, "/api/config/apply", nil)
	w := httptest.NewRecorder()
	h.ApplyConfig(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("apply status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if v := readDiskV2RayAPI(t, path); v != nil {
		t.Errorf("downgrade: expected stale v2ray_api block removed, got %v", v)
	}
}
