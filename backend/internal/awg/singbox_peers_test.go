package awg

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// peerStatsOf builds an injected peerStatsFn from a fixed public-key-keyed map.
func peerStatsOf(stats map[string]PeerStat, err error) func() (map[string]PeerStat, error) {
	return func() (map[string]PeerStat, error) { return stats, err }
}

func TestListPeersSingbox_PeerStatsTakePriorityOverLiveness(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	sum, err := m.AddPeer(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}

	real := time.Now().Unix() - 10
	stale := time.Now().Unix() - 2*onlineWindowSec
	m.SetPeerStats(peerStatsOf(map[string]PeerStat{sum.PublicKey: {LastHandshake: real, TxBytes: 100, RxBytes: 200}}, nil))
	// The traffic fallback would say "stale" — peerStatsFn must win when both are wired.
	m.SetPeerLiveness(livenessOf(map[string]int64{tunnelIP(sum.Address): stale}))

	peers := m.ListPeers(context.Background())
	if len(peers) != 1 {
		t.Fatalf("ListPeers = %+v, want the one peer", peers)
	}
	if !peers[0].Online || peers[0].LastHandshake != real {
		t.Errorf("peer = %+v, want online with the real UAPI handshake %d", peers[0], real)
	}
	if peers[0].Tx != 100 || peers[0].Rx != 200 {
		t.Errorf("peer tx/rx = %d/%d, want 100/200 from the UAPI stats", peers[0].Tx, peers[0].Rx)
	}
}

func TestListPeersSingbox_UnsupportedStatsFallsBackToLiveness(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	sum, err := m.AddPeer(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}

	seen := time.Now().Unix() - 30
	m.SetPeerStats(peerStatsOf(nil, ErrAwgPeerStatsUnsupported))
	m.SetPeerLiveness(livenessOf(map[string]int64{tunnelIP(sum.Address): seen}))

	peers := m.ListPeers(context.Background())
	if !peers[0].Online || peers[0].LastHandshake != seen {
		t.Errorf("peer = %+v, want the traffic-liveness fallback to have applied", peers[0])
	}
	if peers[0].Tx != 0 || peers[0].Rx != 0 {
		t.Errorf("peer tx/rx = %d/%d, want 0/0 — the traffic fallback has no per-peer counters", peers[0].Tx, peers[0].Rx)
	}
}

func TestListPeersSingbox_PeerNotInStatsReadsOffline(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	if _, err := m.AddPeer(context.Background(), "alice"); err != nil {
		t.Fatal(err)
	}

	// Stats source is up and answering, just has no entry for this peer (never
	// handshaked at all) — must not fall through to the traffic fallback, or a
	// truly-never-connected peer could read "online" off unrelated traffic.
	m.SetPeerStats(peerStatsOf(map[string]PeerStat{}, nil))
	m.SetPeerLiveness(livenessOf(map[string]int64{"10.10.0.99": time.Now().Unix()}))

	peers := m.ListPeers(context.Background())
	if peers[0].Online || peers[0].LastHandshake != 0 {
		t.Errorf("peer = %+v, want offline with no last-seen", peers[0])
	}
}

func TestFetchAwgPeerStats_ParsesPeersAndAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/awg/awg-server/peers" {
			t.Errorf("path = %s, want /awg/awg-server/peers", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer s3cret" {
			t.Errorf("Authorization = %q, want Bearer s3cret", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"peers": []map[string]any{
				{"public_key": "abc=", "last_handshake": 1700000000, "tx_bytes": 10, "rx_bytes": 20},
			},
		})
	}))
	defer srv.Close()

	stats, err := FetchAwgPeerStats(srv.Listener.Addr().String(), "s3cret", "awg-server")
	if err != nil {
		t.Fatal(err)
	}
	want := PeerStat{LastHandshake: 1700000000, TxBytes: 10, RxBytes: 20}
	if got := stats["abc="]; got != want {
		t.Errorf("stats[abc=] = %+v, want %+v", got, want)
	}
}

func TestFetchAwgPeerStats_404IsUnsupported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := FetchAwgPeerStats(srv.Listener.Addr().String(), "", "awg-server")
	if err != ErrAwgPeerStatsUnsupported {
		t.Errorf("err = %v, want ErrAwgPeerStatsUnsupported", err)
	}
}

func TestFetchAwgPeerStats_OtherStatusIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := FetchAwgPeerStats(srv.Listener.Addr().String(), "", "awg-server")
	if err == nil || err == ErrAwgPeerStatsUnsupported {
		t.Errorf("err = %v, want a non-nil, non-unsupported error", err)
	}
}

