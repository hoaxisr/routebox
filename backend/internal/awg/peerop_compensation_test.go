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

// breakStore points the store's file at a path under a REGULAR FILE, so every
// save from here on fails with ENOTDIR — whatever the process runs as, and
// without disturbing what is already in memory.
func breakStore(t *testing.T, m *Manager) {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	m.store.path = filepath.Join(blocker, "peers.toml")
}

// The kernel RemovePeer takes the peer off the interface and out of the .conf
// first and deletes the secret second. When the delete fails the peer is gone
// from everywhere that serves it while the store — and so the panel — still
// lists it as an active client. Nothing heals that: it has not expired, and the
// store owning it is exactly what makes it OURS to the foreign-peer sweep.
//
// The singbox branch of the same method already compensates. admit is
// idempotent, so putting the peer back is the whole fix.
func TestRemovePeerKernelPutsThePeerBackWhenItsSecretCannotBeDeleted(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	seedConf(t, m)
	sum, err := m.AddPeer(context.Background(), "phone")
	if err != nil {
		t.Fatal(err)
	}
	breakStore(t, m)

	if _, err := m.RemovePeer(context.Background(), sum.PublicKey); err == nil {
		t.Fatal("RemovePeer must fail when the secret cannot be deleted")
	}

	if _, ok := m.store.Get(sum.PublicKey); !ok {
		t.Fatal("the store kept nothing to compensate for — nothing to test")
	}
	// The store still lists it, so the interface and the conf have to carry it
	// too: one `awg set ... preshared-key` for the original add, a second for the
	// re-admit.
	admits := 0
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c, " "), sum.PublicKey+" preshared-key") {
			admits++
		}
	}
	if admits != 2 {
		t.Fatalf("peer admitted %d time(s), want 2 (add + re-admit); calls=%v", admits, f.calls)
	}
	data, _ := os.ReadFile(m.confPath)
	if !strings.Contains(string(data), sum.PublicKey) {
		t.Fatalf("the peer the store still lists is missing from the .conf:\n%s", data)
	}
}

// Restoring a snapshot means restoring what was there, not more. A suspended
// (expired) peer lives in the store ONLY — peerLines keeps it out of the conf
// and the sweep took it off the interface — so re-admitting it on the way out of
// a failed delete would put an expired key back into service until the next tick
// noticed. The store rolls its own failed Delete back, so there is nothing else
// to put back: the secret is there, the interface is not, exactly as before.
func TestRemovePeerKernelDoesNotReAdmitAnExpiredPeer(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	seedConf(t, m)
	m.store.now = func() int64 { return 2000 }
	if err := m.store.Put(Peer{PublicKey: validPub, PresharedKey: "p", Address: "10.10.0.2/32", ExpiresAt: 1000}); err != nil {
		t.Fatal(err)
	}
	breakStore(t, m)

	if _, err := m.RemovePeer(context.Background(), validPub); err == nil {
		t.Fatal("RemovePeer must fail when the secret cannot be deleted")
	}

	if _, ok := m.store.Get(validPub); !ok {
		t.Fatal("the failed delete must leave the secret in the store")
	}
	if f.sawContains(validPub + " preshared-key") {
		t.Fatalf("an expired peer was put back into service; calls=%v", f.calls)
	}
}

// Both writes failing is the one case with nowhere left to go. It must not be
// silent: the peer is off the interface and the store still claims it.
func TestRemovePeerKernelReportsAFailedCompensation(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	f := newFakeRunner()
	m := newTestManager(t, f)
	seedConf(t, m)
	sum, err := m.AddPeer(context.Background(), "phone")
	if err != nil {
		t.Fatal(err)
	}
	breakStore(t, m)
	f.errsContains[" preshared-key "] = errFake // the re-admit fails too

	if _, err := m.RemovePeer(context.Background(), sum.PublicKey); err == nil {
		t.Fatal("RemovePeer must fail when the secret cannot be deleted")
	}
	if !strings.Contains(buf.String(), sum.PublicKey) {
		t.Fatalf("a peer stranded by two failed writes must be logged, got:\n%s", buf.String())
	}
}

// AddPeer's rollback has the same duty as every other compensation in the
// package: when it fails, say so. The singbox branch logs it; the kernel branch
// discarded the error, so a peer live on the interface with no secret anywhere
// left no trace at all.
func TestAddPeerKernelReportsAFailedRollback(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	f := newFakeRunner()
	m := newTestManager(t, f)
	seedConf(t, m)
	breakStore(t, m)
	f.errsContains[" remove"] = errFake // the rollback's `awg set ... remove` fails too

	if _, err := m.AddPeer(context.Background(), "phone"); err == nil {
		t.Fatal("AddPeer must fail when the secret cannot be persisted")
	}
	if !strings.Contains(buf.String(), "could not be rolled back") {
		t.Fatalf("a failed rollback must be logged, got:\n%s", buf.String())
	}
}
