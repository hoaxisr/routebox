package bootstrap

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func installFixture() Install {
	return Install{Domain: "media.example.com", Dest: "127.0.0.1:8443", PanelPath: "/panel-9c2f1a",
		Caddyfile: "/var/lib/routebox/Caddyfile", StubRoot: "/var/lib/routebox/stub",
		FrontPort: 443, PanelPort: 8080, WebFrontPort: 8446}
}

// The re-plan must reproduce the bootstrap's own Caddyfile from what the
// install keeps: nothing the bootstrap knew may be needed twice.
func TestCaddyParamsFromInstallRoundTrips(t *testing.T) {
	p := fixture()
	p.NaiveUsers = NaiveUsersPath("/var/lib/routebox/Caddyfile")
	got, err := CaddyParamsFromInstall(installFixture(), planOK(t, p))
	if err != nil {
		t.Fatal(err)
	}
	if caddyfileOK(t, got) != caddyfileOK(t, p) {
		t.Fatalf("re-planned Caddyfile differs")
	}
}

func TestCaddyParamsFromInstallSkipsWhenAnInboundIsGone(t *testing.T) {
	cfg := planOK(t, fixture())
	cfg["inbounds"] = cfg["inbounds"].([]interface{})[:1] // reality only
	_, err := CaddyParamsFromInstall(installFixture(), cfg)
	if !errors.Is(err, ErrReplanSkipped) {
		t.Fatalf("err = %v, want ErrReplanSkipped", err)
	}
}

func TestRealityServerNameIsReadFromTheActiveConfig(t *testing.T) {
	if got := RealityServerName(planOK(t, fixture())); got != "media.example.com" {
		t.Fatalf("got %q", got)
	}
	if got := RealityServerName(map[string]interface{}{}); got != "" {
		t.Fatalf("got %q for no reality inbound", got)
	}
}

func TestReplanDisabledReadsTheEnv(t *testing.T) {
	for v, want := range map[string]bool{"": false, "1": false, "0": true, "false": true, "FALSE": true} {
		t.Setenv("BOOTSTRAP_CADDYFILE_REPLAN", v)
		if ReplanDisabled() != want {
			t.Errorf("%q: got %v", v, !want)
		}
	}
}

