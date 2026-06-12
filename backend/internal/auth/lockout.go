package auth

import (
	"sync"
	"time"
)

type attempt struct {
	fails int
	until time.Time // locked until this instant
}

// Limiter throttles repeated failures per key (e.g. "IP|username") with
// exponential backoff after a threshold. In-memory.
type Limiter struct {
	mu        sync.Mutex
	attempts  map[string]*attempt
	threshold int
	base      time.Duration
	max       time.Duration
	now       func() time.Time
}

// NewLimiter returns a limiter: 5 failures then 1s,2s,4s… backoff capped at 5m.
func NewLimiter() *Limiter {
	return &Limiter{
		attempts:  map[string]*attempt{},
		threshold: 5,
		base:      1 * time.Second,
		max:       5 * time.Minute,
		now:       time.Now,
	}
}

// Allowed reports whether key may attempt now.
func (l *Limiter) Allowed(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.attempts[key]
	if !ok {
		return true
	}
	return !l.now().Before(a.until)
}

// Fail records a failed attempt and applies backoff once over threshold.
func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.attempts[key]
	if a == nil {
		a = &attempt{}
		l.attempts[key] = a
	}
	a.fails++
	if a.fails >= l.threshold {
		shift := uint(a.fails - l.threshold)
		if shift > 20 { // guard against overflow on extreme counts
			shift = 20
		}
		backoff := l.base << shift
		if backoff > l.max || backoff <= 0 {
			backoff = l.max
		}
		a.until = l.now().Add(backoff)
	}
}

// Reset clears a key's failure history (call on successful auth).
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}

// Cleanup drops entries that are no longer doing useful work: those below the
// lock threshold (transient failures) and those whose lock window has elapsed.
// Call periodically to bound memory.
func (l *Limiter) Cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for k, a := range l.attempts {
		if a.fails < l.threshold || now.After(a.until) {
			delete(l.attempts, k)
		}
	}
}
