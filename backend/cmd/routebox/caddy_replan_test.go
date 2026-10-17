package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"routebox/backend/internal/bootstrap"
	"routebox/backend/internal/config"
)

// staleInstall is a fresh all-in-one install whose Caddyfile has since been
// replaced by something the planner would never write, and whose dest is
// unreachable — so SyncCaddyfile writes without asking Caddy.
func staleInstall(t *testing.T) (func() error, allInOne) {
	t.Helper()
	sm, a, _ := installed(t)
	if err := os.WriteFile(a.CaddyPath, []byte("# stale\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(bootstrap.CaddyAdminEnv, "127.0.0.1:1")
	cfg, err := config.NewManager(a.ConfigPath)
	if err != nil {
		t.Fatalf("load the planned config: %v", err)
	}
	return caddyReplanner(sm, cfg, 8443, a.StubRoot), a
}

func TestCaddyReplannerRewritesAStaleCaddyfile(t *testing.T) {
	replan, a := staleInstall(t)
	if err := replan(); err != nil {
		t.Fatalf("replan: %v", err)
	}
	got, err := os.ReadFile(a.CaddyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), bootstrap.CaddyfileHeader) {
		t.Errorf("the re-planned Caddyfile has no header:\n%s", got)
	}
	if !strings.Contains(string(got), "reverse_proxy 127.0.0.1:9446 {") {
		t.Errorf("the re-planned Caddyfile does not reach the web front:\n%s", got)
	}
}

// BOOTSTRAP_CADDYFILE_REPLAN=0 is the operator's promise that the file is
// theirs now.
func TestCaddyReplannerIsANoOpWhenDisabled(t *testing.T) {
	t.Setenv("BOOTSTRAP_CADDYFILE_REPLAN", "0")
	replan, a := staleInstall(t)
	if err := replan(); err != nil {
		t.Fatalf("replan: %v", err)
	}
	got, err := os.ReadFile(a.CaddyPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# stale\n" {
		t.Errorf("a disabled re-plan rewrote the Caddyfile:\n%s", got)
	}
}

// A plain VPS has no dest of ours to plan for.
func TestCaddyReplannerIsANoOpOffBootstrap(t *testing.T) {
	dir := t.TempDir()
	sm := newVPSSettings(t, dir)
	replan := caddyReplanner(sm, config.NewEmptyManager(filepath.Join(dir, "config.json")), 8443, filepath.Join(dir, "stub"))
	if err := replan(); err != nil {
		t.Fatalf("replan: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Caddyfile")); err == nil {
		t.Error("a re-plan off a bootstrapped install wrote a Caddyfile")
	}
}

// countingAdmin stands in for dest's admin API, refusing every /adapt, and
// counts the requests it gets.
func countingAdmin(t *testing.T) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("fake dest says no"))
	}))
	t.Cleanup(srv.Close)
	t.Setenv(bootstrap.CaddyAdminEnv, strings.TrimPrefix(srv.URL, "http://"))
	return &calls
}

// captureLog sends the standard logger to a buffer for the rest of the test.
func captureLog(t *testing.T) *strings.Builder {
	t.Helper()
	var buf strings.Builder
	var mu sync.Mutex
	log.SetOutput(writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return buf.Write(p)
	}))
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

