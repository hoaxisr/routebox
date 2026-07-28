package awg

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A suspended (expired) peer is intentionally absent from the conf — peerLines
// skips it so Enable/Apply cannot resurrect it. Reconcile must therefore never
// treat "absent from conf" as "delete the secret", or RenewPeer becomes
// impossible for every expired client.
func TestReconcileKeepsSuspendedPeers(t *testing.T) {
	s := newTestStore(t)
	suspended := "SUSPENDEDpubkeyAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	if err := s.Put(Peer{Name: "expired", PublicKey: suspended, Address: "10.9.0.5/32", ExpiresAt: 1}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	removeLive := s.Reconcile(nil)

	if _, ok := s.Get(suspended); !ok {
		t.Fatal("Reconcile must never drop a suspended peer's secrets")
	}
	if len(removeLive) != 0 {
		t.Fatalf("nothing to remove from the interface, got %v", removeLive)
	}
}

// A suspended peer still present on the live interface (e.g. a crash between
// `awg set remove` and the conf rewrite) is OURS: the store knows it, so it must
// not be reported as foreign.
func TestReconcileSuspendedLivePeerIsNotForeign(t *testing.T) {
	s := newTestStore(t)
	suspended := "SUSPENDEDpubkeyAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	if err := s.Put(Peer{Name: "expired", PublicKey: suspended, Address: "10.9.0.5/32", ExpiresAt: 1}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if removeLive := s.Reconcile([]string{suspended}); len(removeLive) != 0 {
		t.Fatalf("a stored peer is never foreign, got %v", removeLive)
	}
}

func TestReconcileReturnsForeignLivePeers(t *testing.T) {
	s := newTestStore(t)
	mine := "MINEpubkeyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	foreign := "FOREIGNpubkeyAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	if err := s.Put(Peer{Name: "mine", PublicKey: mine, Address: "10.9.0.2/32"}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	removeLive := s.Reconcile([]string{mine, foreign})

	if len(removeLive) != 1 || removeLive[0] != foreign {
		t.Fatalf("want only the foreign peer removed, got %v", removeLive)
	}
	if _, ok := s.Get(mine); !ok {
		t.Fatal("stored peers must survive")
	}
}

// --- wiring: Enable sweeps foreign peers off the live kernel interface ---

// enableShow is the `awg show <iface>` output the health gate accepts, with the
// given live peers appended in the real format.
func enableShow(peers ...string) string {
	var b strings.Builder
	b.WriteString("interface: awg-rb0\n  listening port: 51820\n")
	for _, p := range peers {
		b.WriteString("\npeer: " + p + "\n  latest handshake: 1 minute ago\n")
	}
	return b.String()
}

func TestEnableRemovesForeignLivePeers(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	mine := "MINEpubkeyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	suspended := "SUSPENDEDpubkeyAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	foreign := "FOREIGNpubkeyAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	if err := m.store.Put(Peer{Name: "mine", PublicKey: mine, Address: "10.10.0.2/32"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := m.store.Put(Peer{Name: "expired", PublicKey: suspended, Address: "10.10.0.3/32", ExpiresAt: 1}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	f.outputs["awg show awg-rb0"] = enableShow(mine, suspended, foreign)
	f.outputs["iptables -t nat -S"] = "-N RBOX-AWG-NAT\n-N RBOX-AWG-FWD\n-N RBOX-AWG-IN\n"

	if err := m.Enable(context.Background(), goodEnableInput()); err != nil {
		t.Fatalf("Enable: %v", err)
	}

	if !f.sawContains("awg set awg-rb0 peer " + foreign + " remove") {
		t.Fatalf("foreign live peer must be swept off the interface; calls=%v", f.calls)
	}
	for _, ours := range []string{mine, suspended} {
		if f.sawContains("awg set awg-rb0 peer " + ours + " remove") {
			t.Fatalf("our peer %s must never be swept off the interface; calls=%v", ours, f.calls)
		}
	}
	if _, ok := m.store.Get(suspended); !ok {
		t.Fatal("the suspended peer's secrets must survive Enable")
	}
}

// Enable is the only thing that ever swept foreign peers off the interface, so
// a leftover of a crash or of a manual `awg set` sat there until the operator
// happened to re-Apply the server — which on a box that is working fine is
// never. The 30s sweep is the one thing that runs on its own, so it has to do
// this too, including on the common tick where nothing is expired at all.
func TestSweepExpiredRemovesForeignLivePeers(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	mine := "MINEpubkeyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	foreign := "FOREIGNpubkeyAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	if err := m.store.Put(Peer{Name: "mine", PublicKey: mine, Address: "10.10.0.2/32"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	f.outputs["awg show awg-rb0"] = enableShow(mine, foreign)

	m.SweepExpired(context.Background())

	if !f.sawContains("awg set awg-rb0 peer " + foreign + " remove") {
		t.Fatalf("the sweep must strip a foreign live peer; calls=%v", f.calls)
	}
	if f.sawContains("awg set awg-rb0 peer " + mine + " remove") {
		t.Fatalf("a stored peer is never foreign; calls=%v", f.calls)
	}
}

// The suspended peer is the one that makes this delicate: it is deliberately out
// of the conf and still in the store, the sweep takes it off the interface for
// being expired, and its secrets have to survive — without them the renewal the
// operator will eventually run is impossible.
func TestSweepExpiredKeepsSuspendedSecretsWhileSweepingForeignPeers(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	suspended := "SUSPENDEDpubkeyAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	foreign := "FOREIGNpubkeyAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	if err := m.store.Put(Peer{Name: "expired", PublicKey: suspended, Address: "10.10.0.3/32", ExpiresAt: 1}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(m.confPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.confPath, []byte("[Interface]\nListenPort = 51820\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.outputs["awg show awg-rb0"] = enableShow(suspended, foreign)

	m.SweepExpired(context.Background())

	if !f.sawContains("awg set awg-rb0 peer " + foreign + " remove") {
		t.Fatalf("the sweep must strip a foreign live peer; calls=%v", f.calls)
	}
	if _, ok := m.store.Get(suspended); !ok {
		t.Fatal("a suspended peer's secrets must survive the sweep, or renewal becomes impossible")
	}
}

// A failed sweep is reported, never swallowed — but it must not fail Enable: the
// tunnel is up and serving, a leftover foreign peer is not worth a teardown.
func TestEnableReportsFailedForeignPeerRemoval(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	f := newFakeRunner()
	m := newEnableManager(t, f)
	foreign := "FOREIGNpubkeyAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	f.outputs["awg show awg-rb0"] = enableShow(foreign)
	f.outputs["iptables -t nat -S"] = "-N RBOX-AWG-NAT\n-N RBOX-AWG-FWD\n-N RBOX-AWG-IN\n"
	f.errs["awg set awg-rb0 peer "+foreign+" remove"] = errFake

	if err := m.Enable(context.Background(), goodEnableInput()); err != nil {
		t.Fatalf("a failed sweep must not fail Enable: %v", err)
	}
	if !strings.Contains(buf.String(), foreign) {
		t.Fatalf("a failed sweep must be logged, got:\n%s", buf.String())
	}
}
