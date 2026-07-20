package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"routebox/backend/internal/process"
)

func TestParseClashVersion(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantVer string
		wantOK  bool
	}{
		{
			name:    "valid sing-box response",
			body:    `{"meta":true,"version":"1.13.13-awg2.1"}`,
			wantVer: "1.13.13-awg2.1",
			wantOK:  true,
		},
		{
			name:    "version only",
			body:    `{"version":"1.9.0"}`,
			wantVer: "1.9.0",
			wantOK:  true,
		},
		{
			name:    "sing-box product prefix stripped",
			body:    `{"meta":true,"premium":true,"version":"sing-box 1.13.13-awg2.1"}`,
			wantVer: "1.13.13-awg2.1",
			wantOK:  true,
		},
		{
			name:    "missing version field",
			body:    `{"meta":true}`,
			wantVer: "",
			wantOK:  false,
		},
		{
			name:    "empty version field",
			body:    `{"version":""}`,
			wantVer: "",
			wantOK:  false,
		},
		{
			name:    "malformed json",
			body:    `{"version":`,
			wantVer: "",
			wantOK:  false,
		},
		{
			name:    "empty body",
			body:    ``,
			wantVer: "",
			wantOK:  false,
		},
		{
			name:    "not an object",
			body:    `["1.0.0"]`,
			wantVer: "",
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVer, gotOK := parseClashVersion([]byte(tt.body))
			if gotVer != tt.wantVer || gotOK != tt.wantOK {
				t.Fatalf("parseClashVersion(%q) = (%q, %v), want (%q, %v)",
					tt.body, gotVer, gotOK, tt.wantVer, tt.wantOK)
			}
		})
	}
}

// runningSingboxVersion must authenticate with the Clash API secret when one is
// configured, and send no Authorization header when it is empty.
func TestRunningSingboxVersion_SecretHeader(t *testing.T) {
	var mu sync.Mutex
	gotAuth := "unset"
	clash := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotAuth = r.Header.Get("Authorization")
		mu.Unlock()
		if r.URL.Path != "/version" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"meta":true,"version":"1.13.13-awg2.1"}`))
	}))
	defer clash.Close()
	addr := strings.TrimPrefix(clash.URL, "http://")

	t.Run("secret sent as bearer", func(t *testing.T) {
		ver, ok := runningSingboxVersion(addr, "s3cr3t")
		if !ok || ver != "1.13.13-awg2.1" {
			t.Fatalf("runningSingboxVersion = (%q, %v), want (1.13.13-awg2.1, true)", ver, ok)
		}
		mu.Lock()
		defer mu.Unlock()
		if gotAuth != "Bearer s3cr3t" {
			t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer s3cr3t")
		}
	})

	t.Run("no secret sends no header", func(t *testing.T) {
		if _, ok := runningSingboxVersion(addr, ""); !ok {
			t.Fatalf("runningSingboxVersion without secret failed")
		}
		mu.Lock()
		defer mu.Unlock()
		if gotAuth != "" {
			t.Fatalf("Authorization = %q, want none", gotAuth)
		}
	})
}

func decodeStatus(t *testing.T, rec *httptest.ResponseRecorder) process.Status {
	t.Helper()
	var resp struct {
		Success bool           `json:"success"`
		Data    process.Status `json:"data"`
		Error   string         `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}
	if !resp.Success {
		t.Fatalf("success=false: %s", resp.Error)
	}
	return resp.Data
}

// When the process is running and the Clash /version endpoint reports a version,
// GetStatus overlays that running version onto the status.
func TestGetStatus_RunningOverlaysClashVersion(t *testing.T) {
	clash := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"meta":true,"version":"1.13.13-awg2.1"}`))
	}))
	defer clash.Close()

	addr := strings.TrimPrefix(clash.URL, "http://")
	h := &Handler{
		clashAddr: addr,
		statusSource: func() process.Status {
			return process.Status{Running: true, Version: "0.0.0-on-disk"}
		},
	}

	rec := httptest.NewRecorder()
	h.GetStatus(rec, httptest.NewRequest(http.MethodGet, "/api/status", nil))

	status := decodeStatus(t, rec)
	if !status.Running {
		t.Fatalf("expected running=true")
	}
	if status.Version != "1.13.13-awg2.1" {
		t.Fatalf("Version = %q, want running clash version %q", status.Version, "1.13.13-awg2.1")
	}
}

// When the process is running but the Clash query fails (server down/closed),
// GetStatus falls back to the on-disk binary version from process.GetStatus.
func TestGetStatus_RunningClashDownFallsBack(t *testing.T) {
	clash := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := strings.TrimPrefix(clash.URL, "http://")
	clash.Close() // ensure the query fails

	h := &Handler{
		clashAddr: addr,
		statusSource: func() process.Status {
			return process.Status{Running: true, Version: "0.0.0-on-disk"}
		},
	}

	rec := httptest.NewRecorder()
	h.GetStatus(rec, httptest.NewRequest(http.MethodGet, "/api/status", nil))

	status := decodeStatus(t, rec)
	if status.Version != "0.0.0-on-disk" {
		t.Fatalf("Version = %q, want on-disk fallback %q", status.Version, "0.0.0-on-disk")
	}
}

// When the process is not running, GetStatus never queries Clash and reports the
// on-disk binary version unchanged.
func TestGetStatus_NotRunningUsesBinaryVersion(t *testing.T) {
	queried := false
	clash := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queried = true
		w.Write([]byte(`{"version":"should-not-be-used"}`))
	}))
	defer clash.Close()

	h := &Handler{
		clashAddr: strings.TrimPrefix(clash.URL, "http://"),
		statusSource: func() process.Status {
			return process.Status{Running: false, Version: "0.0.0-on-disk"}
		},
	}

	rec := httptest.NewRecorder()
	h.GetStatus(rec, httptest.NewRequest(http.MethodGet, "/api/status", nil))

	status := decodeStatus(t, rec)
	if status.Version != "0.0.0-on-disk" {
		t.Fatalf("Version = %q, want on-disk version %q", status.Version, "0.0.0-on-disk")
	}
	if queried {
		t.Fatalf("Clash /version must not be queried when process is not running")
	}
}
