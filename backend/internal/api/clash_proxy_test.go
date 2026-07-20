package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"routebox/backend/internal/config"
)

// newClashProxyHandler builds a Handler whose config carries a clash_api block
// pointing at addr, with an optional secret. The external_controller in config
// makes getClashAddr return addr without needing a running process.
func newClashProxyHandler(t *testing.T, addr, secret string) *Handler {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	secretField := ""
	if secret != "" {
		secretField = `,"secret":"` + secret + `"`
	}
	cfg := `{
	  "experimental": {"clash_api": {"external_controller": "` + addr + `"` + secretField + `}},
	  "outbounds": [{"type":"direct","tag":"direct"}]
	}`
	if err := os.WriteFile(path, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := config.NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	return &Handler{config: m}
}

// fakeClash starts an httptest server recording the Authorization header of the
// last request it saw. The returned getter is mutex-guarded so tests reading it
// from the test goroutine are race-free.
func fakeClash(t *testing.T) (*httptest.Server, func() string) {
	t.Helper()
	var mu sync.Mutex
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotAuth = r.Header.Get("Authorization")
		mu.Unlock()
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return srv, func() string {
		mu.Lock()
		defer mu.Unlock()
		return gotAuth
	}
}

func TestGetClashSecret(t *testing.T) {
	t.Run("absent secret returns empty", func(t *testing.T) {
		h := newClashProxyHandler(t, "127.0.0.1:9090", "")
		if got := h.getClashSecret(); got != "" {
			t.Fatalf("getClashSecret() = %q, want empty", got)
		}
	})
	t.Run("configured secret returned", func(t *testing.T) {
		h := newClashProxyHandler(t, "127.0.0.1:9090", "s3cr3t")
		if got := h.getClashSecret(); got != "s3cr3t" {
			t.Fatalf("getClashSecret() = %q, want %q", got, "s3cr3t")
		}
	})
	t.Run("nil-safe with no config manager sections", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		if err := os.WriteFile(path, []byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`), 0644); err != nil {
			t.Fatal(err)
		}
		m, err := config.NewManager(path)
		if err != nil {
			t.Fatal(err)
		}
		h := &Handler{config: m}
		if got := h.getClashSecret(); got != "" {
			t.Fatalf("getClashSecret() = %q, want empty when experimental block absent", got)
		}
	})
}

// ProxyClashAPI must authenticate upstream with the configured Clash API secret
// even though the browser's own Authorization header is stripped.
func TestProxyClashAPI_ForwardsConfiguredSecret(t *testing.T) {
	srv, gotAuth := fakeClash(t)
	addr := strings.TrimPrefix(srv.URL, "http://")
	h := newClashProxyHandler(t, addr, "s3cr3t")

	req := httptest.NewRequest(http.MethodGet, "/api/clash/proxies", nil)
	req.Header.Set("Authorization", "Bearer panel-session-token") // browser creds
	rec := httptest.NewRecorder()
	h.ProxyClashAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if got := gotAuth(); got != "Bearer s3cr3t" {
		t.Fatalf("upstream Authorization = %q, want %q", got, "Bearer s3cr3t")
	}
}

// With no secret configured, the browser's Authorization must still be stripped
// and nothing substituted — the upstream sees no Authorization at all.
func TestProxyClashAPI_NoSecretSendsNoAuth(t *testing.T) {
	srv, gotAuth := fakeClash(t)
	addr := strings.TrimPrefix(srv.URL, "http://")
	h := newClashProxyHandler(t, addr, "")

	req := httptest.NewRequest(http.MethodGet, "/api/clash/proxies", nil)
	req.Header.Set("Authorization", "Bearer panel-session-token")
	req.Header.Set("Cookie", "session=abc")
	rec := httptest.NewRecorder()
	h.ProxyClashAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if got := gotAuth(); got != "" {
		t.Fatalf("upstream Authorization = %q, want none", got)
	}
}

// The HTTP-fallback branch of ProxyClashWebSocket must behave like ProxyClashAPI:
// forward the configured secret upstream. (The real WS dial leg carries the same
// header via the dialer; covered by code inspection, not unit-testable here.)
func TestProxyClashWebSocket_HTTPFallbackForwardsSecret(t *testing.T) {
	srv, gotAuth := fakeClash(t)
	addr := strings.TrimPrefix(srv.URL, "http://")
	h := newClashProxyHandler(t, addr, "s3cr3t")

	req := httptest.NewRequest(http.MethodGet, "/api/clash/connections", nil) // no Upgrade header -> fallback
	req.Header.Set("Authorization", "Bearer panel-session-token")
	rec := httptest.NewRecorder()
	h.ProxyClashWebSocket(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if got := gotAuth(); got != "Bearer s3cr3t" {
		t.Fatalf("upstream Authorization = %q, want %q", got, "Bearer s3cr3t")
	}
}