// fakeAdmin stands in for dest's admin API: it answers /adapt and /load with the
// given statuses, records the paths it was asked, and points the bootstrap at
// itself through BOOTSTRAP_CADDY_ADMIN.
func fakeAdmin(t *testing.T, adaptStatus, loadStatus int) (calls *[]string) {
	t.Helper()
	var mu sync.Mutex
	calls = &[]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*calls = append(*calls, r.URL.Path)
		mu.Unlock()
		status := http.StatusNotFound
		switch r.URL.Path {
		case "/adapt":
			status = adaptStatus
		case "/load":
			status = loadStatus
		}
		w.WriteHeader(status)
		if status/100 != 2 {
			w.Write([]byte("fake dest says no"))
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv(CaddyAdminEnv, strings.TrimPrefix(srv.URL, "http://"))
	return calls
}

func syncFixture(t *testing.T) (Params, string) {
	t.Helper()
	p := fixture()
	return p, filepath.Join(t.TempDir(), "Caddyfile")
}

func TestSyncCaddyfileDoesNothingWhenUnchanged(t *testing.T) {
	p, path := syncFixture(t)
	if err := os.WriteFile(path, []byte(caddyfileOK(t, p)), 0644); err != nil {
		t.Fatal(err)
	}
	calls := fakeAdmin(t, 200, 200)
	changed, err := SyncCaddyfile(p, path)
	if changed || err != nil {
		t.Fatalf("changed=%v err=%v, want false nil", changed, err)
	}
	if len(*calls) != 0 {
		t.Fatalf("admin was called: %v", *calls)
	}
}

func TestSyncCaddyfileKeepsTheFileWhenAdaptRefuses(t *testing.T) {
	p, path := syncFixture(t)
	if err := os.WriteFile(path, []byte("old caddyfile\n"), 0644); err != nil {
		t.Fatal(err)
	}
	calls := fakeAdmin(t, 400, 200)
	changed, err := SyncCaddyfile(p, path)
	if changed || err == nil || !strings.Contains(err.Error(), "fake dest says no") {
		t.Fatalf("changed=%v err=%v, want false and dest's reason", changed, err)
	}
	if got, _ := os.ReadFile(path); string(got) != "old caddyfile\n" {
		t.Fatalf("file was rewritten: %q", got)
	}
	if !reflect.DeepEqual(*calls, []string{"/adapt"}) {
		t.Fatalf("calls = %v, want [/adapt]", *calls)
	}
}

func TestSyncCaddyfileWritesAndReloads(t *testing.T) {
	p, path := syncFixture(t)
	calls := fakeAdmin(t, 200, 200)
	changed, err := SyncCaddyfile(p, path)
	if !changed || err != nil {
		t.Fatalf("changed=%v err=%v, want true nil", changed, err)
	}
	if got, _ := os.ReadFile(path); string(got) != caddyfileOK(t, p) {
		t.Fatalf("file is not the rendered Caddyfile")
	}
	if !reflect.DeepEqual(*calls, []string{"/adapt", "/load"}) {
		t.Fatalf("calls = %v", *calls)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0644 {
		t.Fatalf("mode = %v, want 0644", fi.Mode().Perm())
	}
}

func TestSyncCaddyfileWritesWhenDestIsDown(t *testing.T) {
	p, path := syncFixture(t)
	t.Setenv(CaddyAdminEnv, "127.0.0.1:1")
	changed, err := SyncCaddyfile(p, path)
	if !changed || err != nil {
		t.Fatalf("changed=%v err=%v, want true nil", changed, err)
	}
	if got, _ := os.ReadFile(path); string(got) != caddyfileOK(t, p) {
		t.Fatalf("file is not the rendered Caddyfile")
	}
}

func TestSyncCaddyfileRestoresTheFileWhenLoadRefuses(t *testing.T) {
	p, path := syncFixture(t)
	if err := os.WriteFile(path, []byte("old caddyfile\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fakeAdmin(t, 200, 400)
	changed, err := SyncCaddyfile(p, path)
	if changed || err == nil {
		t.Fatalf("changed=%v err=%v, want false and an error", changed, err)
	}
	if got, _ := os.ReadFile(path); string(got) != "old caddyfile\n" {
		t.Fatalf("refused file left on disk: %q", got)
	}
}

func TestSyncCaddyfileRemovesTheFileWhenLoadRefusesAndNoneExisted(t *testing.T) {
	p, path := syncFixture(t)
	fakeAdmin(t, 200, 400)
	changed, err := SyncCaddyfile(p, path)
	if changed || err == nil {
		t.Fatalf("changed=%v err=%v, want false and an error", changed, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("refused file left on disk: %v", err)
	}
}

// R15: the re-plan follows network.acme_staging only in the safe direction.
func TestSyncCaddyfileLeavesStagingWhenTheSettingIsOff(t *testing.T) {
	p, path := syncFixture(t)
	staged := p
	staged.ACME.Staging = true
	if err := os.WriteFile(path, []byte(caddyfileOK(t, staged)), 0644); err != nil {
		t.Fatal(err)
	}
	fakeAdmin(t, 200, 200)
	p.ACME.Staging = false
	if _, err := SyncCaddyfile(p, path); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != caddyfileOK(t, p) {
		t.Fatalf("a staging file with the setting off was not moved to production:\n%s", got)
	}
}

func TestSyncCaddyfileKeepsTheStagingCAWhileTheSettingIsOn(t *testing.T) {
	p, path := syncFixture(t)
	p.ACME.Staging = true
	if err := os.WriteFile(path, []byte("# old\n"+stagingCA+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fakeAdmin(t, 200, 200)
	if _, err := SyncCaddyfile(p, path); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != caddyfileOK(t, p) || !strings.Contains(string(got), stagingCA) {
		t.Fatalf("the staging CA was dropped while the setting is on:\n%s", got)
	}
}

func TestSyncCaddyfileNeverMovesProductionToStaging(t *testing.T) {
	p, path := syncFixture(t)
	production := p
	production.ACME.Staging = false
	if err := os.WriteFile(path, []byte("# old, production\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fakeAdmin(t, 200, 200)
	p.ACME.Staging = true
	if _, err := SyncCaddyfile(p, path); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != caddyfileOK(t, production) || strings.Contains(string(got), stagingCA) {
		t.Fatalf("a production file was moved to the staging CA:\n%s", got)
	}
}

func TestSyncCaddyfileFollowsTheSettingWithNoFileYet(t *testing.T) {
	p, path := syncFixture(t)
	p.ACME.Staging = true
	fakeAdmin(t, 200, 200)
	if _, err := SyncCaddyfile(p, path); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), stagingCA) {
		t.Fatalf("a first plan ignored the staging setting:\n%s", got)
	}
}

func TestCaddyParamsFromInstallCarriesTheStagingSetting(t *testing.T) {
	in := installFixture()
	in.ACMEStaging = true
	got, err := CaddyParamsFromInstall(in, planOK(t, fixture()))
	if err != nil {
		t.Fatal(err)
	}
	if !got.ACME.Staging {
		t.Fatal("ACME.Staging = false, want network.acme_staging carried over")
	}
}
