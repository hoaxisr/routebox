package awg

import (
	"context"
	"testing"
	"time"
)

// The sing-box backend serves AWG from a userspace endpoint: there is no
// awg-rb0 and no `awg show`, so the kernel liveness path reported every peer as
// never-handshaked. That read as "offline" forever in the client roster and on
// the per-user monitor, for peers that were connected and passing traffic.
//
// These tests pin the substitute: recorded traffic for the peer's tunnel IP.

// livenessOf builds the injected source from bare IPs, the shape the traffic
// store returns.
func livenessOf(seen map[string]int64) func(int64) map[string]int64 {
	return func(int64) map[string]int64 { return seen }
}

func TestListPeersSingbox_RecentTrafficIsOnline(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	sum, err := m.AddPeer(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}

	seen := time.Now().Unix() - 30
	m.SetPeerLiveness(livenessOf(map[string]int64{tunnelIP(sum.Address): seen}))

	peers := m.ListPeers(context.Background())
	if len(peers) != 1 {
		t.Fatalf("ListPeers = %+v, want the one peer", peers)
	}
	if !peers[0].Online {
		t.Error("a peer that moved bytes 30s ago must read online")
	}
	if peers[0].LastHandshake != seen {
		t.Errorf("last handshake = %d, want the last-seen stamp %d", peers[0].LastHandshake, seen)
	}
}

func TestListPeersSingbox_StaleTrafficIsOffline(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	sum, err := m.AddPeer(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}

	// Outside the online window, but still reported — the roster shows "last
	// seen 2h ago", which is the whole point of carrying the timestamp.
	seen := time.Now().Unix() - 2*onlineWindowSec
	m.SetPeerLiveness(livenessOf(map[string]int64{tunnelIP(sum.Address): seen}))

	peers := m.ListPeers(context.Background())
	if peers[0].Online {
		t.Error("traffic older than the online window must not read online")
	}
	if peers[0].LastHandshake != seen {
		t.Errorf("last handshake = %d, want %d — a stale peer still has a last-seen", peers[0].LastHandshake, seen)
	}
}

func TestListPeersSingbox_NeverSeenPeer(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	if _, err := m.AddPeer(context.Background(), "alice"); err != nil {
		t.Fatal(err)
	}

	m.SetPeerLiveness(livenessOf(map[string]int64{"10.10.0.99": time.Now().Unix()}))

	peers := m.ListPeers(context.Background())
	if peers[0].Online || peers[0].LastHandshake != 0 {
		t.Errorf("peer = %+v, want offline with no last-seen — that IP is someone else's", peers[0])
	}
}

// Without a traffic store (no monitoring configured) the roster must still
// render. The singbox Manager has no Runner at all in this harness, so a
// regression that falls back to the kernel path panics here rather than
// silently shelling out.
func TestListPeersSingbox_WithoutALivenessSource(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	if _, err := m.AddPeer(context.Background(), "alice"); err != nil {
		t.Fatal(err)
	}

	peers := m.ListPeers(context.Background())
	if len(peers) != 1 || peers[0].Online {
		t.Errorf("ListPeers = %+v, want the peer, offline", peers)
	}
}

func TestTunnelIP(t *testing.T) {
	for in, want := range map[string]string{
		"10.10.0.2/32":  "10.10.0.2",
		"fd00::2/128":   "fd00::2",
		"10.10.0.2":     "10.10.0.2", // already bare
		"":              "",
		"not-an-adress": "not-an-adress",
	} {
		if got := tunnelIP(in); got != want {
			t.Errorf("tunnelIP(%q) = %q, want %q", in, got, want)
		}
	}
}
