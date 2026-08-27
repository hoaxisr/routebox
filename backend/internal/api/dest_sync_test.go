package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/bootstrap"
	"routebox/backend/internal/config"
	"routebox/backend/internal/process"
	"routebox/backend/internal/settings"
	"routebox/backend/internal/users"
)

// destStub stands in for dest's admin API and records every reload it is asked
// for, so a test can assert both that a change reached dest and that an
// unchanged list did not cost a reload.
type destStub struct {
	reloads int
	fail    bool
}

func (d *destStub) start(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		d.reloads++
		if d.fail {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"dest says no"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	t.Setenv(bootstrap.CaddyAdminEnv, strings.TrimPrefix(srv.URL, "http://"))
}

// destInstall builds a handler on an install that came up from the bootstrap
// plan: a Caddyfile, the credential list it imports, and the users of a
// trojan inbound the panel owns.
func destInstall(t *testing.T, inbounds string) (*Handler, *config.Manager, *users.Manager, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := `{"inbounds":` + inbounds + `,"outbounds":[{"type":"direct","tag":"direct"}],"route":{"rules":[]}}`
	if err := os.WriteFile(path, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := config.NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	um := users.NewManager(filepath.Join(dir, "users.toml"))
	if _, err := um.Reconcile(m.GetActive()); err != nil {
		t.Fatal(err)
	}
	sm, err := settings.NewManager(filepath.Join(dir, "routebox.toml"))
	if err != nil {
		t.Fatal(err)
	}
	caddyfile := filepath.Join(dir, "Caddyfile")
	if err := os.WriteFile(caddyfile, []byte("example.com {\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := sm.SetBootstrap("/panel-secret", "127.0.0.1:9443", caddyfile, 443); err != nil {
		t.Fatal(err)
	}

	h := &Handler{config: m, settings: sm}
	h.SetUsers(um)
	h.statusSource = func() process.Status { return process.Status{Running: false} }
	h.v2rayAPISupported = func() bool { return false }
	return h, m, um, caddyfile
}

func apply(t *testing.T, h *Handler) map[string]interface{} {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ApplyConfig(rr, httptest.NewRequest("POST", "/api/config/apply", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("ApplyConfig status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("apply answer is not JSON: %v", err)
	}
	return resp.Data
}

func naiveList(t *testing.T, caddyfile string) string {
	t.Helper()
	body, err := os.ReadFile(bootstrap.NaiveUsersPath(caddyfile))
	if err != nil {
		t.Fatalf("read the naive credential list: %v", err)
	}
	return string(body)
}

const oneUser = `[{"type":"trojan","tag":"trojan-ws-in","listen_port":8445,
	"users":[{"name":"alice","password":"pw-alice"}]}]`

// Adding a user in the panel makes them work over naive too — which means the
// applied config reaching dest, since dest is what checks naive's passwords.
func TestApplyConfig_AddedUserReachesDest(t *testing.T) {
	dest := &destStub{}
	dest.start(t)
	h, m, _, caddyfile := destInstall(t, oneUser)

	draft := m.Get()
	in := draft["inbounds"].([]interface{})[0].(map[string]interface{})
	in["users"] = append(in["users"].([]interface{}),
		map[string]interface{}{"name": "bob", "password": "pw-bob"})
	if err := m.SetDraft(draft); err != nil {
		t.Fatal(err)
	}

	apply(t, h)
	got := naiveList(t, caddyfile)
	for _, want := range []string{"basic_auth alice pw-alice", "basic_auth bob pw-bob"} {
		if !strings.Contains(got, want) {
			t.Errorf("the credential list has no %q:\n%s", want, got)
		}
	}
	if dest.reloads != 1 {
		t.Errorf("dest was reloaded %d times, want exactly one", dest.reloads)
	}
}

// Removing a user closes their naive access: gone from the inbound is gone from
// the list dest checks.
func TestApplyConfig_RemovedUserLeavesDest(t *testing.T) {
	dest := &destStub{}
	dest.start(t)
	h, m, _, caddyfile := destInstall(t, `[{"type":"trojan","tag":"trojan-ws-in","listen_port":8445,
		"users":[{"name":"alice","password":"pw-alice"},{"name":"bob","password":"pw-bob"}]}]`)

	draft := m.Get()
	in := draft["inbounds"].([]interface{})[0].(map[string]interface{})
	in["users"] = []interface{}{map[string]interface{}{"name": "alice", "password": "pw-alice"}}
	if err := m.SetDraft(draft); err != nil {
		t.Fatal(err)
	}

	apply(t, h)
	if got := naiveList(t, caddyfile); strings.Contains(got, "bob") {
		t.Errorf("a deleted user still has naive access:\n%s", got)
	}
}

// A rename is one user under a new name, not a new user next to the old one.
func TestApplyConfig_RenamedUserDoesNotSplit(t *testing.T) {
	dest := &destStub{}
	dest.start(t)
	h, m, _, caddyfile := destInstall(t, oneUser)

	draft := m.Get()
	in := draft["inbounds"].([]interface{})[0].(map[string]interface{})
	in["users"] = []interface{}{map[string]interface{}{"name": "alice-renamed", "password": "pw-alice"}}
	if err := m.SetDraft(draft); err != nil {
		t.Fatal(err)
	}

	apply(t, h)
	got := naiveList(t, caddyfile)
	if strings.Contains(got, "basic_auth alice ") {
		t.Errorf("the old name survived the rename:\n%s", got)
	}
	if strings.Count(got, "basic_auth ") != 1 {
		t.Errorf("a rename produced more than one account:\n%s", got)
	}
}

// A reload nobody needed is dest re-adapting its whole config for nothing, so an
// apply that changed no user must not ask for one.
func TestApplyConfig_UnchangedUsersDoNotReloadDest(t *testing.T) {
	dest := &destStub{}
	dest.start(t)
	h, _, _, _ := destInstall(t, oneUser)

	apply(t, h) // first apply writes the list
	before := dest.reloads
	apply(t, h) // nothing changed
	if dest.reloads != before {
		t.Errorf("an apply with no user change reloaded dest (%d -> %d)", before, dest.reloads)
	}
}

// The failure this ticket exists to prevent is a silent one: if dest did not
// take the change, the operator has to be told, not left with a panel claiming
// success.
func TestApplyConfig_ReportsADestThatRefused(t *testing.T) {
	dest := &destStub{fail: true}
	dest.start(t)
	h, _, _, _ := destInstall(t, oneUser)

	data := apply(t, h)
	warning, _ := data["warning"].(string)
	if warning == "" {
		t.Fatalf("a dest that refused the change was swallowed: %#v", data)
	}
	if !strings.Contains(warning, "dest says no") {
		t.Errorf("the warning hides dest's own reason: %q", warning)
	}
}

// Every other way of installing RouteBox has no dest and no Caddyfile of ours.
// Applying there must not write files next to a config directory that never
// asked for them.
func TestApplyConfig_LeavesANonBootstrappedInstallAlone(t *testing.T) {
	dest := &destStub{}
	dest.start(t)
	h, _, _, caddyfile := destInstall(t, oneUser)
	// Settings with no bootstrap mark and no Caddyfile: what every other way of
	// installing RouteBox leaves behind.
	plain, err := settings.NewManager(filepath.Join(t.TempDir(), "routebox.toml"))
	if err != nil {
		t.Fatal(err)
	}
	h.settings = plain

	apply(t, h)
	if _, err := os.Stat(bootstrap.NaiveUsersPath(caddyfile)); !os.IsNotExist(err) {
		t.Errorf("a credential list was written on an install with no dest (err=%v)", err)
	}
	if dest.reloads != 0 {
		t.Errorf("dest was reloaded on an install that has none (%d)", dest.reloads)
	}
}

// Disabling a user is enforced inside sing-box by a route rule dest never sees,
// so the same decision has to reach dest by dropping them from its list.
func TestSyncRejectRule_DisabledUserLosesNaive(t *testing.T) {
	dest := &destStub{}
	dest.start(t)
	h, _, um, caddyfile := destInstall(t, oneUser)

	apply(t, h)
	if !strings.Contains(naiveList(t, caddyfile), "alice") {
		t.Fatal("the user is missing before the disable, so the test proves nothing")
	}

	for _, u := range um.List() {
		u.Enabled = false
		if err := um.Put(&u); err != nil {
			t.Fatal(err)
		}
	}
	h.syncRejectRule()

	if got := naiveList(t, caddyfile); strings.Contains(got, "alice") {
		t.Errorf("a disabled user keeps naive access:\n%s", got)
	}
}

// Disabling a user is answered by the PATCH handler, so a dest that did not
// take the change has to come back in that answer. Logging it would leave the
// panel showing a clean success over a user who can still connect.
func TestUpdateUser_ReportsADestThatRefused(t *testing.T) {
	dest := &destStub{}
	dest.start(t)
	h, _, um, _ := destInstall(t, oneUser)
	apply(t, h) // the credential list now exists, so the disable really changes it
	dest.fail = true

	id := um.List()[0].ID
	r := chi.NewRouter()
	r.Patch("/api/users/{id}", h.UpdateUser)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodPatch, "/api/users/"+id, strings.NewReader(`{"enabled":false}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			Warning string `json:"warning"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Warning == "" {
		t.Fatalf("a dest that refused the disable was swallowed: %s", rr.Body.String())
	}
	if !strings.Contains(resp.Data.Warning, "dest says no") {
		t.Errorf("the warning hides dest's own reason: %q", resp.Data.Warning)
	}
}
