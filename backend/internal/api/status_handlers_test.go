package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"routebox/backend/internal/process"
)

// TestVPSModeOmitsRouterSystemChecks: system_checks answer "can this host route
// a LAN through a TUN interface", which a VPS panel never does. Reporting them
// there put a red "System Requirements Not Met — run routebox with sudo" banner
// on a panel that was working as designed, advising the operator to undo the
// unprivileged operation vps mode exists to allow.
func TestVPSModeOmitsRouterSystemChecks(t *testing.T) {
	checks := &process.SystemChecks{IsRoot: false, IPv4Forward: false, AllChecksPassed: false}

	h := &Handler{statusSource: func() process.Status {
		return process.Status{SystemChecks: checks}
	}}
	h.SetPanelMode("vps")

	rec := httptest.NewRecorder()
	h.GetStatus(rec, httptest.NewRequest("GET", "/api/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if _, present := resp.Data["system_checks"]; present {
		t.Fatalf("vps mode must not report router system checks: %v", resp.Data["system_checks"])
	}

	// Router mode is untouched: the banner is the whole point there.
	h.SetPanelMode("router")
	rec = httptest.NewRecorder()
	h.GetStatus(rec, httptest.NewRequest("GET", "/api/status", nil))
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if _, present := resp.Data["system_checks"]; !present {
		t.Fatal("router mode must still report system checks")
	}
}
