package consumers

import (
	"context"
	"net"
	"testing"

	"github.com/9seconds/mtg/v2/mtglib"

	"routebox/backend/internal/mtproto"
)

func TestMtprotoSourceRowsAndCounters(t *testing.T) {
	store := openStore(t)
	_ = store.UpsertUser(60, mtproto.TrafficKey("mama"), 5, 50)
	es := mtproto.NewEventStream([]string{"mama"})
	ctx := context.Background()
	es.Send(ctx, mtglib.NewEventStart("s1", net.IPv4(1, 2, 3, 4)))
	es.Send(ctx, mtglib.NewEventClientMatched("s1", 0))
	es.Send(ctx, mtglib.NewEventTraffic("s1", 700, true))

	src := &MtprotoSource{
		Clients: func() []mtproto.Client {
			return []mtproto.Client{{Name: "mama", Enabled: true}, {Name: "old", Enabled: false}}
		},
		Events: func() *mtproto.EventStream { return es },
		Store:  store,
		Now:    func() int64 { return 120 },
	}
	if src.Kind() != "mtproto" {
		t.Fatalf("kind = %q", src.Kind())
	}
	rows, err := src.List(0, 120)
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	if rows[0].Kind != "mtproto" || rows[0].ID != "mama" || rows[0].Name != "mama" || rows[0].Upload != 5 || rows[0].Download != 50 || rows[0].State != StateActive {
		t.Fatalf("rows = %+v", rows)
	}
	if len(rows[0].History) == 0 {
		t.Fatalf("history must come from the store, row = %+v", rows[0])
	}
	if rows[1].ID != "old" || rows[1].State != StateDisabled || rows[1].StateReason != "" {
		t.Fatalf("rows = %+v", rows)
	}
	c, err := src.Counters(ctx)
	if err != nil || c["mama"] != (Counter{Down: 700}) {
		t.Fatalf("counters = %v err=%v", c, err)
	}
}

func TestMtprotoSourceSortsByNameAndFlagsExpiry(t *testing.T) {
	src := &MtprotoSource{
		Clients: func() []mtproto.Client {
			return []mtproto.Client{{Name: "zeta", Enabled: true, ExpiresAt: 100}, {Name: "alpha", Enabled: true}}
		},
		Events: func() *mtproto.EventStream { return nil },
		Now:    func() int64 { return 200 },
	}
	rows, err := src.List(0, 200)
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	if rows[0].ID != "alpha" || rows[1].ID != "zeta" {
		t.Fatalf("order = %+v", rows)
	}
	if rows[1].State != StateSuspended || rows[1].StateReason != "expired" || rows[1].ExpiresAt != 100 {
		t.Fatalf("expired row = %+v", rows[1])
	}
}

func TestMtprotoSourceNotStartedHasNoCounters(t *testing.T) {
	src := &MtprotoSource{Clients: func() []mtproto.Client { return nil }, Events: func() *mtproto.EventStream { return nil }}
	c, err := src.Counters(context.Background())
	if err != nil || c != nil {
		t.Fatalf("got %v err=%v", c, err)
	}
	rows, err := src.List(0, 1)
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
}

func TestMtprotoSourceListDoesNotMutateCallerSlice(t *testing.T) {
	orig := []mtproto.Client{{Name: "b", Enabled: true}, {Name: "a", Enabled: true}}
	src := &MtprotoSource{
		Clients: func() []mtproto.Client { return orig },
		Events:  func() *mtproto.EventStream { return nil },
		Now:     func() int64 { return 1 },
	}
	if _, err := src.List(0, 1); err != nil {
		t.Fatal(err)
	}
	if orig[0].Name != "b" || orig[1].Name != "a" {
		t.Fatalf("caller's slice was reordered: %+v", orig)
	}
}
