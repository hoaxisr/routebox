package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/config"
	"routebox/backend/internal/users"
)

// newConfigWithVless writes a config.json with one vless server inbound and
// returns a config.Manager loaded from it.
func newConfigWithVless(t *testing.T) (*config.Manager, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := `{
	  "inbounds": [
	    {"type":"vless","tag":"vless-in","listen_port":443,
	     "tls":{"enabled":true,"reality":{"enabled":true,"private_key":"` + testRealityPriv + `","short_id":"aa"}},
	     "users":[{"name":"alice","uuid":"u-1","flow":"xtls-rprx-vision"}]}
	  ],
	  "outbounds": [{"type":"direct","tag":"direct"}]
	}`
	if err := os.WriteFile(path, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := config.NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	return m, dir
}

// testRealityPriv is a valid base64url x25519 private key for share-link tests.
// Use the value emitted by `amnezia-box generate reality-keypair` (any valid
// 32-byte key works). If reality-keypair derivation is unavailable in the dev
// env, change the fixture inbound's TLS to {"enabled":true} (plain TLS, no
// reality) so the link builder needs no key derivation; the test still proves
// "link by ID against active".
const testRealityPriv = "MIGe2NoiJrINuvUbVAdMaLaG7HJaqK4ze6mefxpVxh8"

func newUsersTestHandler(t *testing.T) (*Handler, *config.Manager, *users.Manager) {
	cfg, dir := newConfigWithVless(t)
	um := users.NewManager(filepath.Join(dir, "users.toml"))
	if _, err := um.Reconcile(cfg.GetActive()); err != nil {
		t.Fatal(err)
	}
	h := &Handler{config: cfg}
	h.SetUsers(um)
	return h, cfg, um
}

func TestGetUsersReturnsUnifiedList(t *testing.T) {
	h, cfg, _ := newUsersTestHandler(t)
	// Stage a NEW vless user into the draft only (not yet applied).
	if err := cfg.EnsureDraft(); err != nil {
		t.Fatal(err)
	}
	ib, _ := cfg.GetInbound("vless-in")
	ib["users"] = append(ib["users"].([]interface{}),
		map[string]interface{}{"name": "bob", "uuid": "u-2"})
	if err := cfg.UpdateInbound("vless-in", ib); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Get("/api/users", h.ListUsers)
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	// KEEP A's choice: ONE unified list; pending entries carry pending:true + empty id.
	var resp struct {
		Data []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Pending bool   `json:"pending"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 1 registry + 1 pending = 2, got %d: %s", len(resp.Data), w.Body.String())
	}
	var pending, registered int
	for _, u := range resp.Data {
		if u.Pending {
			pending++
			if u.ID != "" {
				t.Fatalf("pending user must have empty id, got %q", u.ID)
			}
		} else {
			registered++
			if u.ID == "" {
				t.Fatalf("registry user must have an id")
			}
		}
	}
	if pending != 1 || registered != 1 {
		t.Fatalf("want 1 pending + 1 registry, got pending=%d registered=%d", pending, registered)
	}
}

func TestCreateUserStagesDraft(t *testing.T) {
	h, cfg, _ := newUsersTestHandler(t)
	r := chi.NewRouter()
	r.Post("/api/users", h.CreateUser)
	body := `{"name":"bob","protocol":"vless","inbound_tag":"vless-in"}`
	req := httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	// Draft now has 2 users; active still has 1 (registry untouched).
	ib, _ := cfg.GetInbound("vless-in")
	us, _ := ib["users"].([]interface{})
	if len(us) != 2 {
		t.Fatalf("draft inbound should have 2 users, got %d", len(us))
	}
}

func TestGetUserLinkByID(t *testing.T) {
	h, _, um := newUsersTestHandler(t)
	id := um.List()[0].ID
	r := chi.NewRouter()
	r.Get("/api/users/{id}/link", h.GetUserLinkByID)
	req := httptest.NewRequest(http.MethodGet,
		"/api/users/"+id+"/link?tag=vless-in&host=vpn.example.com", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Link string `json:"link"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resp.Data.Link, "vless://u-1@vpn.example.com:443") {
		t.Fatalf("unexpected link: %q", resp.Data.Link)
	}
}

func TestGetUserLinkByIDUnknown(t *testing.T) {
	h, _, _ := newUsersTestHandler(t)
	r := chi.NewRouter()
	r.Get("/api/users/{id}/link", h.GetUserLinkByID)
	req := httptest.NewRequest(http.MethodGet, "/api/users/nope/link?tag=vless-in&host=h", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDeleteUserRemovesFromDraft(t *testing.T) {
	h, cfg, um := newUsersTestHandler(t)
	id := um.List()[0].ID
	r := chi.NewRouter()
	r.Delete("/api/users/{id}", h.DeleteUser)
	req := httptest.NewRequest(http.MethodDelete, "/api/users/"+id, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	ib, _ := cfg.GetInbound("vless-in")
	us, _ := ib["users"].([]interface{})
	if len(us) != 0 {
		t.Fatalf("draft inbound should have 0 users after delete, got %d", len(us))
	}
}