// #75: the roster showed "never connected, 0 B" for peers whose numbers simply
// could not be read — on every amnezia-box predating the per-peer UAPI route
// that was EVERY peer, and the panel presented it as measured fact. Each row now
// carries where its numbers came from, so the UI can decline to state a figure
// it does not have.
func TestListPeersSingbox_RowsSayWhereTheirNumbersCameFrom(t *testing.T) {
	t.Run("real UAPI stats are live", func(t *testing.T) {
		m, _, _ := newSingboxMgr(t)
		sum, err := m.AddPeer(context.Background(), "alice")
		if err != nil {
			t.Fatal(err)
		}
		m.SetPeerStats(peerStatsOf(map[string]PeerStat{sum.PublicKey: {LastHandshake: time.Now().Unix()}}, nil))
		if got := m.ListPeers(context.Background())[0].Stats; got != PeerStatsLive {
			t.Errorf("Stats = %q, want %q", got, PeerStatsLive)
		}
	})

	t.Run("a binary without the route falls back, and says so", func(t *testing.T) {
		m, _, _ := newSingboxMgr(t)
		sum, err := m.AddPeer(context.Background(), "alice")
		if err != nil {
			t.Fatal(err)
		}
		m.SetPeerStats(peerStatsOf(nil, ErrAwgPeerStatsUnsupported))
		m.SetPeerLiveness(livenessOf(map[string]int64{tunnelIP(sum.Address): time.Now().Unix() - 5}))

		p := m.ListPeers(context.Background())[0]
		if p.Stats != PeerStatsApproximate {
			t.Errorf("Stats = %q, want %q", p.Stats, PeerStatsApproximate)
		}
		// Liveness is inferred; the byte counts are not available this way at all,
		// and the row must not be read as "moved no bytes".
		if !p.Online {
			t.Error("traffic-derived liveness was dropped")
		}
		if p.Rx != 0 || p.Tx != 0 {
			t.Errorf("rx/tx = %d/%d — the fallback has no byte counts to give", p.Rx, p.Tx)
		}
	})

	t.Run("nothing wired at all is unavailable, not zero", func(t *testing.T) {
		m, _, _ := newSingboxMgr(t)
		if _, err := m.AddPeer(context.Background(), "alice"); err != nil {
			t.Fatal(err)
		}
		if got := m.ListPeers(context.Background())[0].Stats; got != PeerStatsUnavailable {
			t.Errorf("Stats = %q, want %q", got, PeerStatsUnavailable)
		}
	})

	t.Run("a peer the device does not know is genuinely never-connected", func(t *testing.T) {
		m, _, _ := newSingboxMgr(t)
		if _, err := m.AddPeer(context.Background(), "alice"); err != nil {
			t.Fatal(err)
		}
		// The fetch worked; this peer is simply absent from it — that IS
		// "never handshaked", and calling it unknown would be its own lie.
		m.SetPeerStats(peerStatsOf(map[string]PeerStat{"someone-else": {LastHandshake: 1}}, nil))
		p := m.ListPeers(context.Background())[0]
		if p.Stats != PeerStatsLive {
			t.Errorf("Stats = %q, want %q", p.Stats, PeerStatsLive)
		}
		if p.LastHandshake != 0 || p.Online {
			t.Errorf("peer = %+v, want a real never-connected row", p)
		}
	})
}

// The kind says how much the numbers are worth; the reason says why. They are
// different questions, and the panel's advice hangs on the second: telling an
// operator to update a binary that is already current, or to check a proxy that
// is running, is worse than saying nothing.
func TestListPeersSingbox_ReasonIsNotTheKind(t *testing.T) {
	newWithPeer := func(t *testing.T) *Manager {
		t.Helper()
		m, _, _ := newSingboxMgr(t)
		if _, err := m.AddPeer(context.Background(), "alice"); err != nil {
			t.Fatal(err)
		}
		return m
	}

	t.Run("an old binary is unsupported, whether or not traffic can stand in", func(t *testing.T) {
		m := newWithPeer(t)
		m.SetPeerStats(peerStatsOf(nil, ErrAwgPeerStatsUnsupported))
		m.SetPeerLiveness(livenessOf(map[string]int64{}))
		p := m.ListPeers(context.Background())[0]
		if p.Stats != PeerStatsApproximate || p.StatsReason != PeerStatsReasonUnsupported {
			t.Errorf("with liveness: %q/%q, want approximate/unsupported", p.Stats, p.StatsReason)
		}

		// Monitoring off: nothing to infer from, but the CAUSE is still the binary,
		// and the advice must stay "update it" rather than "check the proxy".
		m2 := newWithPeer(t)
		m2.SetPeerStats(peerStatsOf(nil, ErrAwgPeerStatsUnsupported))
		p2 := m2.ListPeers(context.Background())[0]
		if p2.Stats != PeerStatsUnavailable || p2.StatsReason != PeerStatsReasonUnsupported {
			t.Errorf("without liveness: %q/%q, want unavailable/unsupported", p2.Stats, p2.StatsReason)
		}
	})

	t.Run("a current binary that did not answer is unreachable, not out of date", func(t *testing.T) {
		// What every Apply looks like: amnezia-box restarts, the 5s poll hits a
		// refused socket. Advising an update here is a false diagnosis.
		m := newWithPeer(t)
		m.SetPeerStats(peerStatsOf(nil, errors.New("connection refused")))
		m.SetPeerLiveness(livenessOf(map[string]int64{}))
		p := m.ListPeers(context.Background())[0]
		if p.StatsReason != PeerStatsReasonUnreachable {
			t.Errorf("StatsReason = %q, want %q", p.StatsReason, PeerStatsReasonUnreachable)
		}
	})

	t.Run("nothing wired at all blames neither", func(t *testing.T) {
		m := newWithPeer(t)
		p := m.ListPeers(context.Background())[0]
		if p.Stats != PeerStatsUnavailable || p.StatsReason != PeerStatsReasonNoSource {
			t.Errorf("%q/%q, want unavailable/no_source", p.Stats, p.StatsReason)
		}
	})

	t.Run("a working fetch carries no reason at all", func(t *testing.T) {
		m := newWithPeer(t)
		m.SetPeerStats(peerStatsOf(map[string]PeerStat{}, nil))
		p := m.ListPeers(context.Background())[0]
		if p.Stats != PeerStatsLive || p.StatsReason != PeerStatsReasonNone {
			t.Errorf("%q/%q, want live with no reason", p.Stats, p.StatsReason)
		}
	})
}
