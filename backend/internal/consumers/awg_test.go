package consumers

import (
	"context"
	"testing"

	"routebox/backend/internal/awg"
	"routebox/backend/internal/quota"
)

func TestAwgSourceDirectionIsClientView(t *testing.T) {
	store := openStore(t)
	// What the sweep observer writes for a peer that only downloaded: Up=rx=0, Down=tx=500.
	_ = store.UpsertUser(60, awg.TrafficKey("PK"), 0, 500)
	src := &AwgSource{
		Peers: func(context.Context) []awg.PeerSummary {
			return []awg.PeerSummary{{Name: "alex", PublicKey: "PK", Address: "10.10.0.2/32", Online: true,
				Rx: 7, Tx: 9, QuotaBytes: 100, SuspendReason: quota.ReasonNone}}
		},
		Live: func(context.Context) (map[string]awg.PeerUsage, error) {
			return map[string]awg.PeerUsage{"PK": {Up: 0, Down: 500}}, nil
		},
		Store: store,
	}
	rows, err := src.List(0, 120)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	r := rows[0]
	if r.Upload != 0 || r.Download != 500 || r.ID != "PK" || !r.Online || r.Used != 16 || r.Address != "10.10.0.2/32" {
		t.Fatalf("row = %+v", r)
	}
	c, _ := src.Counters(context.Background())
	if c["PK"] != (Counter{Up: 0, Down: 500}) {
		t.Fatalf("counters = %v", c)
	}
}

func TestAwgSourceSuspendedPeer(t *testing.T) {
	src := &AwgSource{Peers: func(context.Context) []awg.PeerSummary {
		return []awg.PeerSummary{{PublicKey: "PK", SuspendReason: quota.ReasonExpired}}
	}}
	rows, _ := src.List(0, 1)
	if rows[0].State != StateSuspended || rows[0].StateReason != "expired" {
		t.Fatalf("row = %+v", rows[0])
	}
}
