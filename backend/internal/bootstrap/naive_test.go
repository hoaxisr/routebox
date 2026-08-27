package bootstrap

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// configWith builds the shape ServerUsersOfConfig reads: inbounds with users.
func configWith(inbounds ...map[string]interface{}) map[string]interface{} {
	arr := make([]interface{}, 0, len(inbounds))
	for _, ib := range inbounds {
		arr = append(arr, ib)
	}
	return map[string]interface{}{"inbounds": arr}
}

func inbound(tag, typ string, users ...map[string]interface{}) map[string]interface{} {
	arr := make([]interface{}, 0, len(users))
	for _, u := range users {
		arr = append(arr, u)
	}
	return map[string]interface{}{"tag": tag, "type": typ, "users": arr}
}

// The whole point of the ticket: the list dest checks naive passwords against is
// derived from the inbound users, so adding a user in the panel adds them here.
func TestNaiveUsersOfConfigTakesThePasswordUsers(t *testing.T) {
	cfg := configWith(
		inbound("trojan-ws-in", "trojan",
			map[string]interface{}{"name": "alice", "password": "pw-alice"},
			map[string]interface{}{"name": "bob", "password": "pw-bob"}),
	)
	got := NaiveUsersOfConfig(cfg, nil)
	want := []NaiveUser{{Name: "alice", Password: "pw-alice"}, {Name: "bob", Password: "pw-bob"}}
	if !equalUsers(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

// One person on five inbounds is one naive account, not five: the panel user is
// derived by name, and so is this.
func TestNaiveUsersOfConfigCollapsesOneUserAcrossInbounds(t *testing.T) {
	cfg := configWith(
		inbound("vless-reality-in", "vless", map[string]interface{}{"name": "alice", "uuid": "u-1"}),
		inbound("trojan-ws-in", "trojan", map[string]interface{}{"name": "alice", "password": "pw-alice"}),
		inbound("mieru-in", "mieru", map[string]interface{}{"name": "alice", "password": "pw-alice"}),
	)
	if got := NaiveUsersOfConfig(cfg, nil); !equalUsers(got, []NaiveUser{{Name: "alice", Password: "pw-alice"}}) {
		t.Fatalf("one user across three inbounds became %#v", got)
	}
}

// A rename in the config is a rename, not a second account: the old name is gone
// from the list the moment it is gone from the inbound.
func TestNaiveUsersOfConfigFollowsARename(t *testing.T) {
	cfg := configWith(inbound("trojan-ws-in", "trojan",
		map[string]interface{}{"name": "alice-renamed", "password": "pw-alice"}))
	if got := NaiveUsersOfConfig(cfg, nil); !equalUsers(got, []NaiveUser{{Name: "alice-renamed", Password: "pw-alice"}}) {
		t.Fatalf("rename produced %#v", got)
	}
}

// vless carries a uuid, not a password. A user with nothing but vless has no
// naive credential to check, and inventing one would hand out access the panel
// never granted.
func TestNaiveUsersOfConfigSkipsUsersWithoutAPassword(t *testing.T) {
	cfg := configWith(inbound("vless-reality-in", "vless",
		map[string]interface{}{"name": "alice", "uuid": "u-1"}))
	if got := NaiveUsersOfConfig(cfg, nil); len(got) != 0 {
		t.Fatalf("uuid-only user got a naive credential: %#v", got)
	}
}

// Disabled and expired users are rejected on the sing-box side by the managed
// route rule, which dest never sees — so they have to be dropped here instead.
func TestNaiveUsersOfConfigDropsBlockedNames(t *testing.T) {
	cfg := configWith(inbound("trojan-ws-in", "trojan",
		map[string]interface{}{"name": "alice", "password": "pw-alice"},
		map[string]interface{}{"name": "bob", "password": "pw-bob"}))
	got := NaiveUsersOfConfig(cfg, map[string]bool{"bob": true})
	if !equalUsers(got, []NaiveUser{{Name: "alice", Password: "pw-alice"}}) {
		t.Fatalf("blocked user survived: %#v", got)
	}
}

func TestNaiveUsersOfConfigIsDeterministic(t *testing.T) {
	cfg := configWith(inbound("trojan-ws-in", "trojan",
		map[string]interface{}{"name": "carol", "password": "pw-c"},
		map[string]interface{}{"name": "alice", "password": "pw-a"},
		map[string]interface{}{"name": "bob", "password": "pw-b"}))
	first := NaiveUsersOfConfig(cfg, nil)
	for i := 0; i < 5; i++ {
		if !equalUsers(NaiveUsersOfConfig(cfg, nil), first) {
			t.Fatalf("same config gave a different order: %#v vs %#v", NaiveUsersOfConfig(cfg, nil), first)
		}
	}
}

func TestRenderNaiveUsersRendersOneLinePerUser(t *testing.T) {
	out, err := RenderNaiveUsers([]NaiveUser{{Name: "alice", Password: "pw-a"}, {Name: "bob", Password: "pw-b"}})
	if err != nil {
		t.Fatalf("RenderNaiveUsers: %v", err)
	}
	requireLine(t, out, "basic_auth alice pw-a")
	requireLine(t, out, "basic_auth bob pw-b")
}

func TestRenderNaiveUsersQuotesCredentials(t *testing.T) {
	out, err := RenderNaiveUsers([]NaiveUser{{Name: "owner", Password: `pa ss"word`}})
	if err != nil {
		t.Fatalf("RenderNaiveUsers: %v", err)
	}
	requireLine(t, out, `basic_auth owner "pa ss\"word"`)
}

// An empty list is not an empty file: forward_proxy with no credentials is an
// open proxy, and dest would relay for anyone who found it.
func TestRenderNaiveUsersRefusesAnEmptyList(t *testing.T) {
	if _, err := RenderNaiveUsers(nil); err == nil {
		t.Fatal("empty user list accepted")
	}
}

func TestSyncNaiveUsersWritesAndReportsChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "naive-users.caddy")
	u := []NaiveUser{{Name: "alice", Password: "pw-a"}}

	changed, err := SyncNaiveUsers(path, u)
	if err != nil || !changed {
		t.Fatalf("first write: changed=%v err=%v", changed, err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	requireLine(t, string(body), "basic_auth alice pw-a")

	// Unchanged input must not report a change: every change here costs dest a
	// reload, and a reload nobody needed is connections dropped for nothing.
	changed, err = SyncNaiveUsers(path, u)
	if err != nil || changed {
		t.Fatalf("rewrite of identical content: changed=%v err=%v", changed, err)
	}

	changed, err = SyncNaiveUsers(path, append(u, NaiveUser{Name: "bob", Password: "pw-b"}))
	if err != nil || !changed {
		t.Fatalf("added user: changed=%v err=%v", changed, err)
	}
}

// A failed render must leave the previous list in place: the alternative is dest
// refusing to load at all on its next restart.
func TestSyncNaiveUsersKeepsThePreviousListOnRefusal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "naive-users.caddy")
	if _, err := SyncNaiveUsers(path, []NaiveUser{{Name: "alice", Password: "pw-a"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncNaiveUsers(path, []NaiveUser{{Name: "bob", Password: ""}}); err == nil {
		t.Fatal("a user with no password was accepted")
	}
	body, _ := os.ReadFile(path)
	requireLine(t, string(body), "basic_auth alice pw-a")
}

// The last user leaving must lock naive, not open it: an empty credential list
// is a forward_proxy that authenticates nobody, which relays for everybody.
func TestSyncNaiveUsersLocksNaiveWhenNobodyIsLeft(t *testing.T) {
	path := filepath.Join(t.TempDir(), "naive-users.caddy")
	if _, err := SyncNaiveUsers(path, []NaiveUser{{Name: "alice", Password: "pw-a"}}); err != nil {
		t.Fatal(err)
	}
	changed, err := SyncNaiveUsers(path, nil)
	if err != nil || !changed {
		t.Fatalf("emptying the list: changed=%v err=%v", changed, err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "alice") {
		t.Fatalf("the last user kept their access:\n%s", body)
	}
	if strings.Count(string(body), "basic_auth ") != 1 {
		t.Fatalf("want exactly one locked account:\n%s", body)
	}
	// The lock password is random, so re-locking would rewrite the file and
	// reload dest on every apply for as long as the install has no users.
	if changed, err := SyncNaiveUsers(path, nil); err != nil || changed {
		t.Fatalf("re-lock: changed=%v err=%v", changed, err)
	}
}

func TestNaiveUsersPathSitsNextToTheCaddyfile(t *testing.T) {
	if got := NaiveUsersPath("/etc/routebox/Caddyfile"); got != "/etc/routebox/naive-users.caddy" {
		t.Fatalf("got %q", got)
	}
}

// The Caddyfile imports the snippet rather than carrying the credentials, so a
// user change rewrites one small generated file and never the operator's own.
func TestPlanCaddyfileImportsTheNaiveUserList(t *testing.T) {
	p := fixture()
	requireLine(t, caddyfileOK(t, p), "import "+p.NaiveUsers)
}

func TestPlanCaddyfileRejectsAnUnsafeNaiveUsersPath(t *testing.T) {
	p := fixture()
	p.NaiveUsers = "/etc/route box/naive-users.caddy"
	if _, err := PlanCaddyfile(p); err == nil {
		t.Fatal("path with a space accepted")
	}
	p.NaiveUsers = ""
	if _, err := PlanCaddyfile(p); err == nil {
		t.Fatal("missing path accepted")
	}
}

func TestReloadCaddyPostsTheCaddyfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Caddyfile")
	if err := os.WriteFile(path, []byte("example.com {\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var gotType, gotBody string
	admin := caddyAdminStub(t, func(contentType, body string) (int, string) {
		gotType, gotBody = contentType, body
		return 200, ""
	})
	t.Setenv(CaddyAdminEnv, admin)

	if err := ReloadCaddy(path); err != nil {
		t.Fatalf("ReloadCaddy: %v", err)
	}
	if gotType != "text/caddyfile" {
		t.Fatalf("content type %q — Caddy would read the body as JSON", gotType)
	}
	if !strings.Contains(gotBody, "example.com") {
		t.Fatalf("body was not the Caddyfile: %q", gotBody)
	}
}

// A dest that refused the new list is the failure this whole ticket exists to
// make visible, so it must come back as an error and carry Caddy's own reason.
func TestReloadCaddyReportsRefusal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Caddyfile")
	if err := os.WriteFile(path, []byte("nonsense {"), 0644); err != nil {
		t.Fatal(err)
	}
	admin := caddyAdminStub(t, func(string, string) (int, string) {
		return 400, `{"error":"unexpected end of file"}`
	})
	t.Setenv(CaddyAdminEnv, admin)

	err := ReloadCaddy(path)
	if err == nil {
		t.Fatal("a rejected reload reported success")
	}
	if !strings.Contains(err.Error(), "unexpected end of file") {
		t.Fatalf("error hides Caddy's reason: %v", err)
	}
}

func TestReloadCaddyReportsAnUnreachableDest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Caddyfile")
	if err := os.WriteFile(path, []byte("example.com {\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Port 1 on loopback: nothing listens, and connecting fails immediately.
	t.Setenv(CaddyAdminEnv, "127.0.0.1:1")
	if err := ReloadCaddy(path); err == nil {
		t.Fatal("an unreachable admin endpoint reported success")
	}
}

func equalUsers(a, b []NaiveUser) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// caddyAdminStub stands in for Caddy's admin API and returns its address.
func caddyAdminStub(t *testing.T, reply func(contentType, body string) (int, string)) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/load" {
			t.Errorf("posted to %q, not /load", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		code, msg := reply(r.Header.Get("Content-Type"), string(body))
		w.WriteHeader(code)
		w.Write([]byte(msg))
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}
