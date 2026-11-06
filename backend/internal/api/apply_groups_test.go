package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"routebox/backend/internal/config"
	"routebox/backend/internal/process"
)

// A fallback group whose interval outlives idle_timeout passes `check` and
// kills amnezia-box on reload; apply must refuse it before anything is written.
func TestApplyConfig_RefusesGroupThatFailsOnStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	active := `{"outbounds":[{"type":"direct","tag":"direct"}]}`
	if err := os.WriteFile(path, []byte(active), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := config.NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CreateOutbound(map[string]interface{}{"type": "fallback", "tag": "fb", "outbounds": []interface{}{"direct"}, "interval": "1h"}); err != nil {
		t.Fatal(err)
	}
	h := &Handler{config: m}
	h.statusSource = func() process.Status { return process.Status{Running: false} }

	rr := httptest.NewRecorder()
	h.ApplyConfig(rr, httptest.NewRequest("POST", "/api/config/apply", nil))
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "idle_timeout") {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	if !m.HasDraft() {
		t.Error("draft must survive a refused apply")
	}
	if b, _ := os.ReadFile(path); string(b) != active {
		t.Errorf("active config was rewritten: %s", b)
	}
}

// An emptied domain set passes every save and only surfaces as a FATAL that
// names no set; apply must say which one.
func TestApplyConfig_RefusesEmptyInlineRuleSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := config.NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CreateRuleSet(map[string]interface{}{"tag": "youtube", "type": "inline", "rules": []interface{}{}}); err != nil {
		t.Fatal(err)
	}
	h := &Handler{config: m}
	h.statusSource = func() process.Status { return process.Status{Running: false} }

	rr := httptest.NewRecorder()
	h.ApplyConfig(rr, httptest.NewRequest("POST", "/api/config/apply", nil))
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "youtube") {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
}

func TestApplyConfig_WarnsWhenRuleSetDownloadNeedsATunnelWithoutCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	active := `{"outbounds":[{"type":"direct","tag":"direct"},{"type":"socks","tag":"tun1","server":"127.0.0.1","server_port":1080}],
		"route":{"rule_set":[{"tag":"youtube","type":"remote","url":"https://x/y.srs","http_client":{"detour":"tun1"}}]}}`
	if err := os.WriteFile(path, []byte(active), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := config.NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{config: m}
	h.statusSource = func() process.Status { return process.Status{Running: false} }

	rr := httptest.NewRecorder()
	h.ApplyConfig(rr, httptest.NewRequest("POST", "/api/config/apply", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `\"youtube\"`) {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
}

// The warning matters most when the start it warns about did fail.
func TestApplyConfig_FailedReloadCarriesTheRuleSetWarning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	active := `{"outbounds":[{"type":"direct","tag":"direct"},{"type":"socks","tag":"tun1","server":"127.0.0.1","server_port":1080}],
		"route":{"rule_set":[{"tag":"youtube","type":"remote","url":"https://x/y.srs","http_client":{"detour":"tun1"}}]}}`
	if err := os.WriteFile(path, []byte(active), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := config.NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	fail := func() error { return errors.New("exited immediately") }
	h := &Handler{config: m, reloader: fail, restarter: func(string) error { return fail() }}
	h.statusSource = func() process.Status { return process.Status{Running: true} }

	rr := httptest.NewRecorder()
	h.ApplyConfig(rr, httptest.NewRequest("POST", "/api/config/apply", nil))
	if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "exited immediately") || !strings.Contains(rr.Body.String(), `\"youtube\"`) {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
}
