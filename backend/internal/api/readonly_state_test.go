package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/awg"
	"routebox/backend/internal/clients"
	"routebox/backend/internal/config"
	"routebox/backend/internal/process"
	"routebox/backend/internal/settings"
	"routebox/backend/internal/subscriptions"
	"routebox/backend/internal/users"
)

// The sing-box config and RouteBox's own state live in different directories and
// can be mounted separately, so "read-only" is not one file's property. These
// tests pin the whole surface: every store refuses with a 409 naming its own
// file, and the status endpoint reports the union so one badge can stand for all
// of them.

func readOnlyDir(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })
	return dir
}

// writableConfig builds a config manager that is emphatically NOT read-only, so
// a badge raised in these tests can only have come from a state store.
func writableConfig(t *testing.T) *config.Manager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"log":{"level":"info"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := config.NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.IsReadOnly() {
		t.Fatal("harness: this config must be writable")
	}
	return m
}

func statusPaths(t *testing.T, h *Handler) (readOnly bool, paths []string) {
	t.Helper()
	rr := httptest.NewRecorder()
	h.GetStatus(rr, httptest.NewRequest("GET", "/api/status", nil))
	var resp struct {
		Data struct {
			ConfigReadOnly bool     `json:"config_read_only"`
			ReadOnlyPaths  []string `json:"read_only_paths"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("%v (body %s)", err, rr.Body.String())
	}
	return resp.Data.ConfigReadOnly, resp.Data.ReadOnlyPaths
}

// The partial case is the real one: RouteBox's state directory and the sing-box
// config directory are different mounts. A store that cannot be written has to
// raise the badge even though the config is fine — and must NOT claim the config
// is read-only, because that flag is what greys out the config save buttons.
func TestStatusReportsAStoreThatIsReadOnlyWhileTheConfigIsNot(t *testing.T) {
	dir := readOnlyDir(t)
	usersPath := filepath.Join(dir, "users.toml")
	h := &Handler{config: writableConfig(t), panelUsers: users.NewManager(usersPath)}
	h.statusSource = func() process.Status { return process.Status{} }

	configReadOnly, paths := statusPaths(t, h)
	if configReadOnly {
		t.Fatal("a writable config must not be reported read-only just because a store is")
	}
	if !slices.Contains(paths, usersPath) {
		t.Fatalf("read_only_paths %v must name the unwritable store %q", paths, usersPath)
	}
}

// Several files can be unwritable at once (one read-only mount holds them all).
// The badge names each of them: "something is read-only" is not an instruction.
func TestStatusReportsEveryUnwritablePath(t *testing.T) {
	dir := readOnlyDir(t)
	// A second unwritable directory for the config: in production it is a
	// different mount, which is why it needs its own here too.
	cfgDir := readOnlyDir(t)
	cfgMgr := config.NewEmptyManager(filepath.Join(cfgDir, "config.json"))
	if !cfgMgr.IsReadOnly() {
		t.Fatal("harness: config in a 0500 dir must be read-only")
	}

	h := &Handler{
		config:     cfgMgr,
		clients:    clients.New(filepath.Join(dir, "clients.toml")),
		panelUsers: users.NewManager(filepath.Join(dir, "users.toml")),
		subs:       subscriptions.NewManager(filepath.Join(dir, "subscriptions.toml")),
	}
	h.statusSource = func() process.Status { return process.Status{} }

	configReadOnly, paths := statusPaths(t, h)
	if !configReadOnly {
		t.Fatal("the config-specific flag must still report the config")
	}
	for _, want := range []string{
		cfgMgr.GetPath(),
		filepath.Join(dir, "clients.toml"),
		filepath.Join(dir, "users.toml"),
		filepath.Join(dir, "subscriptions.toml"),
	} {
		if !slices.Contains(paths, want) {
			t.Fatalf("read_only_paths %v is missing %q", paths, want)
		}
	}
}

func TestStatusReportsNoPathsWhenEverythingIsWritable(t *testing.T) {
	dir := t.TempDir()
	h := &Handler{
		config:     writableConfig(t),
		clients:    clients.New(filepath.Join(dir, "clients.toml")),
		panelUsers: users.NewManager(filepath.Join(dir, "users.toml")),
		subs:       subscriptions.NewManager(filepath.Join(dir, "subscriptions.toml")),
	}
	h.statusSource = func() process.Status { return process.Status{} }

	configReadOnly, paths := statusPaths(t, h)
	if configReadOnly || len(paths) != 0 {
		t.Fatalf("a healthy install must report nothing: config_read_only=%v paths=%v", configReadOnly, paths)
	}
}

func TestUpdateClientAnswers409WhenTheStoreIsReadOnly(t *testing.T) {
	dir := readOnlyDir(t)
	path := filepath.Join(dir, "clients.toml")
	h := &Handler{clients: clients.New(path)}

	r := chi.NewRouter()
	r.Put("/api/clients/{ip}", h.UpdateClient)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/clients/10.0.0.5",
		strings.NewReader(`{"name":"laptop"}`)))

	assert409Naming(t, rr, path)
}

func TestCreateSubscriptionAnswers409WhenTheStoreIsReadOnly(t *testing.T) {
	dir := readOnlyDir(t)
	path := filepath.Join(dir, "subscriptions.toml")
	h := &Handler{subs: subscriptions.NewManager(path)}

	rr := httptest.NewRecorder()
	h.CreateSubscription(rr, httptest.NewRequest(http.MethodPost, "/api/subscriptions",
		strings.NewReader(`{"name":"Home","url":"https://example.com/sub","interval_hrs":24}`)))

	assert409Naming(t, rr, path)
}

// routebox.toml is where the panel's own settings live, so "Save" on the
// settings page is a write like any other and gets the same 409.
func TestUpdateSettingsAnswers409WhenSettingsAreReadOnly(t *testing.T) {
	dir := readOnlyDir(t)
	sm, err := settings.NewManager(filepath.Join(dir, "routebox.toml"))
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{settings: sm}

	rr := httptest.NewRecorder()
	h.UpdateSettings(rr, httptest.NewRequest(http.MethodPut, "/api/settings",
		strings.NewReader(`{"ui.language":"ru"}`)))

	assert409Naming(t, rr, sm.GetPath())
}

// A malformed body is a malformed body whatever the filesystem is doing. Routing
// it through the read-only helper suggested a 409 it can never produce.
func TestAMalformedBodyIsStillA400OnAReadOnlyStore(t *testing.T) {
	dir := readOnlyDir(t)
	h := &Handler{subs: subscriptions.NewManager(filepath.Join(dir, "subscriptions.toml"))}

	rr := httptest.NewRecorder()
	h.CreateSubscription(rr, httptest.NewRequest(http.MethodPost, "/api/subscriptions",
		strings.NewReader(`{not json`)))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a malformed body, got %d: %s", rr.Code, rr.Body.String())
	}
}

func assert409Naming(t *testing.T, rr *httptest.ResponseRecorder, path string) {
	t.Helper()
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 on a read-only store, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), path) {
		t.Fatalf("response %q must name %q — the file the operator has to fix", rr.Body.String(), path)
	}
}

// --- AWG operations against a read-only config -------------------------------
//
// Disable, delete-peer and set-expiry all rewrite the sing-box endpoint. Their
// error paths were converted to the 409 helper without a test to hold them
// there; these are it.

func awgRouterFor(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Post("/api/awg/disable", h.DisableAWG)
	r.Delete("/api/awg/peers/{publicKey}", h.DeleteAWGPeer)
	r.Patch("/api/awg/peers/{publicKey}/expiry", h.SetAWGPeerExpiry)
	return r
}

func seedAWGPeer(t *testing.T, h *Handler) {
	t.Helper()
	// Straight into the store: the peers directory is writable, only the config
	// is not, which is the situation these tests are about.
	if err := h.awg.Store().Put(awg.Peer{
		PublicKey:    knownPub,
		PrivateKey:   knownPriv,
		PresharedKey: knownPSK,
		Address:      "10.10.0.2/32",
		Name:         "phone",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDisableAWGAnswers409WhenConfigIsReadOnly(t *testing.T) {
	h, path := newReadOnlyAWGHandler(t)

	rr := httptest.NewRecorder()
	awgRouterFor(h).ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/awg/disable", nil))

	assert409Naming(t, rr, path)
}

func TestDeleteAWGPeerAnswers409WhenConfigIsReadOnly(t *testing.T) {
	h, path := newReadOnlyAWGHandler(t)
	seedAWGPeer(t, h)

	rr := httptest.NewRecorder()
	awgRouterFor(h).ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/api/awg/peers/"+knownPub, nil))

	assert409Naming(t, rr, path)
	if _, ok := h.awg.Store().Get(knownPub); !ok {
		t.Fatal("a refused delete must leave the peer's secret in place — the client is still live")
	}
}

func TestSetAWGPeerExpiryAnswers409WhenConfigIsReadOnly(t *testing.T) {
	h, path := newReadOnlyAWGHandler(t)
	seedAWGPeer(t, h)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/awg/peers/"+knownPub+"/expiry",
		strings.NewReader(`{"expires_at":0}`))
	awgRouterFor(h).ServeHTTP(rr, req)

	assert409Naming(t, rr, path)
}

// The AWG secret store has its own directory, so it can be read-only on its own.
func TestStatusReportsAReadOnlyPeerStore(t *testing.T) {
	dir := readOnlyDir(t)
	m := awg.NewManagerForTest(awgNoopRunner{}, dir, serverPriv, awg.Config{
		Iface: "awg-rb0", Subnet: "10.10.0.0/24", ServerIP: "10.10.0.1",
		ListenPort: 51820, MTU: 1420,
	})
	h := &Handler{config: writableConfig(t)}
	h.SetAWG(m)
	h.statusSource = func() process.Status { return process.Status{} }

	_, paths := statusPaths(t, h)
	want := filepath.Join(dir, "peers.toml")
	if !slices.Contains(paths, want) {
		t.Fatalf("read_only_paths %v must name %q", paths, want)
	}
}

// routebox.toml is RouteBox's own settings file and belongs in the same union.
func TestStatusReportsReadOnlySettings(t *testing.T) {
	dir := readOnlyDir(t)
	sm, err := settings.NewManager(filepath.Join(dir, "routebox.toml"))
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{config: writableConfig(t), settings: sm}
	h.statusSource = func() process.Status { return process.Status{} }

	_, paths := statusPaths(t, h)
	if !slices.Contains(paths, sm.GetPath()) {
		t.Fatalf("read_only_paths %v must name %q", paths, sm.GetPath())
	}
}
