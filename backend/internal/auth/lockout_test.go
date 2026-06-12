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
	// Backoff must not exceed max (5m). Advancing 5m+ unlocks.
	clock = clock.Add(5*time.Minute + time.Second)
	if !l.Allowed(key) {
		t.Fatal("backoff must be capped at max")
	}
}
