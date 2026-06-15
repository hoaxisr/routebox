package api

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"routebox/backend/internal/auth"
	"routebox/backend/internal/settings"
)

func newAuthHandler(t *testing.T) *Handler {
	t.Helper()
	dir := t.TempDir()
	// NewManager already calls Load() internally; a missing file uses defaults.
	m, err := settings.NewManager(dir + "/routebox.toml")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Update(map[string]interface{}{"security.auth_enabled": true, "security.auth_username": "admin"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Update(map[string]interface{}{"security.auth_password": "pw"}); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(nil, nil, "", nil, m, nil, nil)
	h.SetAuth(auth.NewSessionStore(), auth.NewLimiter(), auth.NewCachedVerifier())
	return h
}

func authRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Post("/api/auth/login", h.Login)
		r.Post("/api/auth/logout", h.Logout)
		r.Get("/api/auth/session", h.Session)
	})
	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(h.settings, h.sessions, h.limiter, h.verifier))
		r.Get("/api/secret", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	})
	return r
}

func TestLoginFlow(t *testing.T) {
	h := newAuthHandler(t)
	srv := httptest.NewServer(authRouter(h))
	defer srv.Close()

	resp, _ := srv.Client().Get(srv.URL + "/api/secret")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauth secret should be 401, got %d", resp.StatusCode)
	}

	bad, _ := srv.Client().Post(srv.URL+"/api/auth/login", "application/json", strings.NewReader(`{"username":"admin","password":"nope"}`))
	if bad.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad login should be 401, got %d", bad.StatusCode)
	}

	jar, _ := cookiejar.New(nil)
	jc := srv.Client()
	jc.Jar = jar
	ok, _ := jc.Post(srv.URL+"/api/auth/login", "application/json", strings.NewReader(`{"username":"admin","password":"pw"}`))
	if ok.StatusCode != http.StatusOK {
		t.Fatalf("good login should be 200, got %d", ok.StatusCode)
	}
	sec, _ := jc.Get(srv.URL + "/api/secret")
	if sec.StatusCode != http.StatusOK {
		t.Fatalf("authed secret should be 200, got %d", sec.StatusCode)
	}
	jc.Post(srv.URL+"/api/auth/logout", "application/json", nil)
	sec2, _ := jc.Get(srv.URL + "/api/secret")
	if sec2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after logout secret should be 401, got %d", sec2.StatusCode)
	}
}

func TestBasicAuthStillWorks(t *testing.T) {
	h := newAuthHandler(t)
	srv := httptest.NewServer(authRouter(h))
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/api/secret", nil)
	req.SetBasicAuth("admin", "pw")
	resp, _ := srv.Client().Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid Basic should be 200, got %d", resp.StatusCode)
	}
}

func TestSessionEndpoint(t *testing.T) {
	h := newAuthHandler(t)
	srv := httptest.NewServer(authRouter(h))
	defer srv.Close()
	// Unauthenticated call: authenticated=false, auth_enabled=true, no username key.
	resp, _ := srv.Client().Get(srv.URL + "/api/auth/session")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("session endpoint should be 200, got %d", resp.StatusCode)
	}
	var envelope struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode session response: %v", err)
	}
	body := envelope.Data
	if authed, _ := body["authenticated"].(bool); authed {
		t.Fatalf("unauthenticated session should have authenticated=false")
	}
	if enabled, _ := body["auth_enabled"].(bool); !enabled {
		t.Fatalf("auth_enabled should be true")
	}
	if _, hasUsername := body["username"]; hasUsername {
		t.Fatalf("unauthenticated session must not expose username")
	}
}

func TestEmptyUsernameDenied(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	m, err := settings.NewManager(dir + "/routebox.toml")
	if err != nil {
		t.Fatal(err)
	}
	// Enable auth and set a password, but do NOT set auth_username (stays "").
	if err := m.Update(map[string]interface{}{"security.auth_enabled": true}); err != nil {
		t.Fatal(err)
	}
	if err := m.Update(map[string]interface{}{"security.auth_password": "pw"}); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(nil, nil, "", nil, m, nil, nil)
	h.SetAuth(auth.NewSessionStore(), auth.NewLimiter(), auth.NewCachedVerifier())

	srv := httptest.NewServer(authRouter(h))
	defer srv.Close()

	// Login with empty username should be 401.
	loginResp, _ := srv.Client().Post(
		srv.URL+"/api/auth/login",
		"application/json",
		strings.NewReader(`{"username":"","password":"pw"}`),
	)
	if loginResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("empty-username login should be 401, got %d", loginResp.StatusCode)
	}

	// Protected route with empty Basic creds should also be 401.
	req, _ := http.NewRequest("GET", srv.URL+"/api/secret", nil)
	req.SetBasicAuth("", "pw")
	secResp, _ := srv.Client().Do(req)
	if secResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("empty Basic username should be 401, got %d", secResp.StatusCode)
	}
}

