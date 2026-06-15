package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/auth"
	"routebox/backend/internal/awg"
	"routebox/backend/internal/settings"
)

// Real 32-byte std-base64 keys (decode to exactly 32 bytes, so ValidatePublicKey
// accepts them as path params). The plan's literal "GOODPUB==" does NOT decode to
// 32 bytes, so it would 400 at the path-validation gate — these are the canonical
// fixtures.
const (
	knownPub  = "Yluwfrt+6ChDx8TJcdmDuw63AdoQDqA18LMVPr5b4Ks="
	knownPriv = "v4lHbWpEt1SZkmHfwXDcsFHhdyOTXCcZP0TiYysL1Qs="
	knownPSK  = "Q2hlY2tUaGlzUFNLSXNUaGlydHlUd29CeXRlc0xuZz0="
	// serverPriv: any 32-byte std-base64 is a valid x25519 private key.
	serverPriv      = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEs="
	validButUnknown = "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI="
)

// awgNoopRunner satisfies awg.Runner without touching the system: Status() and
// AddPeer's `awg set` calls return empty output / nil error so the interface looks
// down and peer adds succeed.
type awgNoopRunner struct{}

func (awgNoopRunner) Run(_ context.Context, _ string, _ ...string) (string, string, error) {
	return "", "", nil
}

