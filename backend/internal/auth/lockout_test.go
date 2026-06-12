package auth

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	l := NewLimiter()
	clock := time.Unix(0, 0)
	l.now = func() time.Time { return clock }
	key := "1.2.3.4|admin"

	// Under threshold (5): always allowed.
	for i := 0; i < 4; i++ {
		if !l.Allowed(key) {
			t.Fatalf("should be allowed before threshold (i=%d)", i)
		}
		l.Fail(key)
	}
	if !l.Allowed(key) {
		t.Fatal("4 fails still under threshold -> allowed")
	}

	// 5th fail trips the lock.
	l.Fail(key)
	if l.Allowed(key) {
		t.Fatal("threshold reached -> must be locked")
	}

	// Lock clears after backoff elapses.
	clock = clock.Add(2 * time.Second)
	if !l.Allowed(key) {
		t.Fatal("after backoff window -> allowed again")
	}

	// Reset clears history.
	l.Fail(key)
	l.Reset(key)
	if !l.Allowed(key) {
		t.Fatal("reset -> allowed")
	}
}

func TestLimiterBackoffCap(t *testing.T) {
	l := NewLimiter()
	clock := time.Unix(0, 0)
	l.now = func() time.Time { return clock }
	key := "k"
	for i := 0; i < 30; i++ {
		l.Fail(key)
	}
	// Backoff must not exceed max (5m). Check still locked just before cap window
	// elapses, then confirm it unlocks just after.
	clock = clock.Add(4*time.Minute + 59*time.Second)
	if l.Allowed(key) {
		t.Fatal("should still be locked just before the cap window elapses")
	}
	clock = clock.Add(2 * time.Second) // now past 5m total
	if !l.Allowed(key) {
		t.Fatal("backoff must be capped at max")
	}
}

func TestLimiterCleanup(t *testing.T) {
	l := NewLimiter()
	clock := time.Unix(0, 0)
	l.now = func() time.Time { return clock }

	// A: below threshold (3 fails) -> evicted.
	for i := 0; i < 3; i++ {
		l.Fail("below")
	}
	// B: locked (5 fails) and still within window -> kept.
	for i := 0; i < 5; i++ {
		l.Fail("locked")
	}
	// C: locked then window elapses -> evicted.
	for i := 0; i < 5; i++ {
		l.Fail("expired")
	}

	clock = clock.Add(10 * time.Second) // past 'expired' and 'locked' 1s backoff...
	// NOTE: both 'locked' and 'expired' had 5 fails -> backoff 1s, so after 10s both unlock.
	// To keep B locked, give it a fresh fail so its window is current:
	l.Fail("locked") // 6th fail -> backoff 2s from now (clock=10s), until=12s

	l.Cleanup()

	if !l.Allowed("below") {
		t.Fatal("below-threshold key should be allowed (and was evicted)")
	}
	if l.Allowed("locked") {
		t.Fatal("actively-locked key must remain locked after cleanup")
	}
	if !l.Allowed("expired") {
		t.Fatal("expired-lock key should be allowed (and was evicted)")
	}
	// Verify eviction actually shrank the map: only 'locked' should remain.
	l.mu.Lock()
	n := len(l.attempts)
	l.mu.Unlock()
	if n != 1 {
		t.Fatalf("expected 1 remaining entry (locked), got %d", n)
	}
}
