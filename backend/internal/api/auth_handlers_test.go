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
