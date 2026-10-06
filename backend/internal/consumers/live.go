package consumers

import (
	"context"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	liveInterval = 2 * time.Second
	liveIdle     = 15 * time.Second
	ringSize     = 30 // 60 s at liveInterval
	// sourceBudget bounds one Counters call; a slow source must not stall the tick.
	// Shared with the users source, which talks to v2ray_api under the same budget.
	sourceBudget = 1500 * time.Millisecond
)

// TrendPoint is one sampled rate.
type TrendPoint struct {
	Ts      int64 `json:"ts"`
	DownBps int64 `json:"down_bps"`
	UpBps   int64 `json:"up_bps"`
}

// LiveRow is one consumer's current rate plus its recent trend.
type LiveRow struct {
	Kind    string       `json:"kind"`
	ID      string       `json:"id"`
	DownBps int64        `json:"down_bps"`
	UpBps   int64        `json:"up_bps"`
	Trend   []TrendPoint `json:"trend"`
}

// LiveSnapshot is the sampler's view at Ts. Unavailable maps a source kind to
// why its last read failed; kinds that read fine are absent.
type LiveSnapshot struct {
	Ts          int64             `json:"ts"`
	Rows        []LiveRow         `json:"rows"`
	Unavailable map[string]string `json:"unavailable"`
}

// Live samples every source's cumulative counters every 2 s while someone is
// asking (the monitor page polls /live) and turns them into rates. It sleeps
// 15 s after the last request, so a closed page costs nothing.
type Live struct {
	sources []Source
	now     func() time.Time

	mu          sync.Mutex
	running     bool
	lastReq     time.Time
	prev        map[string]Counter // "kind:id" → last counters; nil = not primed
	prevAt      time.Time
	ring        map[string][]TrendPoint
	unavailable map[string]string
	ts          int64
}

// NewLive builds a sleeping sampler over sources; the first Snapshot wakes it.
func NewLive(sources []Source) *Live {
	return &Live{sources: sources, now: time.Now, ring: map[string][]TrendPoint{}, unavailable: map[string]string{}}
}

// Snapshot marks a request, wakes the sampler if it sleeps, and returns the
// current view. The first call after a sleep returns no rows yet.
func (l *Live) Snapshot() LiveSnapshot {
	l.mu.Lock()
	l.lastReq = l.now()
	start := !l.running
	l.running = true
	l.mu.Unlock()
	if start {
		go l.loop()
	}
	return l.snapshot()
}

func (l *Live) loop() {
	t := time.NewTicker(liveInterval)
	defer t.Stop()
	l.tick(context.Background())
	for range t.C {
		if l.stopIfIdle() {
			return
		}
		l.tick(context.Background())
	}
}

// stopIfIdle ends the loop after liveIdle without a request and forgets the
// baseline, so the next wake-up primes from scratch instead of turning the
// whole sleep into one giant "rate".
func (l *Live) stopIfIdle() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.now().Sub(l.lastReq) <= liveIdle {
		return false
	}
	l.running = false
	l.prev = nil
	l.ring = map[string][]TrendPoint{}
	l.unavailable = map[string]string{}
	return true
}

// tick reads every source once and appends one rate point per consumer seen in
// two consecutive snapshots. A counter that went down (process restart) gives
// 0 for that tick and becomes the new baseline. A source that failed this tick
// loses its baseline and rows: its next success primes again. Failures are
// logged on transition only, never every tick.
func (l *Live) tick(ctx context.Context) {
	cur := map[string]Counter{}
	unavailable := map[string]string{}
	for _, s := range l.sources {
		cctx, cancel := context.WithTimeout(ctx, sourceBudget)
		c, err := s.Counters(cctx)
		cancel()
		if err != nil {
			unavailable[s.Kind()] = err.Error()
			continue
		}
		for id, v := range c {
			cur[s.Kind()+":"+id] = v
		}
	}
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()
	for kind, why := range unavailable {
		if l.unavailable[kind] != why {
			log.Printf("consumers: live %s unavailable: %s", kind, why)
		}
	}
	for kind := range l.unavailable {
		if _, still := unavailable[kind]; !still {
			log.Printf("consumers: live %s available again", kind)
		}
	}
	l.unavailable = unavailable
	if secs := now.Sub(l.prevAt).Seconds(); l.prev != nil && secs > 0 {
		for key, c := range cur {
			p, ok := l.prev[key]
			if !ok {
				continue // first seen: no rate until the next snapshot
			}
			pt := TrendPoint{Ts: now.Unix()}
			if c.Down >= p.Down {
				pt.DownBps = int64(float64(c.Down-p.Down) / secs)
			}
			if c.Up >= p.Up {
				pt.UpBps = int64(float64(c.Up-p.Up) / secs)
			}
			r := append(l.ring[key], pt)
			if len(r) > ringSize {
				r = r[len(r)-ringSize:]
			}
			l.ring[key] = r
		}
	}
	for key := range l.ring {
		if _, ok := cur[key]; !ok {
			delete(l.ring, key)
		}
	}
	l.prev, l.prevAt, l.ts = cur, now, now.Unix()
}

// snapshot copies the current view: one row per consumer with at least one
// rate point, its newest point as the current rate. Sorted for stable output.
// strings.Cut splits on the FIRST colon, so ids with colons (IPv6) stay whole.
func (l *Live) snapshot() LiveSnapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := LiveSnapshot{Ts: l.ts, Rows: []LiveRow{}, Unavailable: map[string]string{}}
	for k, v := range l.unavailable {
		out.Unavailable[k] = v
	}
	keys := make([]string, 0, len(l.ring))
	for k := range l.ring {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		pts := l.ring[key]
		kind, id, _ := strings.Cut(key, ":")
		last := pts[len(pts)-1]
		out.Rows = append(out.Rows, LiveRow{Kind: kind, ID: id, DownBps: last.DownBps, UpBps: last.UpBps, Trend: append([]TrendPoint(nil), pts...)})
	}
	return out
}