func TestChangePasswordSuccess(t *testing.T) {
	h := newAuthHandler(t) // username "admin", password "pw"
	body := `{"current_password":"pw","new_password":"newsecret123"}`
	req := httptest.NewRequest("POST", "/api/auth/change-password", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ChangePassword(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("change-password should be 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	// The stored hash must now verify the NEW password and reject the OLD one.
	sec := h.settings.Get().Security
	if sec.AuthPasswordHash == "" {
		t.Fatal("hash should not be empty after change")
	}
	if !verifyPassword(h.verifier, sec.AuthPasswordHash, "newsecret123") {
		t.Fatal("new password should verify against the stored hash")
	}
	if verifyPassword(h.verifier, sec.AuthPasswordHash, "pw") {
		t.Fatal("old password must no longer verify")
	}
}

func TestChangePasswordWrongCurrent(t *testing.T) {
	h := newAuthHandler(t)
	before := h.settings.Get().Security.AuthPasswordHash
	body := `{"current_password":"WRONG","new_password":"newsecret123"}`
	req := httptest.NewRequest("POST", "/api/auth/change-password", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ChangePassword(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong current should be 401, got %d", rec.Code)
	}
	if after := h.settings.Get().Security.AuthPasswordHash; after != before {
		t.Fatal("hash must be unchanged on wrong current password")
	}
}

func TestChangePasswordShortNew(t *testing.T) {
	h := newAuthHandler(t)
	before := h.settings.Get().Security.AuthPasswordHash
	body := `{"current_password":"pw","new_password":"short"}`
	req := httptest.NewRequest("POST", "/api/auth/change-password", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ChangePassword(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("short new should be 400, got %d", rec.Code)
	}
	if after := h.settings.Get().Security.AuthPasswordHash; after != before {
		t.Fatal("hash must be unchanged when new password is rejected")
	}
}

func TestChangePasswordUnconfigured(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	m, err := settings.NewManager(dir + "/routebox.toml")
	if err != nil {
		t.Fatal(err)
	}
	// Enable auth but never set a password → AuthPasswordHash == "".
	if err := m.Update(map[string]interface{}{"security.auth_enabled": true, "security.auth_username": "admin"}); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(nil, nil, "", nil, m, nil, nil)
	h.SetAuth(auth.NewSessionStore(), auth.NewLimiter(), auth.NewCachedVerifier())

	body := `{"current_password":"anything","new_password":"newsecret123"}`
	req := httptest.NewRequest("POST", "/api/auth/change-password", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ChangePassword(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unconfigured change should be 400, got %d", rec.Code)
	}
}

func TestChangePasswordLockout(t *testing.T) {
	h := newAuthHandler(t)
	// Hammer wrong current until the limiter trips, then expect 429.
	// auth.NewLimiter() locks after a bounded number of failures; loop generously
	// and assert that a 429 occurs (and that the last response is 429).
	var last int
	for i := 0; i < 50; i++ {
		body := `{"current_password":"WRONG","new_password":"newsecret123"}`
		req := httptest.NewRequest("POST", "/api/auth/change-password", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ChangePassword(rec, req)
		last = rec.Code
		if rec.Code == http.StatusTooManyRequests {
			return // lockout reached — success
		}
	}
	t.Fatalf("expected a 429 after repeated wrong attempts, last status %d", last)
}

func authRouterWithChangePw(h *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Post("/api/auth/login", h.Login)
		r.Post("/api/auth/logout", h.Logout)
		r.Get("/api/auth/session", h.Session)
	})
	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(h.settings, h.sessions, h.limiter, h.verifier))
		r.Post("/api/auth/change-password", h.ChangePassword)
	})
	return r
}

func TestChangePasswordRequiresSession(t *testing.T) {
	h := newAuthHandler(t)
	srv := httptest.NewServer(authRouterWithChangePw(h))
	defer srv.Close()

	// No session/Basic creds → AuthMiddleware rejects with 401.
	body := `{"current_password":"pw","new_password":"newsecret123"}`
	noauth, _ := srv.Client().Post(srv.URL+"/api/auth/change-password", "application/json", strings.NewReader(body))
	if noauth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("change-password without session should be 401, got %d", noauth.StatusCode)
	}

	// With a valid session cookie → reaches the handler → 200.
	jar, _ := cookiejar.New(nil)
	jc := srv.Client()
	jc.Jar = jar
	if ok, _ := jc.Post(srv.URL+"/api/auth/login", "application/json", strings.NewReader(`{"username":"admin","password":"pw"}`)); ok.StatusCode != http.StatusOK {
		t.Fatalf("login should be 200, got %d", ok.StatusCode)
	}
	resp, _ := jc.Post(srv.URL+"/api/auth/change-password", "application/json", strings.NewReader(body))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("change-password with session should be 200, got %d", resp.StatusCode)
	}
}
