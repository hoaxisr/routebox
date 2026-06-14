package v2stats

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeQuerier returns canned stats; no gRPC.
type fakeQuerier struct {
	stats []*Stat
	err   error
}

func (f fakeQuerier) QueryStats(_ context.Context, _ string, _ bool) ([]*Stat, error) {
	return f.stats, f.err
}

func TestAggregateStats(t *testing.T) {
	in := []*Stat{
		{Name: "user>>>alice>>>traffic>>>uplink", Value: 100},
		{Name: "user>>>alice>>>traffic>>>downlink", Value: 250},
		{Name: "user>>>bob>>>traffic>>>uplink", Value: 7},
		{Name: "inbound>>>tun>>>traffic>>>uplink", Value: 9999}, // ignored
		{Name: "garbage", Value: 1},                             // ignored
	}
	got := aggregateStats(in)
	if len(got) != 2 {
		t.Fatalf("users = %d, want 2 (alice,bob)", len(got))
	}
	if got["alice"].Uplink != 100 || got["alice"].Downlink != 250 {
		t.Errorf("alice = %+v, want {100,250}", got["alice"])
	}
	if got["bob"].Uplink != 7 || got["bob"].Downlink != 0 {
		t.Errorf("bob = %+v, want {7,0}", got["bob"])
	}
}

func TestQueryUsers_UsesQuerierResult(t *testing.T) {
	c := &Client{q: fakeQuerier{stats: []*Stat{
		{Name: "user>>>alice>>>traffic>>>uplink", Value: 5},
		{Name: "user>>>alice>>>traffic>>>downlink", Value: 6},
	}}}
	got, err := c.QueryUsers(context.Background())
	if err != nil {
		t.Fatalf("QueryUsers: %v", err)
	}
	if got["alice"].Uplink != 5 || got["alice"].Downlink != 6 {
		t.Errorf("alice = %+v, want {5,6}", got["alice"])
	}
}

func TestQueryUsers_PropagatesError(t *testing.T) {
	c := &Client{q: fakeQuerier{err: errors.New("boom")}}
	if _, err := c.QueryUsers(context.Background()); err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestDial_DeadAddressFailsAtRPC(t *testing.T) {
	// grpc.NewClient is lazy — Dial succeeds; the RPC against a dead port fails.
	c, err := Dial("127.0.0.1:1")
	if err != nil {
		t.Fatalf("Dial should be lazy, got %v", err)
	}
	defer c.Close()
	if _, err := c.QueryUsersTimeout(2 * time.Second); err == nil {
		t.Fatal("expected RPC error against dead address")
	}
}
