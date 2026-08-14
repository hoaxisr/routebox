package awg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// With header protection off the server is AWG 2.0 to any client, so no AWG3-only
// key may reach the export — the iOS AmneziaWG rejects the whole config over one
// (#64) and AWGM mislabels it 3.0 (#60). Every obfuscation preset but "off" fills
// these device-timers, so this used to fire for ordinary AWG 2.0 servers.
func TestRenderClientConfStripsAwg3WhenHeaderProtectionOff(t *testing.T) {
	awg3Obf := Obfuscation{
		CPA: "0-48", RAT: "120-150", RekeyTimeout: "5",
		RejectAfterTime: "180-210", KeepaliveTimeout: "10-25", MaxHandshakeAttempts: "18",
	}
	awg3Keys := []string{"ContentPaddingAddition", "RekeyAfterTime", "RekeyTimeout",
		"RejectAfterTime", "KeepaliveTimeout", "MaxHandshakeAttempts"}

	render := func(t *testing.T, headerKey string) string {
		t.Helper()
		m := newTestManager(t, newFakeRunner())
		m.serverPriv = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEs="
		m.obf, m.headerKey = awg3Obf, headerKey
		_ = m.store.Put(Peer{PublicKey: validPub, PrivateKey: "cpriv", Address: "10.10.0.2/32", Name: "bob"})
		conf, err := m.RenderClientConf(validPub, "1.2.3.4")
		if err != nil {
			t.Fatalf("RenderClientConf: %v", err)
		}
		return conf
	}

	conf := render(t, "")
	for _, k := range awg3Keys {
		if strings.Contains(conf, k) {
			t.Fatalf("header protection off, but %s leaked into the client conf:\n%s", k, conf)
		}
	}

	// ...and they must still be there when it is on, or the fix broke awg3.
	conf = render(t, "TESTHPK000000000000000000000000000000000000==")
	for _, k := range awg3Keys {
		if !strings.Contains(conf, k) {
			t.Fatalf("header protection on, but %s is missing from the client conf:\n%s", k, conf)
		}
	}
}

// A "lo-hi" PersistentKeepalive is AWG 3.0-only as well, and nothing ties it to
// header protection, so it went on leaking into 2.0 exports after the fields above
// were dealt with — same rejection, one line further down the file.
func TestRenderClientConfCollapsesKeepaliveRangeWhenHeaderProtectionOff(t *testing.T) {
	render := func(t *testing.T, headerKey string) string {
		t.Helper()
		m := newTestManager(t, newFakeRunner())
		m.serverPriv = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEs="
		m.headerKey = headerKey
		m.desired = func() EnableInput { return EnableInput{ClientKeepalive: "20-30"} }
		_ = m.store.Put(Peer{PublicKey: validPub, PrivateKey: "cpriv", Address: "10.10.0.2/32", Name: "bob"})
		conf, err := m.RenderClientConf(validPub, "1.2.3.4")
		if err != nil {
			t.Fatalf("RenderClientConf: %v", err)
		}
		return conf
	}

	if conf := render(t, ""); !strings.Contains(conf, "PersistentKeepalive = 20\n") {
		t.Fatalf("a ranged keepalive reached a non-awg3 export:\n%s", conf)
	}
	if conf := render(t, "TESTHPK000000000000000000000000000000000000=="); !strings.Contains(conf, "PersistentKeepalive = 20-30") {
		t.Fatalf("header protection on, but the keepalive range was collapsed anyway:\n%s", conf)
	}
}

