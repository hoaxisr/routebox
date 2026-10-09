package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"routebox/backend/internal/process"
)

func TestApplyProgress_StaleRunDoesNotOverwrite(t *testing.T) {
	var p applyProgress
	seq := p.start()
	p.set(seq, "error", "boom", "")
	seq = p.start()
	if s := p.snapshot(); s["phase"] != "checking" || s["error"] != nil {
		t.Fatalf("new run must clear the old error: %v", s)
	}
	p.set(seq-1, "done", "", "")
	if s := p.snapshot(); s["phase"] != "checking" {
		t.Fatalf("stale set leaked into the current run: %v", s)
	}
}

func TestApplyRecorder_Outcome(t *testing.T) {
	cases := []struct {
		name                   string
		write                  func(w http.ResponseWriter)
		phase, errMsg, warning string
	}{
		{"200", func(w http.ResponseWriter) { writeSuccess(w, map[string]interface{}{"message": "ok"}) }, "done", "", ""},
		{"200 warning", func(w http.ResponseWriter) {
			writeSuccess(w, map[string]interface{}{"message": "ok", "warning": "naive old"})
		}, "done", "", "naive old"},
		{"400", func(w http.ResponseWriter) { writeError(w, 400, "Config validation failed: x") }, "error", "Config validation failed: x", ""},
		{"500", func(w http.ResponseWriter) { writeError(w, 500, "Saved but failed to restart: y") }, "error", "Saved but failed to restart: y", ""},
		{"502 non-json", func(w http.ResponseWriter) { w.WriteHeader(502); w.Write([]byte("<html>")) }, "error", "Bad Gateway", ""},
		{"nothing written (panic)", func(w http.ResponseWriter) {}, "error", "apply crashed, see the RouteBox log", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := &applyRecorder{ResponseWriter: httptest.NewRecorder()}
			c.write(rec)
			phase, msg, warning := rec.outcome()
			if phase != c.phase || msg != c.errMsg || warning != c.warning {
				t.Fatalf("outcome = (%q, %q, %q), want (%q, %q, %q)", phase, msg, warning, c.phase, c.errMsg, c.warning)
			}
		})
	}
}

func applyBlock(t *testing.T, h *Handler) map[string]interface{} {
	t.Helper()
	rr := httptest.NewRecorder()
	h.GetApplyProgress(rr, httptest.NewRequest("GET", "/api/config/apply/progress", nil))
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil || resp.Data == nil {
		t.Fatalf("progress: %v %s", err, rr.Body.String())
	}
	return resp.Data
}

func postApply(h *Handler) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	h.ApplyConfig(rr, httptest.NewRequest("POST", "/api/config/apply", nil))
	return rr
}

func TestApplyProgress_FollowsApplyConfig(t *testing.T) {
	h, _ := newApplyHandler(t, "alice")
	if a := applyBlock(t, h); a["seq"] != float64(0) || a["phase"] != "" {
		t.Fatalf("before any apply: %v", a)
	}
	if rr := postApply(h); rr.Code != http.StatusOK {
		t.Fatalf("apply = %d, body=%s", rr.Code, rr.Body.String())
	}
	if a := applyBlock(t, h); a["seq"] != float64(1) || a["phase"] != "done" || a["error"] != nil {
		t.Fatalf("after a successful apply: %v", a)
	}

	// A rejected draft answers 400; the phone may never see it, so progress
	// must carry the same message.
	draft := h.config.Get()
	draft["route"] = map[string]interface{}{"rule_set": []interface{}{
		map[string]interface{}{"type": "local", "tag": "x", "format": "binary", "path": "missing.srs"},
	}}
	if err := h.config.SetDraft(draft); err != nil {
		t.Fatal(err)
	}
	if rr := postApply(h); rr.Code != http.StatusBadRequest {
		t.Fatalf("apply = %d, body=%s", rr.Code, rr.Body.String())
	}
	a := applyBlock(t, h)
	msg, _ := a["error"].(string)
	if a["seq"] != float64(2) || a["phase"] != "error" || !strings.Contains(msg, "Config validation failed") {
		t.Fatalf("after a rejected apply: %v", a)
	}
}

func TestApplyConfig_ReportsReloadingBeforeTheReload(t *testing.T) {
	h, _ := newApplyHandler(t, "alice")
	h.statusSource = func() process.Status { return process.Status{Running: true} }
	var seen interface{}
	h.reloader = func() error { seen = h.applyProg.snapshot()["phase"]; return nil }

	if rr := postApply(h); rr.Code != http.StatusOK {
		t.Fatalf("apply = %d, body=%s", rr.Code, rr.Body.String())
	}
	if seen != "reloading" {
		t.Fatalf("phase at SIGHUP time = %v, want reloading", seen)
	}
	if a := applyBlock(t, h); a["phase"] != "done" {
		t.Fatalf("after reload: %v", a)
	}
}

func TestApplyConfig_ReloadAndRestartFail_ProgressCarriesTheError(t *testing.T) {
	h, _ := newApplyHandler(t, "alice")
	h.statusSource = func() process.Status { return process.Status{Running: true} }
	h.reloader = func() error { return errors.New("sighup refused") }
	h.restarter = func(string) error { return errors.New("unit dead") }

	if rr := postApply(h); rr.Code != http.StatusInternalServerError {
		t.Fatalf("apply = %d", rr.Code)
	}
	a := applyBlock(t, h)
	if msg, _ := a["error"].(string); a["phase"] != "error" || !strings.Contains(msg, "unit dead") {
		t.Fatalf("progress after failed reload: %v", a)
	}
}

func TestApplyConfig_OneAtATime(t *testing.T) {
	h, _ := newApplyHandler(t, "alice")
	h.applyMu.Lock()
	rr := postApply(h)
	h.applyMu.Unlock()
	if rr.Code != http.StatusConflict {
		t.Fatalf("concurrent apply = %d, want 409", rr.Code)
	}
	if a := applyBlock(t, h); a["seq"] != float64(0) {
		t.Fatalf("a refused apply must not start a run: %v", a)
	}
}
