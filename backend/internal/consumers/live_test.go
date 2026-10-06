package consumers

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// newTestLive returns a Live whose clock the test advances by hand; tests call
// tick directly instead of running the goroutine.
func newTestLive(src ...Source) (*Live, *time.Time) {
	now := time.Unix(1000, 0)
	l := NewLive(src)
	l.now = func() time.Time { return now }
	return l, &now
}

func rowOf(s LiveSnapshot, kind, id string) (LiveRow, bool) {
	for _, r := range s.Rows {
		if r.Kind == kind && r.ID == id {
			return r, true
		}
	}
	return LiveRow{}, false
}

func TestLiveRateFromTwoSnapshots(t *testing.T) {
	src := &fakeSource{kind: "awg", counters: map[string]Counter{"P": {Up: 0, Down: 0}}}
	l, now := newTestLive(src)
	l.tick(context.Background())
	if _, ok := rowOf(l.snapshot(), "awg", "P"); ok {
		t.Fatal("first snapshot only primes: no rate yet")
	}
	*now = now.Add(2 * time.Second)
	src.counters = map[string]Counter{"P": {Up: 200, Down: 2000}}
	l.tick(context.Background())
	r, ok := rowOf(l.snapshot(), "awg", "P")
	if !ok || r.DownBps != 1000 || r.UpBps != 100 || len(r.Trend) != 1 {
		t.Fatalf("got %+v ok=%v", r, ok)
	}
}

func TestLiveCounterResetGivesZeroRate(t *testing.T) {
	src := &fakeSource{kind: "awg", counters: map[string]Counter{"P": {Down: 9_000_000}}}
	l, now := newTestLive(src)
	l.tick(context.Background())
	*now = now.Add(2 * time.Second)
	src.counters = map[string]Counter{"P": {Down: 500}} // amnezia-box restarted
	l.tick(context.Background())
	r, _ := rowOf(l.snapshot(), "awg", "P")
	if r.DownBps != 0 || r.UpBps != 0 {
		t.Fatalf("reset tick rate = %+v, want 0", r)
	}
	*now = now.Add(2 * time.Second)
	src.counters = map[string]Counter{"P": {Down: 2500}}
	l.tick(context.Background())
	if r, _ := rowOf(l.snapshot(), "awg", "P"); r.DownBps != 1000 {
		t.Fatalf("after rebaseline rate = %d, want 1000", r.DownBps)
	}
}

func TestLiveFailingSourceDoesNotBlankOthers(t *testing.T) {
	users := &fakeSource{kind: "user", err: errors.New("v2ray_api: connection refused")}
	peers := &fakeSource{kind: "awg", counters: map[string]Counter{"P": {}}}
	l, now := newTestLive(users, peers)
	l.tick(context.Background())
	*now = now.Add(2 * time.Second)
	peers.counters = map[string]Counter{"P": {Down: 4000}}
	l.tick(context.Background())
	s := l.snapshot()
	if r, ok := rowOf(s, "awg", "P"); !ok || r.DownBps != 2000 {
		t.Fatalf("awg row = %+v ok=%v", r, ok)
	}
	if s.Unavailable["user"] == "" {
		t.Fatalf("unavailable = %v, want a reason for user", s.Unavailable)
	}
}

func TestLiveSourceRecoversAndPrimesAgain(t *testing.T) {
	src := &fakeSource{kind: "awg", counters: map[string]Counter{"P": {Down: 1000}}}
	l, now := newTestLive(src)
	l.tick(context.Background())
	*now = now.Add(2 * time.Second)
	src.err = errors.New("interface down")
	l.tick(context.Background())
	s := l.snapshot()
	if _, ok := rowOf(s, "awg", "P"); ok {
		t.Fatalf("failed source must drop its rows, got %+v", s.Rows)
	}
	if s.Unavailable["awg"] != "interface down" {
		t.Fatalf("unavailable = %v", s.Unavailable)
	}
	*now = now.Add(2 * time.Second)
	src.err = nil
	src.counters = map[string]Counter{"P": {Down: 1_000_000}}
	l.tick(context.Background())
	s = l.snapshot()
	if _, ok := rowOf(s, "awg", "P"); ok {
		t.Fatal("first tick after recovery only primes: the gap must not become a rate")
	}
	if len(s.Unavailable) != 0 {
		t.Fatalf("unavailable = %v, want empty after recovery", s.Unavailable)
	}
	*now = now.Add(2 * time.Second)
	src.counters = map[string]Counter{"P": {Down: 1_000_000 + 2000}}
	l.tick(context.Background())
	if r, ok := rowOf(l.snapshot(), "awg", "P"); !ok || r.DownBps != 1000 {
		t.Fatalf("after re-prime rate = %+v ok=%v", r, ok)
	}
}

