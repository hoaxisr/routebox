package awg

import (
	"context"
	"encoding/json"
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
