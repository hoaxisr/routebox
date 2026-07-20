package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/config"
)

// newSingboxAWGHandler reuses the shared AWG harness (noop runner, seeded peer
// knownPub, real settings with public_host) and flips the Manager to the singbox
// backend, mounting the /singbox export route plus enable/disable for the
// draft-guard tests.
func newSingboxAWGHandler(t *testing.T) (*Handler, http.Handler) {
	t.Helper()
	h, _ := newAWGTestHandler(t)
	h.awg.SetBackend("singbox")
	r := chi.NewRouter()
	r.Route("/api/awg", func(r chi.Router) {
		r.Get("/peers/{publicKey}/singbox", h.GetAWGPeerSingbox)
		r.Post("/enable", h.EnableAWG)
		r.Post("/disable", h.DisableAWG)
		r.Post("/peers", h.CreateAWGPeer)
		r.Delete("/peers/{publicKey}", h.DeleteAWGPeer)
		r.Patch("/peers/{publicKey}/expiry", h.SetAWGPeerExpiry)
	})
	return h, r
}

func TestGetAWGPeerSingbox(t *testing.T) {
	_, r := newSingboxAWGHandler(t)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/singbox", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// Envelope for the frontend request<T> helper + a client endpoint payload.
	if !strings.Contains(body, `"success":true`) {
		t.Fatalf("body not enveloped: %s", body)
	}
	if !strings.Contains(body, `"type":"awg"`) {
		t.Fatalf("body missing type awg: %s", body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("missing Cache-Control: no-store")
	}
	// The export carries the CLIENT's own secret but never the server's.
	if strings.Contains(body, serverPriv) {
		t.Fatalf("export leaked the SERVER private key:\n%s", body)
	}
}

func TestGetAWGPeerSingboxStatusOrder(t *testing.T) {
	h, r := newSingboxAWGHandler(t)

	// Unknown-but-valid pubkey -> 404 (existence before the public-host 503).
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+validButUnknown+"/singbox", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown pubkey = %d; want 404", rec.Code)
	}

	// Invalid key -> 400 before anything else.
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/not-a-key/singbox", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad pubkey = %d; want 400", rec.Code)
	}

	// Known peer, public_host unset -> 503.
	if err := h.settings.Update(map[string]interface{}{"server.public_host": ""}); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/singbox", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no public_host = %d; want 503", rec.Code)
	}
}

// Hardening B: on the singbox backend a pending config draft must block
// enable/disable with 409 BEFORE the orchestrator runs — otherwise
// SyncAwgEndpointActive silently defers while enabled/phase flip anyway.
func TestEnableAWGSingboxPendingDraftIs409(t *testing.T) {
	h, r := newSingboxAWGHandler(t)
	cm := config.NewEmptyManager(filepath.Join(t.TempDir(), "config.json"))
	if err := cm.EnsureDraft(); err != nil {
		t.Fatal(err)
	}
	h.config = cm

	for _, path := range []string{"/api/awg/enable", "/api/awg/disable"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		if rec.Code != http.StatusConflict {
			t.Fatalf("%s with pending draft = %d; want 409; body=%q", path, rec.Code, rec.Body.String())
		}
	}
}

// Same guard for the PEER ops: on the singbox backend they rewrite the ACTIVE
// config too, and SyncAwgEndpointActive silently defers while a draft is pending
// — the handler would report success for a peer that is not live. 409 BEFORE the
// op, same message as enable/disable.
func TestAWGPeerOpsSingboxPendingDraftIs409(t *testing.T) {
	h, r := newSingboxAWGHandler(t)
	cm := config.NewEmptyManager(filepath.Join(t.TempDir(), "config.json"))
	if err := cm.EnsureDraft(); err != nil {
		t.Fatal(err)
	}
	h.config = cm

	cases := []struct{ method, path, body string }{
		{http.MethodPost, "/api/awg/peers", `{"name":"drafty"}`},
		{http.MethodDelete, "/api/awg/peers/" + knownPub, ""},
		{http.MethodPatch, "/api/awg/peers/" + knownPub + "/expiry", `{"expires_at":0}`},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if rec.Code != http.StatusConflict {
			t.Fatalf("%s %s with pending draft = %d; want 409; body=%q",
				tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

// Kernel backend is NOT draft-guarded: peer ops touch awg-quick/.conf, not the
// sing-box config, so a pending draft must not block them.
func TestAWGPeerOpsKernelIgnoreDraft(t *testing.T) {
	h, r := newAWGTestHandler(t) // kernel backend
	cm := config.NewEmptyManager(filepath.Join(t.TempDir(), "config.json"))
	if err := cm.EnsureDraft(); err != nil {
		t.Fatal(err)
	}
	h.config = cm

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/awg/peers", strings.NewReader(`{"name":"kern"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("kernel create with pending draft = %d; want 200; body=%q", rec.Code, rec.Body.String())
	}
}
