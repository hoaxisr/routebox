package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A verdict taken at startup says nothing about a mount that goes read-only —
// or a chattr +i — while RouteBox is running. Every other RouteBox store
// notices that at the moment its write fails (util.WriteGuard.Note); the config
// used to guard only BEFORE the write and only when the standing verdict was
// already negative, so a config that went unwritable under a running panel came
// back as a raw errno with no path to fix and no read-only state anywhere.

// wentUnwritable builds a manager on a writable config and then takes the
// directory away under it, the way a remount does.
func wentUnwritable(t *testing.T) (*Manager, string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"log":{"level":"info"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.IsReadOnly() {
		t.Fatal("harness: a writable config must not start read-only")
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0700) })
	return m, path
}

func assertReadOnlyNaming(t *testing.T, what string, err error, path string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected a refusal, got nil", what)
	}
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("%s: error %v does not match ErrReadOnly — the API cannot tell a state conflict from a failure", what, err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("%s: error %q does not name %q, so the operator is not told what to fix", what, err, path)
	}
}

func TestSaveClassifiesAConfigThatWentUnwritableUnderIt(t *testing.T) {
	m, path := wentUnwritable(t)

	err := m.Save(map[string]interface{}{"log": map[string]interface{}{"level": "debug"}})
	assertReadOnlyNaming(t, "Save", err, path)
	if !m.IsReadOnly() {
		t.Fatal("the failed write must leave the manager in read-only mode, or the panel keeps offering to save")
	}
}

func TestSetDraftClassifiesAConfigThatWentUnwritableUnderIt(t *testing.T) {
	m, path := wentUnwritable(t)

	err := m.SetDraft(map[string]interface{}{"log": map[string]interface{}{"level": "debug"}})
	assertReadOnlyNaming(t, "SetDraft", err, path)
	if !m.IsReadOnly() {
		t.Fatal("the failed draft write must leave the manager in read-only mode")
	}
	// Same invariant as the pre-write refusal: a draft that cannot reach disk
	// must not stay in memory pretending the manager holds newer state than the
	// file does.
	if m.HasDraft() {
		t.Fatal("a draft that could not be written must not survive in memory")
	}
}

func TestApplyDraftClassifiesAConfigThatWentUnwritableUnderIt(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"log":{"level":"info"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SetDraft(map[string]interface{}{"log": map[string]interface{}{"level": "debug"}}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0700) })

	assertReadOnlyNaming(t, "ApplyDraft", m.ApplyDraft(), path)
	if !m.IsReadOnly() {
		t.Fatal("the failed apply must leave the manager in read-only mode")
	}
}

func TestSyncAwgEndpointActiveClassifiesAConfigThatWentUnwritableUnderIt(t *testing.T) {
	m, path := wentUnwritable(t)

	// changed is only meaningful when err is nil (the sync reports "a change was
	// attempted"), so what the refusal has to leave behind is the config: the
	// endpoint must not appear in memory when it never reached disk.
	_, err := m.SyncAwgEndpointActive("awg-server", specFixture())
	assertReadOnlyNaming(t, "SyncAwgEndpointActive", err, path)
	if _, ok := m.GetActive()["endpoints"]; ok {
		t.Fatal("a refused sync must not leave the endpoint in the active config")
	}
}

// The verdict must also come back down on its own. It used to be lifted only by
// the next write attempt, so on the live box it hung for 24 seconds until an
// unrelated background sweep happened to write the config — the moment of
// recovery belonged to whichever timer fired first. The guard re-probes a
// standing negative verdict on a plain read, so polling the status is enough.
func TestTheVerdictLiftsOnAStatusReadWithoutAnyWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the guard's re-probe interval")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}
	dir := filepath.Join(t.TempDir(), "etc")
	if err := os.Mkdir(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })
	m := NewEmptyManager(filepath.Join(dir, "config.json"))
	if !m.IsReadOnly() {
		t.Fatal("harness: a config in an unwritable dir must start read-only")
	}

	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !m.IsReadOnly() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("the verdict never lifted on a read: the panel's badge would wait for the next write")
}
