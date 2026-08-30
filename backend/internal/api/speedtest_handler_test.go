package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/config"
	"routebox/backend/internal/process"
)

// speedTestRouter wires the two routes exactly as main.go does, so a typo in
// either path shows up here rather than as a 404 in the panel. The endpoints one
// is #92: the same handler, reached by an honest URL.
func speedTestRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Post("/api/outbounds/{tag}/speedtest", h.SpeedTestOutbound)
	r.Post("/api/endpoints/{tag}/speedtest", h.SpeedTestOutbound)
	return r
}

// Without a config file there is nothing to measure against, and that is ours,
// not the network's: it must not read as a failed measurement.
func TestSpeedTest_NoConfigPath(t *testing.T) {
	h := &Handler{config: &config.Manager{}, process: &process.Manager{}}
	for _, path := range []string{"/api/outbounds/wg/speedtest", "/api/endpoints/wg/speedtest"} {
		rec := httptest.NewRecorder()
		speedTestRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: status = %d, want 503; body=%q", path, rec.Code, rec.Body.String())
		}
	}
}

// Both routes have to resolve to the handler and carry the tag through intact —
// including a tag the client had to percent-encode. Reaching "no binary" proves
// the request got past routing and param decoding; it is the deterministic
// failure of a machine with no amnezia-box installed.
func TestSpeedTest_BothRoutesDecodeTheTag(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.NewManager(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{config: cfg, process: &process.Manager{}}

	for _, path := range []string{
		"/api/outbounds/my%20wg/speedtest",
		"/api/endpoints/my%20wg/speedtest",
	} {
		rec := httptest.NewRecorder()
		speedTestRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("%s: status = %d, want 500; body=%q", path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "amnezia-box binary") {
			t.Fatalf("%s: body %q should name the missing binary", path, rec.Body.String())
		}
	}
}
