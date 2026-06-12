package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"routebox/backend/internal/settings"
)

func newSettingsFromTOML(t *testing.T, content string) *settings.Manager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "routebox.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := settings.NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestBasicAuth(t *testing.T) {
	const enabled = `
[security]
auth_enabled = true
auth_username = "admin"
auth_password = "secret"
`
	const disabled = `
[security]
auth_enabled = false
`
	// auth_enabled without a password is a misconfiguration; must be denied.
	const enabledEmpty = `
[security]
auth_enabled = true
`
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cases := []struct {
		name       string
		toml       string
		user, pass string
		sendCreds  bool
		wantStatus int
	}{
		{"disabled no creds", disabled, "", "", false, http.StatusOK},
		{"enabled no creds", enabled, "", "", false, http.StatusUnauthorized},
		{"enabled wrong creds", enabled, "admin", "wrong", true, http.StatusUnauthorized},
		{"enabled right creds", enabled, "admin", "secret", true, http.StatusOK},
		{"enabled empty creds misconfig denied", enabledEmpty, "", "", true, http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mgr := newSettingsFromTOML(t, tc.toml)
			handler := BasicAuth(mgr)(okHandler)
			req := httptest.NewRequest("GET", "/api/status", nil)
			if tc.sendCreds {
				req.SetBasicAuth(tc.user, tc.pass)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantStatus == http.StatusUnauthorized && rec.Header().Get("WWW-Authenticate") == "" {
				t.Fatal("missing WWW-Authenticate header")
			}
		})
	}
}

// TestBasicAuth_NilManager verifies that a nil settings manager (used in
// development/testing without auth) passes all requests through.
func TestBasicAuth_NilManager(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := BasicAuth(nil)(okHandler)
	req := httptest.NewRequest("GET", "/api/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("nil manager: status = %d, want %d", rec.Code, http.StatusOK)
	}
}
