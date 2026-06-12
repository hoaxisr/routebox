package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"routebox/backend/internal/updates"
)

const githubFixture = `{
  "tag_name": "v2.0.0",
  "body": "notes here",
  "published_at": "2026-06-01T10:00:00Z",
  "assets": [
    {"name": "fake-box-linux-amd64", "browser_download_url": "https://dl/fake-box-linux-amd64"},
    {"name": "checksums.txt", "browser_download_url": "https://dl/checksums.txt"}
  ]
}`

func newUpdatesHandler(t *testing.T) *Handler {
	t.Helper()
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(githubFixture))
	}))
	t.Cleanup(gh.Close)
	t.Setenv("UPDATES_API_BASE", gh.URL)

	target := updates.Target{
		Name:           "amnezia-box",
		Repo:           "hoaxisr/amnezia-box",
		AssetSuffix:    func(string) (string, bool) { return "linux-amd64", true },
		BinaryPath:     func() string { return "/tmp/fake-box" },
		CurrentVersion: func() (string, error) { return "1.0.0", nil },
		Restart:        func() error { return nil },
	}
	h := NewHandler(nil, nil, "", nil, nil, nil, nil)
	h.SetUpdatesService(&updates.Service{
		Checker: updates.NewChecker(),
		Updater: updates.NewUpdater(),
		Targets: []updates.Target{target},
	})
	return h
}

func decodeData(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var resp struct {
		Success bool                   `json:"success"`
		Data    map[string]interface{} `json:"data"`
		Error   string                 `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}
	if !resp.Success {
		t.Fatalf("success=false: %s", resp.Error)
	}
	return resp.Data
}

func TestUpdatesStatusBeforeAnyCheck(t *testing.T) {
	h := newUpdatesHandler(t)
	rec := httptest.NewRecorder()
	h.GetUpdatesStatus(rec, httptest.NewRequest("GET", "/api/updates/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	data := decodeData(t, rec)
	targets := data["targets"].([]interface{})
	ts := targets[0].(map[string]interface{})
	if ts["name"] != "amnezia-box" || ts["current"] != "1.0.0" {
		t.Errorf("unexpected target status: %v", ts)
	}
	if ts["update_available"] != false {
		t.Error("no check yet → update_available must be false")
	}
	if _, hasChecked := ts["last_checked"]; hasChecked {
		t.Error("last_checked must be absent before first check")
	}
}

func TestUpdatesCheckThenStatus(t *testing.T) {
	h := newUpdatesHandler(t)

	rec := httptest.NewRecorder()
	h.CheckUpdates(rec, httptest.NewRequest("POST", "/api/updates/check", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("check status = %d: %s", rec.Code, rec.Body.String())
	}
	data := decodeData(t, rec)
	ts := data["targets"].([]interface{})[0].(map[string]interface{})
	if ts["latest"] != "2.0.0" {
		t.Errorf("latest = %v, want 2.0.0", ts["latest"])
	}
	if ts["update_available"] != true {
		t.Error("2.0.0 > 1.0.0 → update_available must be true")
	}
	if !strings.Contains(ts["notes"].(string), "notes here") {
		t.Errorf("notes = %v", ts["notes"])
	}

	// Status now serves from cache without hitting GitHub again
	rec = httptest.NewRecorder()
	h.GetUpdatesStatus(rec, httptest.NewRequest("GET", "/api/updates/status", nil))
	ts = decodeData(t, rec)["targets"].([]interface{})[0].(map[string]interface{})
	if ts["latest"] != "2.0.0" || ts["update_available"] != true {
		t.Errorf("cached status wrong: %v", ts)
	}
}

func TestUpdatesProgressIdle(t *testing.T) {
	h := newUpdatesHandler(t)
	rec := httptest.NewRecorder()
	h.GetUpdatesProgress(rec, httptest.NewRequest("GET", "/api/updates/progress", nil))
	data := decodeData(t, rec)
	if data["phase"] != "idle" {
		t.Errorf("phase = %v, want idle", data["phase"])
	}
}

func TestApplyUnknownTarget(t *testing.T) {
	h := newUpdatesHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/updates/apply", strings.NewReader(`{"target":"nope"}`))
	h.ApplyUpdate(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestApplyWhenUpToDate(t *testing.T) {
	h := newUpdatesHandler(t)
	// Make current == latest so apply must refuse
	h.updates.Targets[0].CurrentVersion = func() (string, error) { return "2.0.0", nil }
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/updates/apply", strings.NewReader(`{"target":"amnezia-box"}`))
	h.ApplyUpdate(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (already up to date)", rec.Code)
	}
}
