package api

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/auth"
	"routebox/backend/internal/settings"
	"routebox/backend/internal/users"
)

// newSubHandler reuses newConfigWithVless (a Reality vless inbound whose
// private_key is a VALID derivable key — so BuildShareLink succeeds — and which
// doubles as the no-secrets fixture), a reconciled registry (so alice has an
// auto-minted token), settings with a public host, and a fresh sub limiter.
func newSubHandler(t *testing.T, publicHost string) (*Handler, *users.Manager) {
	t.Helper()
	cfg, dir := newConfigWithVless(t)
	um := users.NewManager(filepath.Join(dir, "users.toml"))
	if _, err := um.Reconcile(cfg.GetActive()); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(dir, "routebox.toml")
	// Load() decodes the file directly and surfaces a "no such file" error, so a
	// fresh fixture needs the file to exist first.
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
	h := &Handler{config: cfg, settings: sm}
	h.SetUsers(um)
	h.SetSubLimiter(auth.NewLimiter())
	return h, um
}

func tokenOf(t *testing.T, um *users.Manager) string {
	t.Helper()
	list := um.List()
	if len(list) != 1 || list[0].Token == "" {
		t.Fatalf("expected one tokened user, got %+v", list)
	}
	return list[0].Token
}

func serveSub(h *Handler, token, ip string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Get("/sub/{token}", h.GetSubscription)
	req := httptest.NewRequest(http.MethodGet, "/sub/"+token, nil)
	if ip != "" {
		req.RemoteAddr = ip + ":12345"
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestSub_OK_ReturnsBase64WithHeaders(t *testing.T) {
	h, um := newSubHandler(t, "vpn.example.com")
	rec := serveSub(h, tokenOf(t, um), "203.0.113.1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("Content-Type = %q", ct)
	}
	if rec.Header().Get("Profile-Update-Interval") == "" {
		t.Fatal("missing Profile-Update-Interval header")
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	raw, err := base64.StdEncoding.DecodeString(rec.Body.String())
	if err != nil {
		t.Fatalf("body is not std base64: %v", err)
	}
	if !strings.HasPrefix(string(raw), "vless://") {
		t.Fatalf("decoded body not a vless link: %q", raw)
	}
}

func TestSub_NoSecretsInBody(t *testing.T) {
	h, um := newSubHandler(t, "vpn.example.com")
	rec := serveSub(h, tokenOf(t, um), "203.0.113.1")
	raw, _ := base64.StdEncoding.DecodeString(rec.Body.String())
	// testRealityPriv is the inbound's Reality private_key fixture; the share link
	// must emit only the DERIVED public key (pbk), never the private key.
	if strings.Contains(string(raw), testRealityPriv) {
		t.Fatal("Reality private key leaked into subscription body")
	}
}

func TestSub_UnknownAndRevoked_IdenticalNotFound(t *testing.T) {
	h, um := newSubHandler(t, "vpn.example.com")
	// Unknown token.
	unknown := serveSub(h, "totally-unknown-token", "203.0.113.1")
	// Revoke the real user, then hit the (now dead) token.
	live := tokenOf(t, um)
	id := um.List()[0].ID
	if err := um.RevokeToken(id); err != nil {
		t.Fatal(err)
	}
	revoked := serveSub(h, live, "203.0.113.2")

	if unknown.Code != http.StatusNotFound || revoked.Code != http.StatusNotFound {
		t.Fatalf("want 404/404, got %d/%d", unknown.Code, revoked.Code)
	}
	if unknown.Body.String() != revoked.Body.String() {
		t.Fatalf("anti-enumeration broken: unknown body %q != revoked body %q",
			unknown.Body.String(), revoked.Body.String())
	}
}

func TestSub_NoPublicHost_503(t *testing.T) {
	h, um := newSubHandler(t, "") // host unset → handler owns the 503
	rec := serveSub(h, tokenOf(t, um), "203.0.113.1")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestSub_RateLimitedPerIP(t *testing.T) {
	h, um := newSubHandler(t, "vpn.example.com")
	tok := tokenOf(t, um)
	const ip = "198.51.100.7"
	got429 := false
	for i := 0; i < 12; i++ {
		rec := serveSub(h, tok, ip)
		if rec.Code == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Fatal("per-IP flood was never rate-limited (no 429)")
	}
	// A different IP is still served (rate-limit is per-IP). Valid token + fresh IP.
	other := serveSub(h, tok, "198.51.100.8")
	if other.Code != http.StatusOK {
		t.Fatalf("unrelated IP got %d, want 200 (rate-limit is per-IP)", other.Code)
	}
}