// newAWGTestHandler wires a Handler with an awg.Manager (noop runner + a seeded
// peer "knownPub" whose secret is known) and a real settings.Manager. Returns the
// handler and a chi router mounting the /api/awg routes (no auth — the auth gate
// is exercised separately by TestAWGRoutesRequireAuth).
func newAWGTestHandler(t *testing.T) (*Handler, http.Handler) {
	t.Helper()
	dir := t.TempDir()
	awgDir := filepath.Join(dir, "amneziawg")
	if err := os.MkdirAll(awgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Seed a minimal server .conf so subnet/used reads find a file.
	if err := os.WriteFile(filepath.Join(awgDir, "awg-rb0.conf"),
		[]byte("[Interface]\nListenPort = 51820\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := awg.NewManagerForTest(awgNoopRunner{}, awgDir, serverPriv, awg.Config{
		Iface:      "awg-rb0",
		Subnet:     "10.10.0.0/24",
		ServerIP:   "10.10.0.1",
		ListenPort: 51820,
		MTU:        1420,
		DNS:        []string{"1.1.1.1"},
	})
	if err := m.Store().Put(awg.Peer{
		PublicKey:    knownPub,
		PrivateKey:   knownPriv,
		PresharedKey: knownPSK,
		Address:      "10.10.0.2/32",
		Name:         "phone",
	}); err != nil {
		t.Fatal(err)
	}

	sm := newAWGSettings(t, dir, "vpn.example.com")
	h := &Handler{settings: sm}
	h.SetAWG(m)

	r := chi.NewRouter()
	r.Route("/api/awg", func(r chi.Router) {
		r.Get("/status", h.GetAWGStatus)
		r.Post("/enable", h.EnableAWG)
		r.Post("/disable", h.DisableAWG)
		r.Get("/peers", h.ListAWGPeers)
		r.Post("/peers", h.CreateAWGPeer)
		r.Delete("/peers/{publicKey}", h.DeleteAWGPeer)
		r.Get("/peers/{publicKey}/config", h.GetAWGPeerConfig)
	})
	return h, r
}

func newAWGSettings(t *testing.T, dir, publicHost string) *settings.Manager {
	t.Helper()
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
	if publicHost != "" {
		if err := sm.Update(map[string]interface{}{"server.public_host": publicHost}); err != nil {
			t.Fatal(err)
		}
	}
	return sm
}

func TestAWGConfigEndpointStatusCodes(t *testing.T) {
	h, r := newAWGTestHandler(t)

	// 1) {publicKey} not base64 -> 400 before any lookup (no traversal).
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/..%2Fetc%2Fpasswd/config", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("non-base64 pubkey = %d; want 400", rec.Code)
	}

	// 2) valid-shape but unknown pubkey -> 404 (existence before host).
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+validButUnknown+"/config", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown pubkey = %d; want 404", rec.Code)
	}

	// 3) known peer but public_host unset -> 503 (existence already passed).
	if err := h.settings.Update(map[string]interface{}{"server.public_host": ""}); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/config", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no public_host = %d; want 503", rec.Code)
	}

	// 4) known peer + host set -> 200 text/plain, no-store, sanitized filename.
	if err := h.settings.Update(map[string]interface{}{"server.public_host": "vpn.example.com"}); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("config = %d; want 200; body=%q", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("Content-Type = %q", ct)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("missing no-store")
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="phone.conf"` {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	if !strings.Contains(rec.Body.String(), "[Interface]") {
		t.Fatalf("body not a .conf:\n%s", rec.Body.String())
	}
}

// IPv6 public_host must be bracketed in the client Endpoint (BuildClient passes it
// verbatim). A bare-v6 "host:port" would be ambiguous/invalid.
func TestAWGConfigBracketsIPv6Endpoint(t *testing.T) {
	h, r := newAWGTestHandler(t)
	if err := h.settings.Update(map[string]interface{}{"server.public_host": "2001:db8::1"}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("config = %d; want 200; body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Endpoint = [2001:db8::1]:51820") {
		t.Fatalf("IPv6 endpoint not bracketed:\n%s", rec.Body.String())
	}
}

// No private_key/preshared_key of any OTHER peer appears in responses, and the
// /peers summary list carries NO secret at all (mirrors /sub's no-secrets test).
func TestAWGNoSecretsInResponses(t *testing.T) {
	h, r := newAWGTestHandler(t)
	if err := h.settings.Update(map[string]interface{}{"server.public_host": "vpn.example.com"}); err != nil {
		t.Fatal(err)
	}

	// The summary list must contain neither the private nor the preshared key.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers", nil))
	if body := rec.Body.String(); strings.Contains(body, knownPriv) || strings.Contains(body, knownPSK) {
		t.Fatalf("/api/awg/peers leaked a secret:\n%s", body)
	}

	// The create response is also a secret-free summary.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/awg/peers", strings.NewReader(`{"name":"laptop"}`))
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create = %d; want 200; body=%q", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); strings.Contains(body, "private_key") || strings.Contains(body, "preshared_key") {
		t.Fatalf("create response leaked a secret field:\n%s", body)
	}

	// The .conf legitimately contains the CLIENT's own priv/psk; assert it does NOT
	// contain the SERVER's private key (only the derived server pubkey).
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/config", nil))
	if strings.Contains(rec.Body.String(), serverPriv) {
		t.Fatalf("client .conf leaked the SERVER private key:\n%s", rec.Body.String())
	}
}

// All mutating awg routes require auth (mirror TestTokenMutationRoutesRequireAuth):
// they are registered INSIDE the AuthMiddleware group, so WITHOUT panel
// credentials they must 401 before the handler runs.
func TestAWGRoutesRequireAuth(t *testing.T) {
	dir := t.TempDir()
	sm := newAWGSettings(t, dir, "vpn.example.com")
	const (
		username = "admin"
		password = "correct-horse-battery-staple"
	)
	if err := sm.Update(map[string]interface{}{
		"security.auth_enabled":  true,
		"security.auth_username": username,
		"security.auth_password": password,
	}); err != nil {
		t.Fatal(err)
	}

	h := &Handler{settings: sm}
	sessions := auth.NewSessionStore()
	limiter := auth.NewLimiter()
	verifier := auth.NewCachedVerifier()
	h.SetAuth(sessions, limiter, verifier)

	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware(sm, sessions, limiter, verifier))
			r.Route("/awg", func(r chi.Router) {
				r.Post("/enable", h.EnableAWG)
				r.Post("/disable", h.DisableAWG)
				r.Post("/peers", h.CreateAWGPeer)
				r.Delete("/peers/{publicKey}", h.DeleteAWGPeer)
			})
		})
	})

	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/awg/enable"},
		{http.MethodPost, "/api/awg/disable"},
		{http.MethodPost, "/api/awg/peers"},
		{http.MethodDelete, "/api/awg/peers/" + knownPub},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.RemoteAddr = "203.0.113.9:1234"
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s = %d; want 401 (must be inside the auth group)", tc.method, tc.path, rec.Code)
		}
	}
}
