package traffic

import (
	"log"
	"sync"
	"time"

	"routebox/backend/internal/v2stats"
)

// UserDelta is the per-user byte increment attributed since the previous tick.
type UserDelta struct {
	Upload   int64
	Download int64
}

// userQuerier is the slice of *v2stats.Client the sampler needs (test seam).
type userQuerier interface {
	QueryUsersTimeout(d time.Duration) (map[string]v2stats.Counters, error)
}

// UserSampler turns periodic cumulative StatsService snapshots into per-minute
// per-user byte deltas. The per-user twin of Sampler; the reset-handling logic is
// intentionally identical (cur<prev => count cur, never negative).
type UserSampler struct {
	store    *Store
	mu       sync.Mutex
	lastSeen map[string]v2stats.Counters
}

// NewUserSampler constructs a sampler. A nil store makes Run a no-op (additivity:
// no traffic.db => no per-user accounting, exactly like Sampler).
func NewUserSampler(store *Store) *UserSampler {
	return &UserSampler{store: store, lastSeen: map[string]v2stats.Counters{}}
}

// computeUserDeltas diffs the new cumulative snapshot vs lastSeen, reset-safely,
// and evicts users absent from the snapshot. Zero-delta users are omitted.
func (s *UserSampler) computeUserDeltas(cur map[string]v2stats.Counters) map[string]UserDelta {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]UserDelta{}
	for name, c := range cur {
		prev, ok := s.lastSeen[name]
		var dUp, dDown int64
		if ok {
			if c.Uplink >= prev.Uplink {
				dUp = c.Uplink - prev.Uplink
			} else {
				dUp = c.Uplink // counter reset → full current value
			}
			if c.Downlink >= prev.Downlink {
				dDown = c.Downlink - prev.Downlink
			} else {
				dDown = c.Downlink
			}
		} else {
			dUp, dDown = c.Uplink, c.Downlink // new user → full value
		}
		s.lastSeen[name] = c
		if dUp == 0 && dDown == 0 {
			continue
		}
		out[name] = UserDelta{Upload: dUp, Download: dDown}
	}
	// Evict users no longer present so a reappearance starts fresh.
	for name := range s.lastSeen {
		if _, ok := cur[name]; !ok {
			delete(s.lastSeen, name)
		}
	}
	return out
}

// Run polls the StatsService every intervalSec, writing per-minute deltas, and
// prunes hourly. No-op if store or query is nil. Graceful: the FIRST query
// failure logs once, subsequent consecutive failures are silent (no spam) until
// a success resets the gate — so an old binary without with_v2ray_api, or an
// unreachable addr, leaves counters flat without crashing or flooding the log.
// The log gate is a LOCAL var (not a struct field) to avoid a data race.
func (s *UserSampler) Run(query userQuerier, intervalSec, retentionDays int, stop <-chan struct{}) {
	if s.store == nil || query == nil {
		return
	}
	if intervalSec <= 0 {
		intervalSec = 30
	}
	tick := time.NewTicker(time.Duration(intervalSec) * time.Second)
	defer tick.Stop()
	prune := time.NewTicker(time.Hour)
	defer prune.Stop()

	failed := false // first-failure-then-silent log gate (loop-local)

	doSample := func() {
		snap, err := query.QueryUsersTimeout(10 * time.Second)
		if err != nil {
			if !failed {
				log.Printf("v2stats: per-user traffic unavailable (counters stay flat): %v", err)
				failed = true
			}
			return // skip tick: NO state change, counters flat — never zero them
		}
		if failed {
			log.Printf("v2stats: per-user traffic available again")
			failed = false
		}
		deltas := s.computeUserDeltas(snap)
		bucket := (time.Now().Unix() / 60) * 60
		for name, d := range deltas {
			if err := s.store.UpsertUser(bucket, name, d.Upload, d.Download); err != nil {
				log.Printf("user_traffic upsert: %v", err)
			}
		}
	}
	doPrune := func() {
		if retentionDays <= 0 {
			return
		}
		cutoff := time.Now().Unix() - int64(retentionDays)*86400
		if err := s.store.PruneUserOlderThan(cutoff); err != nil {
			log.Printf("user_traffic prune: %v", err)
		}
	}

	for {
		select {
		case <-tick.C:
			doSample()
		case <-prune.C:
			doPrune()
		case <-stop:
			return
		}
	}
}
