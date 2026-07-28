package awg

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"routebox/backend/internal/config"
)

// A read-only sing-box config makes every managed-endpoint sync fail. The store
// holds the secrets, and it lives in a different directory that may well be
// writable — so nothing may be left in it that the endpoint does not carry.

func TestSingbox_AddPeer_ReadOnlyConfigFailsAndStoresNothing(t *testing.T) {
	m, fs, applyCount := newSingboxMgr(t)
	fs.err = errRO()

	if _, err := m.AddPeer(context.Background(), "alice"); err == nil {
		t.Fatal("AddPeer must fail when the config cannot be written")
	} else if !errors.Is(err, config.ErrReadOnly) {
		t.Fatalf("error %v must carry ErrReadOnly so the API can answer 409", err)
	}
	if got := len(m.store.List()); got != 0 {
		t.Fatalf("store kept %d peer(s) that never reached sing-box", got)
	}
	if *applyCount != 0 {
		t.Fatalf("apply must not run on a failed sync (calls=%d)", *applyCount)
	}
}

func TestSingbox_RemovePeer_ReadOnlyConfigKeepsTheSecret(t *testing.T) {
	m, fs, _ := newSingboxMgr(t)
	sum, err := m.AddPeer(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	fs.err = errRO()

	if _, err := m.RemovePeer(context.Background(), sum.PublicKey); err == nil {
		t.Fatal("RemovePeer must fail when the endpoint cannot be rewritten")
	}
	if _, ok := m.store.Get(sum.PublicKey); !ok {
		t.Fatal("the peer is still in the endpoint, so its secret must survive a failed removal")
	}
}

func TestSingbox_RenewPeer_ReadOnlyConfigKeepsTheOldExpiry(t *testing.T) {
	m, fs, _ := newSingboxMgr(t)
	sum, err := m.AddPeer(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	fs.err = errRO()

	if err := m.RenewPeer(context.Background(), sum.PublicKey, 4102444800); err == nil {
		t.Fatal("RenewPeer must fail when the endpoint cannot be rewritten")
	}
	p, ok := m.store.Get(sum.PublicKey)
	if !ok {
		t.Fatal("peer vanished from the store")
	}
	if p.ExpiresAt != 0 {
		t.Fatalf("store recorded expiry %d that never reached sing-box", p.ExpiresAt)
	}
}

func TestSingbox_Enable_ReadOnlyConfigFailsWithACauseTheAPICanClassify(t *testing.T) {
	m, fs, _ := newSingboxMgrDisabled(t)
	fs.err = errRO()

	err := m.Enable(context.Background(), EnableInput{
		Subnet: "10.10.0.0/24", ListenPort: 51820, MTU: 1408,
	})
	if err == nil {
		t.Fatal("Enable must fail when the managed endpoint cannot be written")
	}
	if !errors.Is(err, config.ErrReadOnly) {
		t.Fatalf("error %v lost its cause — the API would answer 500/400 instead of 409", err)
	}
	if m.Status(context.Background()).Enabled {
		t.Fatal("a failed enable must not report the server as enabled")
	}
}

// The sweep runs every ~30s. A read-only config is a standing condition the user
// already knows about from three other places, so it must not fill the journal —
// while any other sync failure still has to be visible.
func TestSingbox_Sweep_ReadOnlyIsQuietButOtherFailuresAreNot(t *testing.T) {
	m, fs, _ := newSingboxMgr(t)

	roErr := errRO() // built before capture: the manager logs its own verdict once

	var buf bytes.Buffer
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(prevOut); log.SetFlags(prevFlags) })

	fs.err = roErr
	m.SweepExpired(context.Background())
	if buf.Len() != 0 {
		t.Fatalf("read-only sweep logged every tick: %q", buf.String())
	}

	fs.err = errors.New("disk on fire")
	m.SweepExpired(context.Background())
	if !strings.Contains(buf.String(), "disk on fire") {
		t.Fatalf("a real sync failure must still be logged, got %q", buf.String())
	}
}

// errRO mimics what a read-only config.Manager returns from every write path.
func errRO() error {
	m := config.NewEmptyManager("/nonexistent-read-only/config.json")
	m.SetReadOnly(true)
	_, err := m.SyncAwgEndpointActive("awg-server", nil)
	if err == nil {
		panic("expected a read-only error from the config manager")
	}
	return err
}