// The vpn:// link is the other half of #60 — AWGM reads the link, not the .conf,
// and hasAwg3() switches its schema on exactly the fields stripped above.
func TestRenderVPNLinkCarriesNoAwg3WhenHeaderProtectionOff(t *testing.T) {
	m := newTestManager(t, newFakeRunner())
	m.serverPriv = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEs="
	m.obf = Obfuscation{
		Jc: 4, Jmin: 40, Jmax: 70, S1: 15, S2: 20, S3: 16, S4: 12,
		H1: "1-2000", H2: "700000-800000", H3: "1200000-1300000", H4: "1700000-1800000",
		CPA: "0-48", RAT: "120-150", RekeyTimeout: "5",
		RejectAfterTime: "180-210", KeepaliveTimeout: "10-25", MaxHandshakeAttempts: "18",
	}
	_ = m.store.Put(Peer{PublicKey: validPub, PrivateKey: "cpriv", Address: "10.10.0.2/32", Name: "bob"})

	link, err := m.RenderVPNLink(validPub, "vpn.example.com")
	if err != nil {
		t.Fatalf("RenderVPNLink: %v", err)
	}
	// Flatten the whole payload — the awg3 values live both in the embedded .conf
	// text and, for the key, in the container's own fields.
	decoded, err := json.Marshal(decodeLink(t, link))
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	for _, k := range []string{"ContentPaddingAddition", "RekeyAfterTime", "RekeyTimeout",
		"RejectAfterTime", "KeepaliveTimeout", "MaxHandshakeAttempts", "HeaderProtectionKey"} {
		if strings.Contains(string(decoded), k) {
			t.Fatalf("header protection off, but %s reached the vpn:// link:\n%s", k, decoded)
		}
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

// DNS is client-only, so Status never marks it dirty and there is no Apply
// button — the export must therefore read the SAVED value, not the Enable-time
// snapshot, or a changed resolver keeps shipping the old one until a re-enable.
func TestRenderClientConfUsesSavedDNS(t *testing.T) {
	m := newTestManager(t, newFakeRunner())
	m.serverPriv = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEs="
	m.dns = []string{"1.1.1.1"} // what was running at Enable time
	_ = m.store.Put(Peer{PublicKey: validPub, PrivateKey: "cpriv", Address: "10.10.0.2/32", Name: "bob"})

	m.SetDesired(func() EnableInput { return EnableInput{DNS: []string{"9.9.9.9"}} })
	conf, err := m.RenderClientConf(validPub, "1.2.3.4")
	if err != nil {
		t.Fatalf("RenderClientConf: %v", err)
	}
	if !strings.Contains(conf, "DNS = 9.9.9.9\n") {
		t.Fatalf("client conf must carry the saved DNS, got:\n%s", conf)
	}

	// Garbage in settings falls back to the running snapshot rather than
	// producing a conf with no resolver at all.
	m.SetDesired(func() EnableInput { return EnableInput{DNS: []string{"not-an-ip"}} })
	if conf, err = m.RenderClientConf(validPub, "1.2.3.4"); err != nil {
		t.Fatalf("RenderClientConf: %v", err)
	}
	if !strings.Contains(conf, "DNS = 1.1.1.1\n") {
		t.Fatalf("invalid saved DNS must fall back to the snapshot, got:\n%s", conf)
	}
}

// awg3Manager builds a manager whose server runs the full awg3 parameter set.
// supports3 chooses whether the running binary is claimed to understand them.
func awg3Manager(t *testing.T, supports3 bool) *Manager {
	t.Helper()
	m := newTestManager(t, newFakeRunner())
	m.serverPriv = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEs="
	m.headerKey = "TESTHPK000000000000000000000000000000000000=="
	m.obf = Obfuscation{
		Jc: 3, Jmin: 10, Jmax: 30, S1: 15, S2: 18, S3: 20, S4: 23,
		H1: "100-200", H2: "300-400", H3: "500-600", H4: "700-800",
		CPA: "0-64", RAT: "120-150", RekeyTimeout: "5",
		RejectAfterTime: "180-210", KeepaliveTimeout: "10-25", MaxHandshakeAttempts: "18",
	}
	m.supports3Fn = func() bool { return supports3 }
	_ = m.store.Put(Peer{PublicKey: validPub, PrivateKey: "cpriv", Address: "10.10.0.2/32", Name: "bob"})
	return m
}

var awg3Keys = []string{
	"HeaderProtectionKey", "ContentPaddingAddition", "RekeyAfterTime",
	"RekeyTimeout", "RejectAfterTime", "KeepaliveTimeout", "MaxHandshakeAttempts",
}

// awg3Values are the expected values for awg3Keys given awg3Manager's fixture
// (headerKey and obf above). Shared by the .conf and link "supported" assertions
// so a gate that strips everything but the header key cannot pass either.
var awg3Values = map[string]string{
	"HeaderProtectionKey":    "TESTHPK000000000000000000000000000000000000==",
	"ContentPaddingAddition": "0-64",
	"RekeyAfterTime":         "120-150",
	"RekeyTimeout":           "5",
	"RejectAfterTime":        "180-210",
	"KeepaliveTimeout":       "10-25",
	"MaxHandshakeAttempts":   "18",
}

// The awg3 capability gate belongs to the shared assembly, not to one exporter.
// ClientEndpoint has always stripped these on a pre-awg3 binary; RenderClientConf
// did not, so the .conf and the sing-box export disagreed about the same peer.
func TestRenderClientConfStripsAwg3OnOldBinary(t *testing.T) {
	conf, err := awg3Manager(t, false).RenderClientConf(validPub, "1.2.3.4")
	if err != nil {
		t.Fatalf("RenderClientConf: %v", err)
	}
	for _, k := range awg3Keys {
		if strings.Contains(conf, k) {
			t.Errorf("%s must not be emitted on a pre-awg3 binary:\n%s", k, conf)
		}
	}
	// The AWG 2.0 fields are unaffected.
	if !strings.Contains(conf, "Jc = 3") || !strings.Contains(conf, "H1 = 100-200") {
		t.Errorf("AWG 2.0 obfuscation must survive:\n%s", conf)
	}

	conf, err = awg3Manager(t, true).RenderClientConf(validPub, "1.2.3.4")
	if err != nil {
		t.Fatalf("RenderClientConf: %v", err)
	}
	// An awg3 binary must emit every awg3 field, not just the header key — a gate
	// that strips CPA/RAT/device-timers unconditionally would still pass a check
	// that only looked for HeaderProtectionKey.
	for _, k := range awg3Keys {
		want := fmt.Sprintf("%s = %s", k, awg3Values[k])
		if !strings.Contains(conf, want) {
			t.Errorf("an awg3 binary must emit %q:\n%s", want, conf)
		}
	}
}

// The gate must reach the link too, not just the .conf — a client told to use awg3
// against a server that is not running it cannot connect.
func TestRenderVPNLinkStripsAwg3OnOldBinary(t *testing.T) {
	link, err := awg3Manager(t, false).RenderVPNLink(validPub, "vpn.example.com")
	if err != nil {
		t.Fatalf("RenderVPNLink: %v", err)
	}
	last := lastConfig(t, decodeLink(t, link))
	for _, k := range awg3Keys {
		if _, ok := last[k]; ok {
			t.Errorf("%s must be absent on a pre-awg3 binary", k)
		}
	}
	// It is still an AWG peer: the AWG 2.0 fields survive.
	if last["Jc"] != "3" || last["H1"] != "100-200" {
		t.Errorf("AWG 2.0 obfuscation must survive: Jc=%v H1=%v", last["Jc"], last["H1"])
	}
}

// The mirror of the .conf "supported" assertion above: an awg3 binary must emit
// every awg3 field into the link too, not just the header key.
func TestRenderVPNLinkKeepsAwg3WhenSupported(t *testing.T) {
	link, err := awg3Manager(t, true).RenderVPNLink(validPub, "vpn.example.com")
	if err != nil {
		t.Fatalf("RenderVPNLink: %v", err)
	}
	last := lastConfig(t, decodeLink(t, link))
	for _, k := range awg3Keys {
		if got := last[k]; got != awg3Values[k] {
			t.Errorf("%s = %v, want %s", k, got, awg3Values[k])
		}
	}
}

// RenderVPNLink must propagate ErrLinkUnrepresentable unwrapped (via errors.Is),
// not swallow it behind a generic error — the handler (Task 5) distinguishes a
// 422-worthy unrepresentable peer from a 500 by errors.Is(err, ErrLinkUnrepresentable).
func TestRenderVPNLinkUnrepresentablePropagatesSentinel(t *testing.T) {
	m := awg3Manager(t, true)
	m.obf.H2 = "" // partial obfuscation: header magic incomplete, AmneziaLink must refuse
	_, err := m.RenderVPNLink(validPub, "vpn.example.com")
	if !errors.Is(err, ErrLinkUnrepresentable) {
		t.Fatalf("err = %v, want ErrLinkUnrepresentable", err)
	}
}

func TestRenderVPNLinkUsesPeerNameAndHost(t *testing.T) {
	outer := decodeLink(t, func() string {
		link, err := awg3Manager(t, true).RenderVPNLink(validPub, "vpn.example.com")
		if err != nil {
			t.Fatalf("RenderVPNLink: %v", err)
		}
		return link
	}())
	if outer["description"] != "bob — vpn.example.com" {
		t.Fatalf("description = %v", outer["description"])
	}
	if outer["hostName"] != "vpn.example.com" {
		t.Fatalf("hostName = %v", outer["hostName"])
	}
}

func TestRenderVPNLinkUnknownPeer(t *testing.T) {
	m := newTestManager(t, newFakeRunner())
	m.serverPriv = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEs="
	_, err := m.RenderVPNLink(validPub, "vpn.example.com")
	if err == nil {
		t.Fatal("an unknown peer must error")
	}
	if errors.Is(err, ErrLinkUnrepresentable) {
		t.Fatalf("err = %v, want a not-found error, not ErrLinkUnrepresentable (that maps to HTTP 422, not 404)", err)
	}
}
