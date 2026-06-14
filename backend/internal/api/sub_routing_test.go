package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/auth"
	"routebox/backend/internal/config"
	"routebox/backend/internal/settings"
	"routebox/backend/internal/users"
)

// TestSub_IsOutsideAuth mirrors main.go's router shape: a PROTECTED /api group
// behind AuthMiddleware and a PUBLIC /sub sibling with NO middleware. With auth
// ENABLED, /api/users must 401 but /sub/{token} must succeed WITHOUT any panel
// credentials — proving /sub is genuinely outside the auth seam.
func TestSub_IsOutsideAuth(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	const cfg = `{"inbounds":[
	  {"type":"vless","tag":"vless-in","listen_port":443,
	   "tls":{"enabled":true,"server_name":"vpn.example.com"},
	   "users":[{"name":"alice","uuid":"u-1","flow":"xtls-rprx-vision"}]}
	],"outbounds":[{"type":"direct","tag":"direct"}]}`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	cfgMgr, err := config.NewManager(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	um := users.NewManager(filepath.Join(dir, "users.toml"))
	if _, err := um.Reconcile(cfgMgr.GetActive()); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(dir, "routebox.toml")
	if err := os.WriteFile(settingsPath, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	sm, err := settings.NewManager(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := sm.Load(); err != nil {
		t.Fatal(err)
	}
	// Enable auth so the protected group returns 401 without creds, and set the
	// public host so /sub can build links.
	if err := sm.Update(map[string]interface{}{
		"server.public_host":     "vpn.example.com",
		"security.auth_enabled":  true,
		"security.auth_username": "admin",
		"security.auth_password": "correct-horse-battery-staple",
	}); err != nil {
		t.Fatal(err)
	}

	h := &Handler{config: cfgMgr, settings: sm}
	h.SetUsers(um)
	h.SetSubLimiter(auth.NewLimiter())
	sessions := auth.NewSessionStore()
	limiter := auth.NewLimiter()
	verifier := auth.NewCachedVerifier()
	h.SetAuth(sessions, limiter, verifier)

	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware(sm, sessions, limiter, verifier))
			r.Get("/users", h.ListUsers)
		})
	})
	r.Get("/sub/{token}", h.GetSubscription)

	// Protected /api/users with no creds → 401.
	apiRec := httptest.NewRecorder()
	r.ServeHTTP(apiRec, httptest.NewRequest(http.MethodGet, "/api/users", nil))
	if apiRec.Code != http.StatusUnauthorized {
		t.Fatalf("/api/users with no creds = %d, want 401", apiRec.Code)
	}

	// Public /sub with no creds → 200.
	tok := um.List()[0].Token
	sub := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sub/"+tok, nil)
	req.RemoteAddr = "203.0.113.9:1234"
	r.ServeHTTP(sub, req)
	if sub.Code != http.StatusOK {
		t.Fatalf("/sub with no creds = %d, want 200 (must be outside auth)", sub.Code)
	}
}

// TestTokenMutationRoutesRequireAuth is the symmetric counterpart to
// TestSub_IsOutsideAuth: the credential-mutating endpoints
// POST /api/users/{id}/token/rotate and DELETE /api/users/{id}/token are
// registered INSIDE the AuthMiddleware group in main.go. This pins that shape so
// a future refactor that moved them to the public group (silently exposing an
// unauthenticated token-rotation endpoint) fails loudly. With auth ENABLED, both
// routes must 401 WITHOUT panel credentials (auth rejects before the handler
// runs), and must NOT 401 WITH valid credentials (proving the 401 came from auth,
// not a routing miss).
func TestTokenMutationRoutesRequireAuth(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	const cfg = `{"inbounds":[
	  {"type":"vless","tag":"vless-in","listen_port":443,
	   "tls":{"enabled":true,"server_name":"vpn.example.com"},
	   "users":[{"name":"alice","uuid":"u-1","flow":"xtls-rprx-vision"}]}
	],"outbounds":[{"type":"direct","tag":"direct"}]}`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	cfgMgr, err := config.NewManager(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	um := users.NewManager(filepath.Join(dir, "users.toml"))
	if _, err := um.Reconcile(cfgMgr.GetActive()); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(dir, "routebox.toml")
	if err := os.WriteFile(settingsPath, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	sm, err := settings.NewManager(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := sm.Load(); err != nil {
		t.Fatal(err)
	}
	// Auth ENABLED with real credentials — otherwise the 401 assertions below are
	// vacuous (AuthMiddleware passes everything through when auth is off).
	const (
		username = "admin"
		password = "correct-horse-battery-staple"
	)
	if err := sm.Update(map[string]interface{}{
		"server.public_host":     "vpn.example.com",
		"security.auth_enabled":  true,
		"security.auth_username": username,
		"security.auth_password": password,
	}); err != nil {
		t.Fatal(err)
	}

	h := &Handler{config: cfgMgr, settings: sm}
	h.SetUsers(um)
	h.SetSubLimiter(auth.NewLimiter())
	sessions := auth.NewSessionStore()
	limiter := auth.NewLimiter()
	verifier := auth.NewCachedVerifier()
	h.SetAuth(sessions, limiter, verifier)

	// Replicate main.go's shape: protected /api group with the /users route tree
	// wiring rotate/revoke at the EXACT paths main.go uses.
	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware(sm, sessions, limiter, verifier))
			r.Route("/users", func(r chi.Router) {
				r.Post("/{id}/token/rotate", h.RotateUserToken)
				r.Delete("/{id}/token", h.RevokeUserToken)
			})
		})
	})

	cases := []struct {
		name, method, path string
	}{
		{"rotate", http.MethodPost, "/api/users/some-id/token/rotate"},
		{"revoke", http.MethodDelete, "/api/users/some-id/token"},
	}

	// WITHOUT credentials → 401 (auth must reject before the handler runs; not
	// 200, not 404-unknown-id).
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.RemoteAddr = "203.0.113.9:1234"
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s with no creds = %d, want 401 (must be inside auth)", tc.method, tc.path, rec.Code)
		}
	}

	// WITH valid credentials → NOT 401 (the request reaches the handler, which
	// 404s the unknown id). This proves the 401 above came from auth, not routing.
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.RemoteAddr = "203.0.113.9:1234"
		req.SetBasicAuth(username, password)
		r.ServeHTTP(rec, req)
		if rec.Code == http.StatusUnauthorized {
			t.Fatalf("%s %s with valid creds = 401, want non-401 (handler should run)", tc.method, tc.path)
		}
	}
}
