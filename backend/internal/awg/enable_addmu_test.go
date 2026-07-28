package awg

import (
	"context"
	"os"
	"testing"
	"time"
)

// Enable rewrites the server .conf from scratch (interface head + every stored
// peer) and then commits the canonical parameters it rendered from. AddPeer
// holds addMu around a read-modify-write of that same file — allocate a /32 from
// the conf, `awg set` the peer, rewrite the conf without it, append the block —
// and reads m.subnet/m.serverIP to allocate from.
//
// Without addMu, the two interleave: Enable's full rewrite lands between
// AddPeer's read and its append and the peer is gone from the file (live until
// the next awg-quick restart, then silently not), or AddPeer allocates from the
// old subnet into a conf Enable is about to replace with a different one.
func TestEnableRewritesTheConfUnderAddMu(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	f.outputs["awg show awg-rb0"] = "interface: awg-rb0\n  listening port: 51820\n"
	f.outputs["iptables -t nat -S"] = "-N RBOX-AWG-NAT\n"

	m.addMu.Lock()
	done := make(chan error, 1)
	go func() { done <- m.Enable(context.Background(), goodEnableInput()) }()

	select {
	case err := <-done:
		m.addMu.Unlock()
		t.Fatalf("Enable ran to completion while addMu was held (err=%v)", err)
	case <-time.After(200 * time.Millisecond):
	}
	if _, err := os.Stat(m.confPath); err == nil {
		m.addMu.Unlock()
		t.Fatal("Enable rewrote the .conf while an AddPeer critical section held addMu")
	}
	m.addMu.Unlock()

	if err := <-done; err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if _, err := os.Stat(m.confPath); err != nil {
		t.Fatalf("Enable must render the .conf once it has the lock: %v", err)
	}
}

// The singbox backend has the same pair: the managed endpoint is the file, and
// addPeerSingbox stores the secret and then re-renders the endpoint from the
// store, both under addMu. An Enable that renders its own spec in between syncs
// a snapshot taken before the peer existed and writes the new peer straight back
// out of the config — a client the panel lists and sing-box has never heard of.
func TestSingbox_EnableSyncsTheEndpointUnderAddMu(t *testing.T) {
	m, fs, _ := newSingboxMgr(t)

	m.addMu.Lock()
	done := make(chan error, 1)
	go func() { done <- m.Enable(context.Background(), singboxEnableInput()) }()

	select {
	case err := <-done:
		m.addMu.Unlock()
		t.Fatalf("Enable ran to completion while addMu was held (err=%v)", err)
	case <-time.After(200 * time.Millisecond):
	}
	if fs.calls != 0 {
		m.addMu.Unlock()
		t.Fatalf("Enable wrote the endpoint (%d sync calls) while a peer op held addMu", fs.calls)
	}
	m.addMu.Unlock()

	if err := <-done; err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if fs.calls == 0 {
		t.Fatal("Enable must sync the endpoint once it has the lock")
	}
}
