package awg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestManager(t *testing.T, f *fakeRunner) *Manager {
	t.Helper()
	dir := t.TempDir()
	return &Manager{
		run:        f,
		confPath:   filepath.Join(dir, "amneziawg", "awg-rb0.conf"),
		store:      NewStore(filepath.Join(dir, "amneziawg", "peers.toml")),
		iface:      "awg-rb0",
		pskTmpDir:  dir,
		subnet:     "10.10.0.0/24",
		serverIP:   "10.10.0.1",
		listenPort: 51820,
		publicHost: "vpn.example.com",
	}
}

func TestAddPeerLiveAndPersist(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	if err := os.MkdirAll(filepath.Dir(m.confPath), 0700); err != nil {
		t.Fatal(err)
	}
	// seed a minimal server conf so the used-set read finds the file.
	os.WriteFile(m.confPath, []byte("[Interface]\nListenPort = 51820\n"), 0600)

	sum, err := m.AddPeer(context.Background(), "phone")
	if err != nil {
		t.Fatalf("AddPeer: %v", err)
	}
	if sum.Address == "" || sum.PublicKey == "" {
		t.Fatalf("summary missing fields: %#v", sum)
	}
	// Live `awg set` invoked with the peer.
	if !f.sawContains("awg set awg-rb0 peer " + sum.PublicKey) {
		t.Fatalf("expected live awg set; calls=%v", f.calls)
	}
	// PSK temp file was created 0600 then removed (no leftover in pskTmpDir).
	matches, _ := filepath.Glob(filepath.Join(m.pskTmpDir, "*.psk"))
	if len(matches) != 0 {
		t.Fatalf("PSK temp file leaked: %v", matches)
	}
	// [Peer] block appended to .conf; secret persisted.
	data, _ := os.ReadFile(m.confPath)
	if !strings.Contains(string(data), sum.PublicKey) {
		t.Fatalf(".conf missing the new peer:\n%s", data)
	}
	if _, ok := m.store.Get(sum.PublicKey); !ok {
		t.Fatal("secret not persisted")
	}
}

func TestAddPeerConcurrentDistinctIPs(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	os.MkdirAll(filepath.Dir(m.confPath), 0700)
	os.WriteFile(m.confPath, []byte("[Interface]\nListenPort = 51820\n"), 0600)

	const n = 8
	ips := make(chan string, n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			s, err := m.AddPeer(context.Background(), "c")
			if err != nil {
				errs <- err
				return
			}
			ips <- s.Address
		}()
	}
	seen := map[string]bool{}
	for i := 0; i < n; i++ {
		select {
		case e := <-errs:
			t.Fatalf("AddPeer: %v", e)
		case ip := <-ips:
			if seen[ip] {
				t.Fatalf("duplicate IP allocated: %s", ip)
			}
			seen[ip] = true
		}
	}
}

func TestSyncConfUsesTempFileNotProcessSubstitution(t *testing.T) {
	f := newFakeRunner()
	f.outputs["awg-quick strip awg-rb0"] = "[Interface]\n"
	m := newTestManager(t, f)
	if err := m.iface_SyncConf(context.Background()); err != nil {
		t.Fatalf("SyncConf: %v", err)
	}
	for _, c := range f.calls {
		joined := strings.Join(c, " ")
		if strings.Contains(joined, "<(") || c[0] == "sh" || c[0] == "bash" {
			t.Fatalf("SyncConf must not use process-substitution/shell: %v", c)
		}
	}
	if !f.sawContains("awg syncconf awg-rb0") {
		t.Fatalf("expected awg syncconf with a temp file; calls=%v", f.calls)
	}
}

func TestPeerLinesExcludesExpired(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	m.store.now = func() int64 { return 1000 }
	_ = m.store.Put(Peer{PublicKey: "live", Address: "10.10.0.2/32", ExpiresAt: 0})      // never
	_ = m.store.Put(Peer{PublicKey: "future", Address: "10.10.0.3/32", ExpiresAt: 2000}) // active
	_ = m.store.Put(Peer{PublicKey: "gone", Address: "10.10.0.4/32", ExpiresAt: 1000})   // expired (now>=exp)

	got := map[string]bool{}
	for _, pl := range m.peerLines() {
		got[pl.PublicKey] = true
	}
	if !got["live"] || !got["future"] || got["gone"] {
		t.Fatalf("peerLines filter wrong: %v", got)
	}
}

func TestAddPeerReservesSuspendedIP(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	m.store.now = func() int64 { return 1000 }
	os.MkdirAll(filepath.Dir(m.confPath), 0700)
	// conf has ONLY the interface — no peer blocks (the suspended peer is off-conf)
	os.WriteFile(m.confPath, []byte("[Interface]\nListenPort = 51820\n"), 0600)
	// a suspended peer holding .2 lives only in the store
	_ = m.store.Put(Peer{PublicKey: "susp", Address: "10.10.0.2/32", ExpiresAt: 500})

	sum, err := m.AddPeer(context.Background(), "new")
	if err != nil {
		t.Fatalf("AddPeer: %v", err)
	}
	if sum.Address == "10.10.0.2/32" {
		t.Fatalf("AddPeer reused a suspended peer's IP: %s", sum.Address)
	}
}
