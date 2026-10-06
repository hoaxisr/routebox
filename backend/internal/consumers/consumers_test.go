package consumers

import (
	"context"
	"errors"
	"testing"

	"routebox/backend/internal/quota"
)

type fakeSource struct {
	kind     string
	rows     []Row
	listErr  error
	counters map[string]Counter
	err      error
}

func (f *fakeSource) Kind() string                         { return f.kind }
func (f *fakeSource) List(start, end int64) ([]Row, error) { return f.rows, f.listErr }
func (f *fakeSource) Counters(context.Context) (map[string]Counter, error) {
	return f.counters, f.err
}

func TestListMergesSourcesAndSkipsFailingOne(t *testing.T) {
	got := List([]Source{
		&fakeSource{kind: "user", rows: []Row{{Kind: "user", ID: "u1"}}},
		&fakeSource{kind: "awg", listErr: errors.New("db locked")},
		&fakeSource{kind: "lan", rows: []Row{{Kind: "lan", ID: "192.168.1.5"}}},
	}, 0, 100)
	if len(got) != 2 || got[0].ID != "u1" || got[1].ID != "192.168.1.5" {
		t.Fatalf("got %+v", got)
	}
	if got[0].History == nil {
		t.Fatal("history must be [] not null in JSON")
	}
}

func TestListNoSourcesIsEmptyNotNil(t *testing.T) {
	if got := List(nil, 0, 100); got == nil || len(got) != 0 {
		t.Fatalf("got %#v, want empty non-nil slice", got)
	}
}

func TestStateOf(t *testing.T) {
	for _, c := range []struct {
		in           quota.Reason
		state, cause string
	}{
		{quota.ReasonNone, StateActive, ""},
		{quota.ReasonManual, StateDisabled, ""},
		{quota.ReasonQuota, StateSuspended, "quota"},
		{quota.ReasonExpired, StateSuspended, "expired"},
	} {
		if s, r := stateOf(c.in); s != c.state || r != c.cause {
			t.Errorf("stateOf(%q) = %q,%q want %q,%q", c.in, s, r, c.state, c.cause)
		}
	}
}

func TestKeyedNilStoreIsZeros(t *testing.T) {
	up, down, hist, err := keyed(nil, 0, 100, []string{"awg:k"})
	if err != nil || up != 0 || down != 0 || hist != nil {
		t.Fatalf("keyed(nil) = %d,%d,%v,%v", up, down, hist, err)
	}
}
