package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"routebox/backend/internal/auth"
	"routebox/backend/internal/settings"
)

func newSettingsFromTOML(t *testing.T, content string) *settings.Manager {
	t.Helper()
	path := t.TempDir() + "/routebox.toml"
	m, err := settings.NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	_ = content // content handled via Update calls in callers
	return m
}

func TestAuthMiddleware(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// disabled: all requests pass
	t.Run("disabled no creds", func(t *testing.T) {
		dir := t.TempDir()
		m, _ := settings.NewManager(dir + "/routebox.toml")
		_ = m.Load()
		handler := AuthMiddleware(m, nil, nil, nil)(okHandler)
		req := httptest.NewRequest("GET", "/api/status", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("disabled: want 200, got %d", rec.Code)
		}
	})

	// enabled, no creds sent -> 401
	t.Run("enabled no creds", func(t *testing.T) {
		dir := t.TempDir()
		m, _ := settings.NewManager(dir + "/routebox.toml")
		_ = m.Load()
		_ = m.Update(map[string]interface{}{"security.auth_enabled": true, "security.auth_username": "admin"})
		_ = m.Update(map[string]interface{}{"security.auth_password": "secret"})
		handler := AuthMiddleware(m, auth.NewSessionStore(), auth.NewLimiter(), auth.NewCachedVerifier())(okHandler)
		req := httptest.NewRequest("GET", "/api/status", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("no creds: want 401, got %d", rec.Code)
		}
		if rec.Header().Get("WWW-Authenticate") == "" {
			t.Fatal("missing WWW-Authenticate header")
		}
	})

	// enabled, wrong creds -> 401
	t.Run("enabled wrong creds", func(t *testing.T) {
		dir := t.TempDir()
		m, _ := settings.NewManager(dir + "/routebox.toml")
		_ = m.Load()
		_ = m.Update(map[string]interface{}{"security.auth_enabled": true, "security.auth_username": "admin"})
		_ = m.Update(map[string]interface{}{"security.auth_password": "secret"})
		handler := AuthMiddleware(m, auth.NewSessionStore(), auth.NewLimiter(), auth.NewCachedVerifier())(okHandler)
		req := httptest.NewRequest("GET", "/api/status", nil)
		req.SetBasicAuth("admin", "wrong")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("wrong creds: want 401, got %d", rec.Code)
		}
	})

	// enabled, right creds -> 200
	t.Run("enabled right creds", func(t *testing.T) {
		dir := t.TempDir()
		m, _ := settings.NewManager(dir + "/routebox.toml")
		_ = m.Load()
		_ = m.Update(map[string]interface{}{"security.auth_enabled": true, "security.auth_username": "admin"})
		_ = m.Update(map[string]interface{}{"security.auth_password": "secret"})
		handler := AuthMiddleware(m, auth.NewSessionStore(), auth.NewLimiter(), auth.NewCachedVerifier())(okHandler)
		req := httptest.NewRequest("GET", "/api/status", nil)
		req.SetBasicAuth("admin", "secret")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("right creds: want 200, got %d", rec.Code)
		}
	})

	// enabled, no hash configured -> 401 (fail closed)
	t.Run("enabled empty hash misconfig denied", func(t *testing.T) {
		dir := t.TempDir()
		m, _ := settings.NewManager(dir + "/routebox.toml")
		_ = m.Load()
		_ = m.Update(map[string]interface{}{"security.auth_enabled": true, "security.auth_username": "admin"})
		// do not set password — hash stays empty
		handler := AuthMiddleware(m, auth.NewSessionStore(), auth.NewLimiter(), auth.NewCachedVerifier())(okHandler)
		req := httptest.NewRequest("GET", "/api/status", nil)
		req.SetBasicAuth("", "")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("misconfig: want 401, got %d", rec.Code)
		}
	})
}

// TestAuthMiddleware_NilManager verifies that a nil settings manager passes all requests through.
func TestAuthMiddleware_NilManager(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := AuthMiddleware(nil, nil, nil, nil)(okHandler)
	req := httptest.NewRequest("GET", "/api/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("nil manager: status = %d, want %d", rec.Code, http.StatusOK)
	}
}
