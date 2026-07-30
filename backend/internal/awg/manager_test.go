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

func seedConf(t *testing.T, m *Manager) {
	t.Helper()
	os.MkdirAll(filepath.Dir(m.confPath), 0700)
	os.WriteFile(m.confPath, []byte("[Interface]\nListenPort = 51820\n"), 0600)
}

// validPub / otherValidPub are real 32-byte std-base64 keys (ValidatePublicKey is a
// 32-byte-decode length check), required because RenewPeer validates its pub arg.
const validPub = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEs="
const otherValidPub = "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI="

func TestRenewPeerReadmitsSuspended(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	seedConf(t, m)
	m.store.now = func() int64 { return 1000 }
	// a suspended peer: expired, off the conf, secret retained
	_ = m.store.Put(Peer{PublicKey: validPub, PresharedKey: "psk", Address: "10.10.0.2/32", Name: "bob", ExpiresAt: 500})

	if err := m.RenewPeer(context.Background(), validPub, 5000); err != nil {
		t.Fatalf("RenewPeer: %v", err)
	}
	// stored expiry updated
	got, _ := m.store.Get(validPub)
	if got.ExpiresAt != 5000 {
		t.Fatalf("ExpiresAt not updated: %d", got.ExpiresAt)
	}
	// re-admitted live with the SAME key + IP
	if !f.sawContains("awg set awg-rb0 peer " + validPub) {
		t.Fatalf("expected live re-admit; calls=%v", f.calls)
	}
	// [Peer] block back in the conf, exactly once
	data, _ := os.ReadFile(m.confPath)
	if n := strings.Count(string(data), "PublicKey = "+validPub); n != 1 {
		t.Fatalf("expected exactly one conf block, got %d:\n%s", n, data)
	}
}

func TestRenewPeerUnknown(t *testing.T) {
	m := newTestManager(t, newFakeRunner())
	seedConf(t, m)
	if err := m.RenewPeer(context.Background(), otherValidPub, 5000); err != ErrPeerNotFound {
		t.Fatalf("want ErrPeerNotFound, got %v", err)
	}
}

func TestSweepExpiredSuspendsLivePeer(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	seedConf(t, m)
	m.store.now = func() int64 { return 2000 }
	// expired peer present in store AND live on the interface
	_ = m.store.Put(Peer{PublicKey: "old", PresharedKey: "p", Address: "10.10.0.2/32", ExpiresAt: 1000})
	// make iface_ShowPeers (`awg show <iface>`) report it live. The fakeRunner returns
	// scripted output keyed by the joined argv; parseShowPeers reads "peer: <key>" lines.
	f.outputs["awg show awg-rb0"] = "peer: old\n"
	// also put its block in the conf so we can prove it gets rewritten out
	m.appendPeerToConf(PeerLine{Name: "x", PublicKey: "old", PSK: "p", AllowedIP: "10.10.0.2/32"})

	m.SweepExpired(context.Background())

	if !f.sawContains("awg set awg-rb0 peer old remove") {
		t.Fatalf("expected live remove; calls=%v", f.calls)
	}
	data, _ := os.ReadFile(m.confPath)
	if strings.Contains(string(data), "PublicKey = old") {
		t.Fatalf("expired peer still in conf:\n%s", data)
	}
	// secret KEPT for later renewal
	if _, ok := m.store.Get("old"); !ok {
		t.Fatal("SweepExpired must keep the store secret")
	}
}

