package api

import (
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
