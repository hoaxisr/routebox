package awg

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// RenewPeer writes the new expiry to the store BEFORE it re-admits the peer.
// The singbox branch compensates when the sync fails; the kernel branch just
// returned admit's error, leaving the store — and therefore the panel — showing
// a renewal that never reached the interface or the .conf. The sweep does not
// heal it either: it only suspends peers whose expiry has passed.
func TestKernelRenewPeerRestoresTheExpiryWhenAdmitFails(t *testing.T) {
	ctx := context.Background()
	f := newFakeRunner()
	m := newTestManager(t, f)
	if err := os.MkdirAll(filepath.Dir(m.confPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.confPath, []byte("[Interface]\nListenPort = 51820\n"), 0600); err != nil {
		t.Fatal(err)
	}
	sum, err := m.AddPeer(ctx, "phone")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := m.store.Get(sum.PublicKey)
	if !ok {
		t.Fatal("peer not stored")
	}
	const expired = int64(1000)
	p.ExpiresAt = expired
	if err := m.store.Put(p); err != nil {
		t.Fatal(err)
	}

	// Take the .conf away: admit gets past `awg set` and fails on the rewrite,
	// exactly like a full disk or a read-only /etc would.
	if err := os.Remove(m.confPath); err != nil {
		t.Fatal(err)
	}

	if err := m.RenewPeer(ctx, sum.PublicKey, 4102444800); err == nil {
		t.Fatal("RenewPeer must fail when the peer cannot be re-admitted")
	}

	got, ok := m.store.Get(sum.PublicKey)
	if !ok {
		t.Fatal("the peer disappeared from the store")
	}
	if got.ExpiresAt != expired {
		t.Fatalf("stored expiry = %d, want the old %d — the panel would show a renewal that never happened", got.ExpiresAt, expired)
	}
}

// The happy path still persists the new expiry.
func TestKernelRenewPeerKeepsTheNewExpiryOnSuccess(t *testing.T) {
	ctx := context.Background()
	f := newFakeRunner()
	m := newTestManager(t, f)
	if err := os.MkdirAll(filepath.Dir(m.confPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.confPath, []byte("[Interface]\nListenPort = 51820\n"), 0600); err != nil {
		t.Fatal(err)
	}
	sum, err := m.AddPeer(ctx, "phone")
	if err != nil {
		t.Fatal(err)
	}
	const until = int64(4102444800)
	if err := m.RenewPeer(ctx, sum.PublicKey, until); err != nil {
		t.Fatal(err)
	}
	got, _ := m.store.Get(sum.PublicKey)
	if got.ExpiresAt != until {
		t.Fatalf("stored expiry = %d, want %d", got.ExpiresAt, until)
	}
}
