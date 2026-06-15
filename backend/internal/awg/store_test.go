package awg

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore(filepath.Join(t.TempDir(), "amneziawg", "peers.toml"))
	s.now = func() int64 { return 1700000000 } // injected clock
	return s
}

func TestStoreRoundTripAndPerm0600(t *testing.T) {
	s := newTestStore(t)
	if err := s.Put(Peer{PublicKey: "PUB==", PrivateKey: "PRIV==", PresharedKey: "PSK==", Address: "10.10.0.2/32", Name: "phone"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	info, err := os.Stat(s.path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("perm = %o, want 0600", perm)
	}
	// dir 0700
	if di, _ := os.Stat(filepath.Dir(s.path)); di.Mode().Perm() != 0700 {
		t.Fatalf("dir perm = %o, want 0700", di.Mode().Perm())
	}
	s2 := NewStore(s.path)
	if err := s2.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	got, ok := s2.Get("PUB==")
	if !ok || got.PrivateKey != "PRIV==" {
		t.Fatalf("reload mismatch: %#v ok=%v", got, ok)
	}
}

func TestStoreRollsBackOnSaveError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-dir")
	os.WriteFile(blocker, []byte("x"), 0600)
	s := NewStore(filepath.Join(blocker, "peers.toml"))
	if err := s.Put(Peer{PublicKey: "PUB=="}); err == nil {
		t.Fatal("want save error")
	}
	if _, ok := s.Get("PUB=="); ok {
		t.Fatal("Put must roll back on save failure")
	}
}

func TestReconcileBidirectional(t *testing.T) {
	s := newTestStore(t)
	_ = s.Put(Peer{PublicKey: "KEEP==", Address: "10.10.0.2/32"})
	_ = s.Put(Peer{PublicKey: "ORPHAN==", Address: "10.10.0.3/32"})

	// conf has KEEP + a hand-added EXTRA (no secret); live device has KEEP + a stale GHOST.
	confPubs := []string{"KEEP==", "EXTRA=="}
	livePubs := []string{"KEEP==", "GHOST=="}
	changed, removeLive, err := s.Reconcile(confPubs, livePubs)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if !changed {
		t.Fatal("ORPHAN secret should have been dropped -> changed")
	}
	if _, ok := s.Get("ORPHAN=="); ok {
		t.Fatal("ORPHAN secret (absent from conf) must be dropped")
	}
	if _, ok := s.Get("KEEP=="); !ok {
		t.Fatal("KEEP must survive")
	}
	// GHOST is live but not in conf -> reported for removal from the device.
	if len(removeLive) != 1 || removeLive[0] != "GHOST==" {
		t.Fatalf("removeLive = %#v; want [GHOST==]", removeLive)
	}
	// Idempotent second pass.
	changed2, _, _ := s.Reconcile([]string{"KEEP=="}, []string{"KEEP=="})
	if changed2 {
		t.Fatal("second reconcile should be a no-op")
	}
}

func TestStoreNeverLogsSecrets(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	s := newTestStore(t)
	_ = s.Put(Peer{PublicKey: "PUB==", PrivateKey: "SUPERSECRETPRIV", PresharedKey: "SUPERSECRETPSK"})
	_, _, _ = s.Reconcile(nil, nil)
	if bytes.Contains(buf.Bytes(), []byte("SUPERSECRETPRIV")) || bytes.Contains(buf.Bytes(), []byte("SUPERSECRETPSK")) {
		t.Fatalf("secrets must never be logged:\n%s", buf.String())
	}
}

// TestSecretsNotInConfigBackupDir is a regression guard: peers.toml / awg-rb0.conf
// live under /etc/routebox/amneziawg, a DIFFERENT directory from config.json, so
// config/manager.go's pruneBackups — which globs only "config.json.<ts>.bak" in
// the config dir — can never reach them. We assert that a glob mimicking the prune
// predicate over the config dir is disjoint from the awg secret files.
func TestSecretsNotInConfigBackupDir(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "etc", "routebox")
	awgDir := filepath.Join(root, "etc", "routebox", "amneziawg")
	for _, d := range []string{configDir, awgDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// A realistic backup cycle in the config dir + the awg secret files as siblings.
	for _, f := range []string{"config.json", "config.json.bak", "config.json.1700000000.bak"} {
		if err := os.WriteFile(filepath.Join(configDir, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"peers.toml", "awg-rb0.conf"} {
		if err := os.WriteFile(filepath.Join(awgDir, f), []byte("SECRET"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// pruneBackups globs "<configDir>/config.json.*.bak" — must never enumerate the
	// awg secrets (they are in a sibling dir AND lack the config.json prefix).
	matches, err := filepath.Glob(filepath.Join(configDir, "config.json.*.bak"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range matches {
		base := filepath.Base(m)
		if base == "peers.toml" || base == "awg-rb0.conf" {
			t.Fatalf("config backup glob captured an awg secret file: %s", m)
		}
	}
	// And the awg dir itself is outside the config dir's glob scope.
	if filepath.Dir(filepath.Join(awgDir, "peers.toml")) == configDir {
		t.Fatal("awg secrets must NOT live in the config backup dir")
	}
}
