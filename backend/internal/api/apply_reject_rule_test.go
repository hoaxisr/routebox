package api

import (
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

func TestApplyConfig_SyncsRejectRuleAfterReconcile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := `{
	  "inbounds":[{"type":"vless","tag":"in","listen_port":443,
	    "users":[{"name":"alice","uuid":"u-alice","flow":"xtls-rprx-vision"}]}],
	  "outbounds":[{"type":"direct","tag":"direct"}],
	  "route":{"rules":[]}
	}`
	if err := os.WriteFile(path, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := config.NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	um := users.NewManager(filepath.Join(dir, "users.toml"))
	if _, err := um.Reconcile(m.GetActive()); err != nil {
		t.Fatal(err)
	}
	// Disable alice in the registry (no draft involved).
	for _, u := range um.List() {
		u.Enabled = false
		if err := um.Put(&u); err != nil {
			t.Fatal(err)
		}
	}

	sm, err := settings.NewManager(filepath.Join(dir, "routebox.toml"))
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{config: m, settings: sm}
	h.SetUsers(um)
	// statusSource makes the process status overridable so the nil h.process is
	// never dereferenced. REQUIRES the line-185 prerequisite fix above — without it
	// ApplyConfig nil-PANICS (a panic here means the prerequisite was missed).
	h.statusSource = func() process.Status { return process.Status{Running: false} }
	h.v2rayAPISupported = func() bool { return false } // skip v2ray_api block

	// No draft: ApplyConfig persists active and runs the post-apply sync chain.
	req := httptest.NewRequest("POST", "/api/config/apply", nil)
	rr := httptest.NewRecorder()
	h.ApplyConfig(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("ApplyConfig status = %d, body=%s", rr.Code, rr.Body.String())
	}

	if m.HasDraft() {
		t.Error("draft should be cleared after apply (hasDraft must be false for the sync to run)")
	}
	route, _ := m.GetActive()["route"].(map[string]interface{})
	rules, _ := route["rules"].([]interface{})
	if len(rules) != 1 {
		t.Fatalf("active rules = %#v, want one managed reject rule for the disabled user", rules)
	}
	rm := rules[0].(map[string]interface{})
	if rm["action"] != "reject" {
		t.Fatalf("rules[0] = %#v, want a reject rule", rm)
	}
	au, _ := rm["auth_user"].([]interface{})
	if len(au) != 1 || au[0] != "alice" {
		t.Fatalf("auth_user = %#v, want [alice]", au)
	}
}
