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
	// Assert EXACT header values, not mere presence (a header-shape regression
	// — wrong charset, missing no-store, a mangled filename — must fail here).
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want %q", ct, "text/plain; charset=utf-8")
	}
	if pui := rec.Header().Get("Profile-Update-Interval"); pui != "12" {
		t.Fatalf("Profile-Update-Interval = %q, want %q", pui, "12")
	}
	// alice's name sanitizes to "alice"; the FULL disposition incl. filename.
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="alice"` {
		t.Fatalf("Content-Disposition = %q, want %q", cd, `attachment; filename="alice"`)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q, want %q", cc, "no-store")
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
	// Headers must be byte-identical too: a header-based oracle (e.g. a header
	// present on one path but not the other) would defeat the identical 404.
	uh, rh := unknown.Header(), revoked.Header()
	if len(uh) != len(rh) {
		t.Fatalf("anti-enumeration header oracle: unknown headers %v != revoked %v", uh, rh)
	}
	for k, uv := range uh {
		rv, ok := rh[k]
		if !ok || strings.Join(uv, ",") != strings.Join(rv, ",") {
			t.Fatalf("anti-enumeration header oracle on %q: unknown %v != revoked %v", k, uv, rv)
		}
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

// TestSanitizeFilename_RejectsHeaderInjection pins the header-injection-safety
// property: whatever the user name, the emitted filename must never contain a
// double-quote (which would close the quoted-string), a CR/LF (which would inject
// a new header / split the response), or a NUL. It also pins the documented
// fallbacks and that a benign name survives intact.
func TestSanitizeFilename_RejectsHeaderInjection(t *testing.T) {
	hostile := []string{
		`a"b`,                  // double-quote: would terminate filename="..."
		"a\r\nSet-Cookie: x=y", // CRLF header injection
		"a\x00b",               // NUL
		"../../etc",            // path traversal
		"\"\r\n\x00",           // every dangerous byte at once
	}
	for _, in := range hostile {
		out := sanitizeFilename(in)
		for _, bad := range []string{"\"", "\r", "\n", "\x00"} {
			if strings.Contains(out, bad) {
				t.Fatalf("sanitizeFilename(%q) = %q leaks %q", in, out, bad)
			}
		}
	}

	// All-illegal input and the empty string fall back to "subscription".
	for _, in := range []string{"", "\"\r\n", "///", "@@@"} {
		if out := sanitizeFilename(in); out != "subscription" {
			t.Fatalf("sanitizeFilename(%q) = %q, want fallback %q", in, out, "subscription")
		}
	}

	// A normal name is preserved verbatim (no over-zealous mangling).
	if out := sanitizeFilename("alice-1.txt"); out != "alice-1.txt" {
		t.Fatalf("sanitizeFilename(%q) = %q, want it preserved", "alice-1.txt", out)
	}
}

// setUserLifecycle mutates the single reconciled user's Enabled/ExpiresAt and
// persists them, preserving the auto-minted token.
func setUserLifecycle(t *testing.T, um *users.Manager, enabled bool, expiresAt int64) {
	t.Helper()
	u := um.List()[0]
	u.Enabled = enabled
	u.ExpiresAt = expiresAt
	if err := um.Put(&u); err != nil {
		t.Fatal(err)
	}
}

func TestSub_InactiveDisabled_EmptyBodyWithExpire(t *testing.T) {
	h, um := newSubHandler(t, "vpn.example.com")
	tok := tokenOf(t, um)
	setUserLifecycle(t, um, false, 1735689600) // disabled, definite expire to assert

	rec := serveSub(h, tok, "203.0.113.10")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("inactive user body must be empty, got %q", rec.Body.String())
	}
	ui := rec.Header().Get("Subscription-Userinfo")
	if !strings.Contains(ui, "expire=1735689600") {
		t.Fatalf("Subscription-Userinfo = %q, want expire=1735689600", ui)
	}
	// Standard headers still present (clients must parse it as a valid sub).
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/plain; charset=utf-8", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cc)
	}
}

func TestSub_InactiveExpired_EmptyBody(t *testing.T) {
	h, um := newSubHandler(t, "vpn.example.com")
	tok := tokenOf(t, um)
	setUserLifecycle(t, um, true, 1) // enabled but expired (1970)

	rec := serveSub(h, tok, "203.0.113.11")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expired user body must be empty, got %q", rec.Body.String())
	}
}

func TestSub_Active_FullBody(t *testing.T) {
	h, um := newSubHandler(t, "vpn.example.com")
	tok := tokenOf(t, um)
	setUserLifecycle(t, um, true, 0) // active, never expires

	rec := serveSub(h, tok, "203.0.113.12")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() == 0 {
		t.Fatal("active user must get a non-empty subscription body")
	}
}

// TestSub_EmptySubscription_Returns200NotError proves a valid-token user whose
// bindings resolve to NO active nodes is served 200 with an empty (base64 of "")
// body — the PUBLIC endpoint must never 500 on a buildable-empty user.
func TestSub_EmptySubscription_Returns200NotError(t *testing.T) {
	h, um := newSubHandler(t, "vpn.example.com")
	// Take the reconciled tokened user and rewrite its bindings to point at an
	// inbound tag that is ABSENT from active. BuildSubscription skips every stale
	// binding → base64 of "" rather than an error.
	u := um.List()[0]
	tok := u.Token
	u.Bindings = []users.Binding{{
		InboundTag: "gone-from-active",
		Credential: "u-1",
		Protocol:   "vless",
	}}
	if err := um.Put(&u); err != nil {
		t.Fatal(err)
	}

	rec := serveSub(h, tok, "203.0.113.9")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", rec.Code, rec.Body.String())
	}
	// Empty subscription == base64 of "" == "".
	if got := rec.Body.String(); got != "" {
		t.Fatalf("body = %q, want empty (base64 of \"\")", got)
	}
}
