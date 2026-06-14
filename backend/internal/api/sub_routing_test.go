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
