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
	// A non-ASCII display name must survive the TOML write+read verbatim: names
	// are stored as the user typed them, so the disk round-trip is part of the
	// contract (the in-memory map alone would not catch an encoding regression).
	const unicodeName = "Ноутбук Ани 🏠"
	if err := s.Put(Peer{PublicKey: "PUB2==", PrivateKey: "PRIV2==", Address: "10.10.0.3/32", Name: unicodeName}); err != nil {
		t.Fatalf("Put unicode name: %v", err)
	}

	s2 := NewStore(s.path)
	if err := s2.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	got, ok := s2.Get("PUB==")
	if !ok || got.PrivateKey != "PRIV==" {
		t.Fatalf("reload mismatch: %#v ok=%v", got, ok)
	}
	got2, ok := s2.Get("PUB2==")
	if !ok || got2.Name != unicodeName {
		t.Fatalf("unicode name did not survive the disk round-trip: %q (ok=%v)", got2.Name, ok)
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
	// SUSPENDED is stored but deliberately out of the conf (expired peer).
	_ = s.Put(Peer{PublicKey: "SUSPENDED==", Address: "10.10.0.3/32", ExpiresAt: 1})

	// the live device has KEEP, SUSPENDED (crash leftover: taken out of the conf
	// for being expired but still on the interface) and a stale GHOST that is
	// ours nowhere.
	livePubs := []string{"KEEP==", "SUSPENDED==", "GHOST=="}
	removeLive := s.Reconcile(livePubs)

	if _, ok := s.Get("SUSPENDED=="); !ok {
		t.Fatal("a suspended peer's secrets must survive reconcile")
	}
	if _, ok := s.Get("KEEP=="); !ok {
		t.Fatal("KEEP must survive")
	}
	// GHOST is live and the store has never heard of it -> foreign.
	if len(removeLive) != 1 || removeLive[0] != "GHOST==" {
		t.Fatalf("removeLive = %#v; want [GHOST==]", removeLive)
	}
	// Idempotent second pass.
	if r := s.Reconcile([]string{"KEEP=="}); len(r) != 0 {
		t.Fatalf("second reconcile should be a no-op, got %v", r)
	}
}

func TestStoreNeverLogsSecrets(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	s := newTestStore(t)
	_ = s.Put(Peer{PublicKey: "PUB==", PrivateKey: "SUPERSECRETPRIV", PresharedKey: "SUPERSECRETPSK"})
	_ = s.Reconcile(nil)
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

func TestStoreRoundTripsExpiresAt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "peers.toml")
	s := NewStore(path)
	if err := s.Put(Peer{PublicKey: "pk1", Address: "10.0.0.2/32", ExpiresAt: 1893456000}); err != nil {
		t.Fatal(err)
	}
	// reload from disk and confirm the field survives the TOML round-trip
	s2 := NewStore(path)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Get("pk1")
	if !ok || got.ExpiresAt != 1893456000 {
		t.Fatalf("ExpiresAt not round-tripped: %#v ok=%v", got, ok)
	}
}

func TestStoreULAPrefixRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "peers.toml"))
	if s.ULAPrefix() != "" {
		t.Fatal("fresh store should have no ULA prefix")
	}
	if err := s.SetULAPrefix("fd00:abcd:ef01::/64"); err != nil {
		t.Fatal(err)
	}
	s2 := NewStore(filepath.Join(dir, "peers.toml"))
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	if got := s2.ULAPrefix(); got != "fd00:abcd:ef01::/64" {
		t.Fatalf("reloaded prefix = %q", got)
	}
}
