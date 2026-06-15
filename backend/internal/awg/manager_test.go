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