func TestSweepExpiredSkipsRenewedPeer(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	seedConf(t, m)
	// peer is expired at snapshot time and live on the interface
	_ = m.store.Put(Peer{PublicKey: "old", PresharedKey: "p", Address: "10.10.0.2/32", ExpiresAt: 1000})
	f.outputs["awg show awg-rb0"] = "peer: old\n"
	m.appendPeerToConf(PeerLine{Name: "x", PublicKey: "old", PSK: "p", AllowedIP: "10.10.0.2/32"})
	m.store.now = func() int64 { return 2000 }

	// A renewal that lands just before the sweep looks must be honoured, or the
	// peer ends up off the interface with the store calling it active — a state
	// nothing heals. RenewPeer writes the new expiry under addMu, so holding the
	// lock here reproduces the ordering the real thing is forced into: the sweep
	// cannot read the store until the renewal is in it.
	m.addMu.Lock()
	done := make(chan struct{})
	go func() { defer close(done); m.SweepExpired(context.Background()) }()
	p, _ := m.store.Get("old")
	p.ExpiresAt = 999999
	if err := m.store.Put(p); err != nil {
		m.addMu.Unlock()
		t.Fatal(err)
	}
	m.addMu.Unlock()
	<-done

	// the renewed peer must NOT be suspended: no live remove, conf block intact
	if f.sawContains("awg set awg-rb0 peer old remove") {
		t.Fatalf("renewed-during-sweep peer was wrongly suspended; calls=%v", f.calls)
	}
	data, _ := os.ReadFile(m.confPath)
	if !strings.Contains(string(data), "PublicKey = old") {
		t.Fatalf("renewed peer's conf block was removed:\n%s", data)
	}
}

func TestRenewPeerNoDuplicateConfBlock(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	seedConf(t, m)
	m.store.now = func() int64 { return 1000 }
	// active peer already present in the conf
	_ = m.store.Put(Peer{PublicKey: validPub, PresharedKey: "psk", Address: "10.10.0.2/32", Name: "bob", ExpiresAt: 5000})
	m.appendPeerToConf(PeerLine{Name: "bob", PublicKey: validPub, PSK: "psk", AllowedIP: "10.10.0.2/32"})

	if err := m.RenewPeer(context.Background(), validPub, 9000); err != nil {
		t.Fatalf("RenewPeer: %v", err)
	}
	data, _ := os.ReadFile(m.confPath)
	if n := strings.Count(string(data), "PublicKey = "+validPub); n != 1 {
		t.Fatalf("admit must upsert (one block), got %d:\n%s", n, data)
	}
}

// A non-empty m.headerKey must surface as HeaderProtectionKey in the rendered
// client conf (awg3, sing-box backend). The kernel Enable path clears the field,
// so this pins the render plumbing the kernel-leak fix depends on.
func TestRenderClientConfEmitsHeaderProtectionKey(t *testing.T) {
	const hpk = "TESTHPK000000000000000000000000000000000000=="
	f := newFakeRunner()
	m := newTestManager(t, f)
	m.serverPriv = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEs=" // any 32-byte std-base64
	m.headerKey = hpk
	_ = m.store.Put(Peer{PublicKey: validPub, PrivateKey: "cpriv", PresharedKey: "psk", Address: "10.10.0.2/32", Name: "bob"})

	conf, err := m.RenderClientConf(validPub, "1.2.3.4")
	if err != nil {
		t.Fatalf("RenderClientConf: %v", err)
	}
	if !strings.Contains(conf, "HeaderProtectionKey = "+hpk) {
		t.Fatalf("client conf missing HeaderProtectionKey:\n%s", conf)
	}
}

// An empty DNS field means "the client keeps its own resolver", not "silently
// hand it 1.1.1.1". The invented default overrode routing rules that worked
// before the tunnel came up, and nothing in the UI admitted it was there (#45).
func TestRenderClientConfOmitsDNSWhenUnset(t *testing.T) {
	seed := func(t *testing.T, dns []string) string {
		t.Helper()
		m := newTestManager(t, newFakeRunner())
		m.serverPriv = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEs="
		m.dns = dns
		_ = m.store.Put(Peer{PublicKey: validPub, PrivateKey: "cpriv", Address: "10.10.0.2/32", Name: "bob"})
		conf, err := m.RenderClientConf(validPub, "1.2.3.4")
		if err != nil {
			t.Fatalf("RenderClientConf: %v", err)
		}
		return conf
	}

	for _, empty := range [][]string{nil, {}} {
		if conf := seed(t, empty); strings.Contains(conf, "DNS = ") {
			t.Fatalf("unset DNS must emit no DNS line, got:\n%s", conf)
		}
	}
	// A configured resolver still lands in the .conf.
	if conf := seed(t, []string{"10.10.0.1", "9.9.9.9"}); !strings.Contains(conf, "DNS = 10.10.0.1, 9.9.9.9\n") {
		t.Fatalf("configured DNS must be emitted, got:\n%s", conf)
	}
}
