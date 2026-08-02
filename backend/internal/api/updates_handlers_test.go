package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// TestApplySelfUpdateReExecsInstallPath: after Apply swaps the new binary
// into the install path (and the running binary to <path>.old), the handler
// must re-exec the INSTALL path — not os.Executable(), which on Linux
// resolves /proc/self/exe to the renamed .old backup.
func TestApplySelfUpdateReExecsInstallPath(t *testing.T) {
	dir := t.TempDir()
	installPath := filepath.Join(dir, "routebox")

	// Current binary on disk: a real ELF so nothing chokes on it.
	oldBytes, err := os.ReadFile("/bin/true")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installPath, oldBytes, 0755); err != nil {
		t.Fatal(err)
	}
	// New binary: same ELF + padding so content differs but still runs.
	newBytes := append(append([]byte{}, oldBytes...), []byte("\npad")...)
	sum := sha256.Sum256(newBytes)
	hash := hex.EncodeToString(sum[:])

	// Asset server: binary + single-file checksum.
	mux := http.NewServeMux()
	mux.HandleFunc("/asset", func(w http.ResponseWriter, r *http.Request) {
		w.Write(newBytes)
	})
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  routebox-linux-amd64\n", hash)
	})
	assets := httptest.NewServer(mux)
	t.Cleanup(assets.Close)

	// GitHub API fixture pointing at the asset server.
	ghJSON := fmt.Sprintf(`{
	  "tag_name": "v2.0.0",
	  "body": "notes",
	  "published_at": "2026-06-01T10:00:00Z",
	  "assets": [
	    {"name": "routebox-linux-amd64", "browser_download_url": %q},
	    {"name": "checksums.txt", "browser_download_url": %q}
	  ]
	}`, assets.URL+"/asset", assets.URL+"/checksums.txt")
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(ghJSON))
	}))
	t.Cleanup(gh.Close)
	t.Setenv("UPDATES_API_BASE", gh.URL)

	// Swap-aware BinaryPath: mimics os.Executable()//proc/self/exe, which
	// follows the running inode. Before Apply's rename swap the file at
	// installPath is still the original binary → return installPath. After
	// Apply renames it to installPath+".old" (and moves the new binary in),
	// the running executable resolves to the .old path. This pins the
	// pre-Apply capture ordering: a refactor that captured execPath AFTER
	// Apply would observe ".old" and fail the assertions below.
	swapAwareBinaryPath := func() string {
		cur, err := os.ReadFile(installPath)
		if err == nil && bytes.Equal(cur, oldBytes) {
			return installPath
		}
		return installPath + ".old"
	}

	target := updates.Target{
		Name:           "routebox",
		Repo:           "hoaxisr/routebox",
		AssetSuffix:    func(string) (string, bool) { return "linux-amd64", true },
		BinaryPath:     swapAwareBinaryPath,
		CurrentVersion: func() (string, error) { return "1.0.0", nil },
		SelfUpdate:     true,
	}
	h := NewHandler(nil, nil, "", nil, nil, nil, nil)
	h.SetUpdatesService(&updates.Service{
		Checker: updates.NewChecker(),
		Updater: updates.NewUpdater(),
		Targets: []updates.Target{target},
	})

	// Swap the exit seam so the test process survives, capturing the path.
	got := "(not called)"
	orig := scheduleExit
	scheduleExit = func(path string) { got = path }
	t.Cleanup(func() { scheduleExit = orig })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/updates/apply", strings.NewReader(`{"target":"routebox"}`))
	h.ApplyUpdate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	data := decodeData(t, rec)
	if data["restarting"] != true {
		t.Fatalf("restarting = %v, want true", data["restarting"])
	}

	if got == "(not called)" {
		t.Fatal("scheduleExit was not called on successful self-update")
	}
	if got != installPath {
		t.Errorf("scheduleExit path = %q, want install path %q", got, installPath)
	}
	if got == "" || strings.HasSuffix(got, ".old") {
		t.Errorf("scheduleExit must never receive empty or .old path, got %q", got)
	}
}

// dockerHandler returns a handler in Docker mode carrying both target kinds:
// the self-update one (RouteBox's own binary, which lives in the image) and a
// normal one (amnezia-box, which lives on the /config volume and stays
// updatable). The distinction is the whole point of the mode.
func dockerHandler(t *testing.T) *Handler {
	t.Helper()
	h := newUpdatesHandler(t)
	h.updates.Targets = append(h.updates.Targets, updates.Target{
		Name:           "routebox",
		Repo:           "hoaxisr/routebox",
		AssetSuffix:    func(string) (string, bool) { return "linux-amd64", true },
		BinaryPath:     func() string { return "/usr/bin/routebox" },
		CurrentVersion: func() (string, error) { return "1.0.0", nil },
		SelfUpdate:     true,
	})
	h.SetDockerMode(true)
	return h
}

func TestDockerModeRefusesSelfUpdate(t *testing.T) {
	h := dockerHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/updates/apply", strings.NewReader(`{"target":"routebox"}`))
	h.ApplyUpdate(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	// The message must carry the command that replaces the button, or the refusal
	// is a dead end for whoever hit it.
	if body := rec.Body.String(); !strings.Contains(body, "docker compose pull") {
		t.Errorf("refusal does not say what to run instead: %s", body)
	}
}

// TestDockerModeStillUpdatesAmneziaBox is the guard against over-blocking: the
// image is the source of truth for RouteBox's binary only. amnezia-box lives on
// the writable volume and must keep its Apply button.
func TestDockerModeStillUpdatesAmneziaBox(t *testing.T) {
	h := dockerHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/updates/apply", strings.NewReader(`{"target":"amnezia-box"}`))
	h.ApplyUpdate(rec, req)
	if rec.Code == http.StatusConflict {
		t.Fatalf("amnezia-box must not be refused as docker-managed: %s", rec.Body.String())
	}
}

// TestDockerModeStatusFlagsOnlySelfUpdate covers what the Updates page renders
// from: the flag and the command ride on the RouteBox target alone.
func TestDockerModeStatusFlagsOnlySelfUpdate(t *testing.T) {
	h := dockerHandler(t)
	rec := httptest.NewRecorder()
	h.GetUpdatesStatus(rec, httptest.NewRequest("GET", "/api/updates/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	for _, raw := range decodeData(t, rec)["targets"].([]interface{}) {
		ts := raw.(map[string]interface{})
		wantManaged := ts["name"] == "routebox"
		if got := ts["docker_managed"] == true; got != wantManaged {
			t.Errorf("%v: docker_managed = %v, want %v", ts["name"], got, wantManaged)
		}
		if _, has := ts["update_command"]; has != wantManaged {
			t.Errorf("%v: update_command present = %v, want %v", ts["name"], has, wantManaged)
		}
	}
}

// TestWithoutDockerModeNoFlags: the same handler outside a container must look
// exactly as it did before the mode existed — no flag, no command, no button
// swapped out on a bare-metal install.
func TestWithoutDockerModeNoFlags(t *testing.T) {
	h := dockerHandler(t)
	h.SetDockerMode(false)
	rec := httptest.NewRecorder()
	h.GetUpdatesStatus(rec, httptest.NewRequest("GET", "/api/updates/status", nil))
	for _, raw := range decodeData(t, rec)["targets"].([]interface{}) {
		ts := raw.(map[string]interface{})
		if _, has := ts["docker_managed"]; has {
			t.Errorf("%v: docker_managed leaked outside Docker", ts["name"])
		}
		if _, has := ts["update_command"]; has {
			t.Errorf("%v: update_command leaked outside Docker", ts["name"])
		}
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
