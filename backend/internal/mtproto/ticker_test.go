package mtproto

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type flushRow struct {
	bucket int64
	key    string
	up     int64
	down   int64
}

// recordingSink stands in for traffic.Store, which needs a database.
type recordingSink struct {
	mu   sync.Mutex
	rows []flushRow
	err  error
}

func (r *recordingSink) UpsertUser(bucket int64, key string, up, down int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.err != nil {
		return r.err
	}

	r.rows = append(r.rows, flushRow{bucket, key, up, down})

	return nil
}

func (r *recordingSink) snapshot() []flushRow {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]flushRow(nil), r.rows...)
}

// staticSource hands the flush loop one stream, standing in for a Manager.
type staticSource struct{ es *EventStream }

func (s staticSource) Events() *EventStream { return s.es }

func TestFlushWritesNamespacedRows(t *testing.T) {
	es := NewEventStream([]string{"alice"})
	sink := &recordingSink{}

	match(es, "s1", 0)
	traffic(es, "s1", 20, true)
	traffic(es, "s1", 10, false)

	FlushTotals(es, sink, time.Unix(1830, 0))

	rows := sink.snapshot()
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want 1", rows)
	}

	// The prefix is what keeps these out of the panel-users view, which reads
	// user_traffic by explicit panel-user name.
	if rows[0].key != "mtproto:alice" {
		t.Errorf("key = %q, want mtproto:alice", rows[0].key)
	}

	if rows[0].up != 10 || rows[0].down != 20 {
		t.Errorf("got %d up / %d down, want 10/20", rows[0].up, rows[0].down)
	}
}

func TestFlushTruncatesToTheMinuteBucket(t *testing.T) {
	es := NewEventStream([]string{"alice"})
	sink := &recordingSink{}

	match(es, "s1", 0)
	traffic(es, "s1", 1, true)

	FlushTotals(es, sink, time.Unix(1837, 0))

	// The existing user_traffic rows are minute buckets; a stray second would
	// create rows the history queries cannot line up.
	if got := sink.snapshot()[0].bucket; got != 1800 {
		t.Errorf("bucket = %d, want 1800", got)
	}
}

func TestFlushWritesNothingWhenIdle(t *testing.T) {
	es := NewEventStream([]string{"alice"})
	sink := &recordingSink{}

	FlushTotals(es, sink, time.Unix(1800, 0))

	// Empty buckets would pad the history with rows meaning "no data".
	if rows := sink.snapshot(); len(rows) != 0 {
		t.Errorf("rows = %+v, want none", rows)
	}
}

func TestFlushDrainsSoBytesAreNotBilledTwice(t *testing.T) {
	es := NewEventStream([]string{"alice"})
	sink := &recordingSink{}

	match(es, "s1", 0)
	traffic(es, "s1", 50, true)

	FlushTotals(es, sink, time.Unix(1800, 0))
	FlushTotals(es, sink, time.Unix(1860, 0))

	if rows := sink.snapshot(); len(rows) != 1 {
		t.Errorf("rows = %+v, want only the first flush to have written", rows)
	}
}

func TestFlushToleratesANilStreamAndSink(t *testing.T) {
	// Events() is nil until the first Start, and the loop runs regardless.
	FlushTotals(nil, &recordingSink{}, time.Unix(1800, 0))
	FlushTotals(NewEventStream(nil), nil, time.Unix(1800, 0))
}

func TestFlushKeepsGoingAfterASinkError(t *testing.T) {
	es := NewEventStream([]string{"alice", "bob"})
	sink := &recordingSink{err: errors.New("disk is gone")}

	match(es, "s1", 0)
	match(es, "s2", 1)
	traffic(es, "s1", 5, true)
	traffic(es, "s2", 7, true)

	// A failing write must not abort the flush; the other client's row is
	// independent and losing it too would compound the problem.
	FlushTotals(es, sink, time.Unix(1800, 0))
}

func TestRunFlushLoopFlushesOnStop(t *testing.T) {
	es := NewEventStream([]string{"alice"})
	sink := &recordingSink{}
	stop := make(chan struct{})

	done := make(chan struct{})

	go func() {
		defer close(done)

		RunFlushLoop(staticSource{es}, sink, time.Hour, stop)
	}()

	match(es, "s1", 0)
	traffic(es, "s1", 99, true)

	close(stop)
	<-done

	// Bytes from the last partial interval must not be lost on a clean stop.
	rows := sink.snapshot()
	if len(rows) != 1 || rows[0].down != 99 {
		t.Errorf("rows = %+v, want the final flush to have written 99", rows)
	}
}

func TestSweepExpiredDisablesPastDueClients(t *testing.T) {
	s := tempStore(t)
	now := time.Unix(1_000_000, 0)

	mustPut(t, s, Client{Name: "gone", Secret: secretA, Enabled: true, ExpiresAt: now.Unix() - 1})
	mustPut(t, s, Client{Name: "stays", Secret: secretB, Enabled: true})

	if !SweepExpired(s, now) {
		t.Error("SweepExpired must report a change so the caller rebuilds")
	}

	if c, _ := s.Get("gone"); c.Enabled {
		t.Error("the expired client is still enabled")
	}

	if c, _ := s.Get("stays"); !c.Enabled {
		t.Error("an unexpired client must be left alone")
	}
}

func TestSweepExpiredKeepsTheClientVisible(t *testing.T) {
	s := tempStore(t)
	now := time.Unix(1_000_000, 0)

	mustPut(t, s, Client{Name: "gone", Secret: secretA, Enabled: true, ExpiresAt: now.Unix() - 1})
	SweepExpired(s, now)

	// Expiry disables rather than deletes: the row stays on the page with its
	// history, and an admin can extend it instead of issuing a new secret.
	if _, ok := s.Get("gone"); !ok {
		t.Error("an expired client must remain in the roster")
	}
}

func TestSweepExpiredReportsNoChangeWhenNothingLapsed(t *testing.T) {
	s := tempStore(t)
	mustPut(t, s, Client{Name: "stays", Secret: secretA, Enabled: true})

	if SweepExpired(s, time.Unix(1_000_000, 0)) {
		t.Error("nothing expired, so no rebuild should be triggered")
	}
}

func TestSweepExpiredIgnoresAlreadyDisabledClients(t *testing.T) {
	s := tempStore(t)
	now := time.Unix(1_000_000, 0)

	mustPut(t, s, Client{Name: "off", Secret: secretA, Enabled: false, ExpiresAt: now.Unix() - 1})

	// Re-disabling would rewrite the file and trigger a pointless rebuild on
	// every tick for as long as the client exists.
	if SweepExpired(s, now) {
		t.Error("an already-disabled expired client must not report a change")
	}
}

func TestTrafficKeyIsPrefixed(t *testing.T) {
	if !strings.HasPrefix(TrafficKey("alice"), TrafficKeyPrefix) {
		t.Errorf("TrafficKey(alice) = %q, want the mtproto: prefix", TrafficKey("alice"))
	}
}