// The expiry ticker re-plans every 30 seconds. A refusal that nothing has
// changed since must not be sent to dest again: it comes back, unasked, as
// the same error marked as a repeat.
func TestCaddyReplannerDoesNotAskDestAgainUntilSomethingChanges(t *testing.T) {
	sm, a, _ := installed(t)
	if err := os.WriteFile(a.CaddyPath, []byte("# stale\n"), 0644); err != nil {
		t.Fatal(err)
	}
	calls := countingAdmin(t)
	cfg, err := config.NewManager(a.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	replan := caddyReplanner(sm, cfg, 8443, a.StubRoot)

	first := replan()
	if first == nil || !strings.Contains(first.Error(), "fake dest says no") {
		t.Fatalf("first replan: %v, want dest's refusal", first)
	}
	if errors.Is(first, bootstrap.ErrReplanRepeated) {
		t.Error("the first refusal is marked as a repeat")
	}
	for i := 0; i < 3; i++ {
		err := replan()
		if !errors.Is(err, bootstrap.ErrReplanRepeated) || err.Error() != first.Error() {
			t.Fatalf("repeat %d: %v, want the first refusal marked as a repeat", i, err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("dest was asked %d times, want once while nothing changed", got)
	}

	// A changed input is a new attempt.
	if err := sm.Update(map[string]interface{}{"mtproto.web_listen": "127.0.0.1:9550"}); err != nil {
		t.Fatal(err)
	}
	if err := replan(); err == nil || errors.Is(err, bootstrap.ErrReplanRepeated) {
		t.Errorf("after web_listen moved: %v, want a fresh refusal", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("dest was asked %d times, want a second time after web_listen moved", got)
	}

	// So is a Caddyfile the operator edited by hand.
	if err := os.WriteFile(a.CaddyPath, []byte("# edited\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := replan(); err == nil || errors.Is(err, bootstrap.ErrReplanRepeated) {
		t.Errorf("after the file changed: %v, want a fresh refusal", err)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("dest was asked %d times, want a third time after the file changed", got)
	}
}

// A standing skip (an inbound the plan needs is gone) is logged once, not on
// every tick, and never reaches dest.
func TestCaddyReplannerLogsAStandingSkipOnce(t *testing.T) {
	sm, a, _ := installed(t)
	calls := countingAdmin(t)

	raw := readJSON(t, a.ConfigPath)
	var kept []interface{}
	for _, ib := range raw["inbounds"].([]interface{}) {
		if ib.(map[string]interface{})["tag"] != bootstrap.TagVlessGRPC {
			kept = append(kept, ib)
		}
	}
	raw["inbounds"] = kept
	body, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a.ConfigPath, body, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.NewManager(a.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	replan := caddyReplanner(sm, cfg, 8443, a.StubRoot)

	logs := captureLog(t)
	for i := 0; i < 3; i++ {
		if err := replan(); err != nil {
			t.Fatalf("replan %d: %v, want a skip, not an error", i, err)
		}
	}
	if n := strings.Count(logs.String(), "left as it is"); n != 1 {
		t.Errorf("the skip was logged %d times, want once:\n%s", n, logs.String())
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("dest was asked %d times for a skipped re-plan", got)
	}
}

// A refusal can have a cause outside what the memory keys on (a port dest
// could not bind, since freed), so after replanRetryAfter dest is asked again.
// A retry refused the same way still reads as a repeat: logged once.
func TestCaddyReplannerRetriesARefusalOnceItIsOld(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	replanClock = func() time.Time { return now }
	t.Cleanup(func() { replanClock = time.Now })

	sm, a, _ := installed(t)
	if err := os.WriteFile(a.CaddyPath, []byte("# stale\n"), 0644); err != nil {
		t.Fatal(err)
	}
	calls := countingAdmin(t)
	cfg, err := config.NewManager(a.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	replan := caddyReplanner(sm, cfg, 8443, a.StubRoot)

	if err := replan(); err == nil || errors.Is(err, bootstrap.ErrReplanRepeated) {
		t.Fatalf("first replan: %v, want a fresh refusal", err)
	}

	now = now.Add(replanRetryAfter - time.Second)
	if err := replan(); !errors.Is(err, bootstrap.ErrReplanRepeated) {
		t.Fatalf("just before the expiry: %v, want the remembered refusal", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("dest was asked %d times before the expiry, want once", got)
	}

	now = now.Add(2 * time.Second)
	err = replan()
	if got := calls.Load(); got != 2 {
		t.Fatalf("dest was asked %d times after the expiry, want a second time", got)
	}
	if !errors.Is(err, bootstrap.ErrReplanRepeated) || !strings.Contains(err.Error(), "fake dest says no") {
		t.Errorf("the retry: %v, want the same refusal, marked as a repeat", err)
	}

	// The retry restarted the clock.
	now = now.Add(replanRetryAfter - time.Second)
	if err := replan(); !errors.Is(err, bootstrap.ErrReplanRepeated) || calls.Load() != 2 {
		t.Errorf("after the retry: %v with %d calls, want the memory renewed", err, calls.Load())
	}
}