func TestLiveIPv6IDKeepsColons(t *testing.T) {
	src := &fakeSource{kind: "lan", counters: map[string]Counter{"fd00::1": {}}}
	l, now := newTestLive(src)
	l.tick(context.Background())
	*now = now.Add(2 * time.Second)
	src.counters = map[string]Counter{"fd00::1": {Up: 200}}
	l.tick(context.Background())
	if r, ok := rowOf(l.snapshot(), "lan", "fd00::1"); !ok || r.UpBps != 100 {
		t.Fatalf("got %+v ok=%v", r, ok)
	}
}

func TestLiveRingIsBounded(t *testing.T) {
	src := &fakeSource{kind: "lan", counters: map[string]Counter{"ip": {}}}
	l, now := newTestLive(src)
	var total int64
	for i := 0; i < 40; i++ {
		total += 100
		src.counters = map[string]Counter{"ip": {Down: total}}
		l.tick(context.Background())
		*now = now.Add(2 * time.Second)
	}
	r, _ := rowOf(l.snapshot(), "lan", "ip")
	if len(r.Trend) != ringSize {
		t.Fatalf("trend len = %d, want %d", len(r.Trend), ringSize)
	}
}

func TestLiveSnapshotIsSortedAndCopied(t *testing.T) {
	src := &fakeSource{kind: "lan", counters: map[string]Counter{"b": {}, "a": {}}}
	l, now := newTestLive(src)
	l.tick(context.Background())
	*now = now.Add(2 * time.Second)
	src.counters = map[string]Counter{"b": {Down: 200}, "a": {Down: 400}}
	l.tick(context.Background())
	s := l.snapshot()
	if len(s.Rows) != 2 || s.Rows[0].ID != "a" || s.Rows[1].ID != "b" {
		t.Fatalf("rows = %+v, want sorted a,b", s.Rows)
	}
	if s.Ts != now.Unix() {
		t.Fatalf("ts = %d, want %d", s.Ts, now.Unix())
	}
	s.Rows[0].Trend[0].DownBps = -1
	s.Unavailable["x"] = "y"
	if l.snapshot().Rows[0].Trend[0].DownBps != 200 || len(l.snapshot().Unavailable) != 0 {
		t.Fatal("snapshot must be a copy, not a view of internal state")
	}
}

func TestLiveStopsWhenIdleAndForgets(t *testing.T) {
	src := &fakeSource{kind: "awg", counters: map[string]Counter{"P": {}}}
	l, now := newTestLive(src)
	l.lastReq = *now
	l.running = true
	l.tick(context.Background())
	*now = now.Add(5 * time.Second)
	if l.stopIfIdle() {
		t.Fatal("5 s after a request is not idle")
	}
	*now = now.Add(20 * time.Second)
	if !l.stopIfIdle() || l.running || l.prev != nil {
		t.Fatal("must stop and forget the baseline after 15 s without requests")
	}
	if len(l.snapshot().Rows) != 0 {
		t.Fatal("rows must be forgotten with the baseline")
	}
}

// TestLiveSnapshotStartsAndStopsLoop exercises the real goroutine once with the
// wall clock: Snapshot wakes the sampler, and it exits after liveIdle.
func TestLiveSnapshotStartsAndStopsLoop(t *testing.T) {
	src := &fakeSource{kind: "awg", counters: map[string]Counter{"P": {}}}
	l := NewLive([]Source{src})
	base := time.Unix(1000, 0)
	var mu sync.Mutex
	now := base
	l.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }

	s := l.Snapshot()
	if len(s.Rows) != 0 {
		t.Fatalf("first snapshot after wake must be empty, got %+v", s.Rows)
	}
	l.mu.Lock()
	running := l.running
	l.mu.Unlock()
	if !running {
		t.Fatal("Snapshot must start the loop")
	}
	// Push the clock past liveIdle; the next ticker beat must stop the loop.
	mu.Lock()
	now = base.Add(liveIdle + time.Second)
	mu.Unlock()
	deadline := time.Now().Add(2*liveInterval + time.Second)
	for time.Now().Before(deadline) {
		l.mu.Lock()
		running = l.running
		l.mu.Unlock()
		if !running {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("loop did not stop after liveIdle")
}
