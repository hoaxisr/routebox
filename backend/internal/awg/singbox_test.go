package awg

import (
	"context"
	"crypto/rand"
	"testing"

	"routebox/backend/internal/config"
)

type fakeSync struct {
	lastSpec *config.AwgServerSpec
	changed  bool
	calls    int
}

func (f *fakeSync) SyncAwgEndpointActive(tag string, spec *config.AwgServerSpec) (bool, error) {
	f.calls++
	f.lastSpec = spec
	return f.changed, nil
}

func newSingboxMgr(t *testing.T) (*Manager, *fakeSync, *int) {
	t.Helper()
	priv, _, err := Generate(rand.Reader) // real 32-byte key so PublicFromPrivate paths work
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{
		store:      NewStore(""), // in-memory
		iface:      "awg-rb0",
		subnet:     "10.10.0.0/24",
		serverIP:   "10.10.0.1",
		listenPort: 51820,
		mtu:        1408,
		serverPriv: priv,
	}
	fs := &fakeSync{changed: true}
	applyCount := 0
	m.SetBackend("singbox")
	m.SetConfigSync(fs, func() error { applyCount++; return nil }, func() bool { return true })
	return m, fs, &applyCount
}

func TestSingbox_AddPeer_SyncsAndApplies(t *testing.T) {
	m, fs, applyCount := newSingboxMgr(t)
	sum, err := m.AddPeer(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	if sum.Address == "" {
		t.Fatal("expected an allocated /32 address")
	}
	if fs.calls == 0 || fs.lastSpec == nil || len(fs.lastSpec.Peers) != 1 {
		t.Fatalf("sync not called with 1 peer: calls=%d spec=%#v", fs.calls, fs.lastSpec)
	}
	if *applyCount != 1 {
		t.Fatalf("apply calls = %d, want 1", *applyCount)
	}
	// The synced peer has NO address/port (server-side) — enforced by BuildAwgServerEndpoint.
	if fs.lastSpec.Peers[0].AllowedIP == "" {
		t.Fatal("peer AllowedIP missing")
	}
}

func TestSingbox_ExpiredPeerOmittedFromSpec(t *testing.T) {
	m, fs, _ := newSingboxMgr(t)
	m.AddPeer(context.Background(), "bob")
	pk := fs.lastSpec.Peers[0].PublicKey
	// Expire it in the past and re-sync.
	m.RenewPeer(context.Background(), pk, 1) // unix ts 1 = long past
	if len(fs.lastSpec.Peers) != 0 {
		t.Fatalf("expired peer must be omitted from spec, got %d", len(fs.lastSpec.Peers))
	}
}
