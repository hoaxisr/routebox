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
	"routebox/backend/internal/settings"
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
	h, cfg, um := newUsersTestHandler(t)
	// Registry starts with exactly the 1 reconciled active user (alice).
	before := len(um.List())
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
	// CreateUser must NOT write the registry (it materializes only on Apply/reconcile).
	if after := len(um.List()); after != before {
		t.Fatalf("registry must be unchanged by CreateUser: before=%d after=%d", before, after)
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

// newConfigWithTwoVless writes a config.json with two vless server inbounds.
func newConfigWithTwoVless(t *testing.T) (*config.Manager, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := `{
	  "inbounds": [
	    {"type":"vless","tag":"vless-in","listen_port":443,
	     "tls":{"enabled":true,"reality":{"enabled":true,"private_key":"` + testRealityPriv + `","short_id":"aa"}},
	     "users":[{"name":"alice","uuid":"u-1","flow":"xtls-rprx-vision"}]},
	    {"type":"vless","tag":"vless-in-2","listen_port":444,
	     "tls":{"enabled":true,"reality":{"enabled":true,"private_key":"` + testRealityPriv + `","short_id":"bb"}},
	     "users":[]}
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

func TestAddBinding(t *testing.T) {
	cfg, dir := newConfigWithTwoVless(t)
	um := users.NewManager(filepath.Join(dir, "users.toml"))
	if _, err := um.Reconcile(cfg.GetActive()); err != nil {
		t.Fatal(err)
	}
	h := &Handler{config: cfg}
	h.SetUsers(um)

	id := um.List()[0].ID // alice, registered on vless-in only

	r := chi.NewRouter()
	r.Post("/api/users/{id}/bindings", h.AddBinding)

	// Happy path: add alice into the second inbound.
	body := `{"protocol":"vless","inbound_tag":"vless-in-2"}`
	req := httptest.NewRequest(http.MethodPost, "/api/users/"+id+"/bindings", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	ib, _ := cfg.GetInbound("vless-in-2")
	us, _ := ib["users"].([]interface{})
	if len(us) != 1 {
		t.Fatalf("vless-in-2 draft should have 1 user after AddBinding, got %d", len(us))
	}

	// Unknown id -> 404.
	req = httptest.NewRequest(http.MethodPost, "/api/users/nope/bindings", strings.NewReader(body))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown id should be 404, got %d", w.Code)
	}

	// Malformed JSON -> 400.
	req = httptest.NewRequest(http.MethodPost, "/api/users/"+id+"/bindings", strings.NewReader(`{not json`))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("malformed body should be 400, got %d", w.Code)
	}
}

func TestGetUserLinkByIDBadHost(t *testing.T) {
	h, _, um := newUsersTestHandler(t)
	id := um.List()[0].ID
	r := chi.NewRouter()
	r.Get("/api/users/{id}/link", h.GetUserLinkByID)
	req := httptest.NewRequest(http.MethodGet,
		"/api/users/"+id+"/link?tag=vless-in&host=bad_host!!", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid host should be 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetUserLinkByIDFallsBackToPublicHost(t *testing.T) {
	cfg, dir := newConfigWithVless(t)
	um := users.NewManager(filepath.Join(dir, "users.toml"))
	if _, err := um.Reconcile(cfg.GetActive()); err != nil {
		t.Fatal(err)
	}
	sm, err := settings.NewManager(filepath.Join(dir, "routebox.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := sm.Update(map[string]interface{}{"server.public_host": "vpn.example.com"}); err != nil {
		t.Fatal(err)
	}
	h := &Handler{config: cfg, settings: sm}
	h.SetUsers(um)

	id := um.List()[0].ID
	r := chi.NewRouter()
	r.Get("/api/users/{id}/link", h.GetUserLinkByID)
	// No ?host= : must fall back to settings server.public_host.
	req := httptest.NewRequest(http.MethodGet, "/api/users/"+id+"/link?tag=vless-in", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	data := decodeDataBytes(t, w.Body.Bytes())
	link, _ := data["link"].(string)
	if !strings.Contains(link, "vpn.example.com") {
		t.Fatalf("link should use public_host fallback, got %q", link)
	}
}

// TestCreateUserDoesNotMutateActive proves the discard-safety invariant at the
// staging level: POST /api/users must add the user to the draft/working config
// ONLY; the active (on-disk) config must be byte-for-byte unchanged. Regression
// for the live-reference mutation bug (GetInbound returned the active map and the
// handler mutated inbound["users"] in place, corrupting active before EnsureDraft).
func TestCreateUserDoesNotMutateActive(t *testing.T) {
	cfg, dir := newConfigWithVless(t)
	// Drop the seeded user so active starts with ZERO users on the inbound.
	{
		active := cfg.GetActive()
		ib, _ := findActiveInbound(active, "vless-in")
		ib["users"] = []interface{}{}
		if err := cfg.Save(active); err != nil {
			t.Fatal(err)
		}
	}
	um := users.NewManager(filepath.Join(dir, "users.toml"))
	if _, err := um.Reconcile(cfg.GetActive()); err != nil {
		t.Fatal(err)
	}
	h := &Handler{config: cfg}
	h.SetUsers(um)

	r := chi.NewRouter()
	r.Post("/api/users", h.CreateUser)
	body := `{"name":"bob","protocol":"vless","inbound_tag":"vless-in"}`
	req := httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	// Active must be UNTOUCHED: still zero users on the inbound.
	activeIB, ok := findActiveInbound(cfg.GetActive(), "vless-in")
	if !ok {
		t.Fatal("vless-in missing from active config")
	}
	if us, _ := activeIB["users"].([]interface{}); len(us) != 0 {
		t.Fatalf("active inbound must still have 0 users (not mutated), got %d", len(us))
	}

	// Working/draft must show the staged user.
	draftIB, _ := cfg.GetInbound("vless-in")
	if us, _ := draftIB["users"].([]interface{}); len(us) != 1 {
		t.Fatalf("draft inbound must show 1 staged user, got %d", len(us))
	}
}

// TestDiscardRemovesStagedUser reproduces the e2e failure: stage a user via
// POST /api/users, then DiscardDraft. Because staging must not touch active, the
// discard must fully remove the staged user — GetActive shows none, and
// GET /api/users returns it neither as registered nor pending.
func TestDiscardRemovesStagedUser(t *testing.T) {
	cfg, dir := newConfigWithVless(t)
	// Active starts with zero users on the inbound.
	{
		active := cfg.GetActive()
		ib, _ := findActiveInbound(active, "vless-in")
		ib["users"] = []interface{}{}
		if err := cfg.Save(active); err != nil {
			t.Fatal(err)
		}
	}
	um := users.NewManager(filepath.Join(dir, "users.toml"))
	if _, err := um.Reconcile(cfg.GetActive()); err != nil {
		t.Fatal(err)
	}
	h := &Handler{config: cfg}
	h.SetUsers(um)

	r := chi.NewRouter()
	r.Post("/api/users", h.CreateUser)
	r.Get("/api/users", h.ListUsers)

	// Stage a user.
	body := `{"name":"bob","protocol":"vless","inbound_tag":"vless-in"}`
	req := httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("stage status %d: %s", w.Code, w.Body.String())
	}

	// Discard the draft.
	if err := cfg.DiscardDraft(); err != nil {
		t.Fatal(err)
	}

	// Active has no such user (it was never mutated).
	activeIB, _ := findActiveInbound(cfg.GetActive(), "vless-in")
	if us, _ := activeIB["users"].([]interface{}); len(us) != 0 {
		t.Fatalf("after discard, active must have 0 users, got %d", len(us))
	}

	// GET /api/users returns nothing: neither registered nor pending.
	req = httptest.NewRequest(http.MethodGet, "/api/users", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list status %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []struct {
			Name    string `json:"name"`
			Pending bool   `json:"pending"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	for _, u := range resp.Data {
		if u.Name == "bob" {
			t.Fatalf("staged user must be gone after discard, found %+v", u)
		}
	}
}

// TestAddBindingDoesNotMutateActive proves AddBinding stages into the draft
// only: GetActive() for BOTH inbounds is unchanged (the target inbound keeps its
// original users, the source inbound is untouched), while the draft shows the
// new binding.
func TestAddBindingDoesNotMutateActive(t *testing.T) {
	cfg, dir := newConfigWithTwoVless(t)
	um := users.NewManager(filepath.Join(dir, "users.toml"))
	if _, err := um.Reconcile(cfg.GetActive()); err != nil {
		t.Fatal(err)
	}
	h := &Handler{config: cfg}
	h.SetUsers(um)

	id := um.List()[0].ID // alice, registered on vless-in only

	r := chi.NewRouter()
	r.Post("/api/users/{id}/bindings", h.AddBinding)
	body := `{"protocol":"vless","inbound_tag":"vless-in-2"}`
	req := httptest.NewRequest(http.MethodPost, "/api/users/"+id+"/bindings", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	// Active: vless-in-2 must STILL have 0 users (binding only staged in draft).
	active := cfg.GetActive()
	ib2, ok := findActiveInbound(active, "vless-in-2")
	if !ok {
		t.Fatal("vless-in-2 missing from active")
	}
	if us, _ := ib2["users"].([]interface{}); len(us) != 0 {
		t.Fatalf("active vless-in-2 must have 0 users (not mutated), got %d", len(us))
	}
	// Active: vless-in (source) unchanged with its 1 original user.
	ib1, ok := findActiveInbound(active, "vless-in")
	if !ok {
		t.Fatal("vless-in missing from active")
	}
	if us, _ := ib1["users"].([]interface{}); len(us) != 1 {
		t.Fatalf("active vless-in must still have 1 user, got %d", len(us))
	}

	// Draft shows the new binding on vless-in-2.
	draftIB2, _ := cfg.GetInbound("vless-in-2")
	if us, _ := draftIB2["users"].([]interface{}); len(us) != 1 {
		t.Fatalf("draft vless-in-2 must show 1 staged user, got %d", len(us))
	}
}

// TestDeleteUserDoesNotMutateActive proves DELETE only stages the removal in the
// draft: GetActive() still has the user; the draft does not.
func TestDeleteUserDoesNotMutateActive(t *testing.T) {
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

	// Active must STILL have the user (removal only staged in draft).
	activeIB, ok := findActiveInbound(cfg.GetActive(), "vless-in")
	if !ok {
		t.Fatal("vless-in missing from active")
	}
	if us, _ := activeIB["users"].([]interface{}); len(us) != 1 {
		t.Fatalf("active vless-in must still have 1 user (not mutated), got %d", len(us))
	}

	// Draft must show the user removed.
	draftIB, _ := cfg.GetInbound("vless-in")
	if us, _ := draftIB["users"].([]interface{}); len(us) != 0 {
		t.Fatalf("draft vless-in must have 0 users after delete, got %d", len(us))
	}
}

func TestGetUsersPendingDedup(t *testing.T) {
	// alice exists in BOTH active (registry, via Reconcile) AND the draft
	// (same tag+credential). She must appear ONCE, as registered (with id).
	h, cfg, um := newUsersTestHandler(t)
	if err := cfg.EnsureDraft(); err != nil {
		t.Fatal(err)
	}
	// Draft still carries alice (u-1) verbatim from active; no change needed,
	// but force a draft that contains her to exercise the dedup map.
	ib, _ := cfg.GetInbound("vless-in")
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
	if len(resp.Data) != 1 {
		t.Fatalf("alice in both active+draft must appear once, got %d: %s", len(resp.Data), w.Body.String())
	}
	got := resp.Data[0]
	if got.Pending {
		t.Fatalf("deduped entry must be the registered one (pending=false), got pending=true")
	}
	if got.ID == "" {
		t.Fatalf("registered entry must carry an id")
	}
	if got.ID != um.List()[0].ID {
		t.Fatalf("id mismatch: got %q want %q", got.ID, um.List()[0].ID)
	}
}
