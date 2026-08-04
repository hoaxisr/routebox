package mtproto

import (
	"log"
	"time"
)

// TrafficSink is the slice of traffic.Store this package needs.
//
// Narrowed to one method so the dependency runs one way and the flusher can be
// tested without a database.
type TrafficSink interface {
	UpsertUser(bucketTs int64, user string, upload, download int64) error
}

// eventSource is what the flush loop reads from. It is an interface rather than
// an *EventStream because the stream does not exist until the proxy starts, and
// the loop has to pick up one created later.
type eventSource interface {
	Events() *EventStream
}

// TrafficKeyPrefix namespaces MTProto rows inside the shared user_traffic
// table.
//
// Nothing enumerates distinct keys there — every read is by explicit panel-user
// name — so namespaced rows inherit history, sparklines, pruning and reset
// without ever showing up in the panel-users view.
const TrafficKeyPrefix = "mtproto:"

// TrafficKey is the user_traffic key one MTProto client's bytes are stored
// under.
func TrafficKey(client string) string { return TrafficKeyPrefix + client }

// FlushTotals drains the event stream into the traffic store, one row per
// client with traffic since the last flush.
//
// Idle clients write nothing: an empty bucket is indistinguishable from a real
// one in the history queries, so writing them would pad every sparkline with
// rows that mean "no data".
func FlushTotals(es *EventStream, sink TrafficSink, now time.Time) {
	if es == nil || sink == nil {
		return
	}

	bucket := now.Truncate(time.Minute).Unix()

	for name, total := range es.DrainTotals() {
		if total.Upload == 0 && total.Download == 0 {
			continue
		}

		if err := sink.UpsertUser(bucket, TrafficKey(name), total.Upload, total.Download); err != nil {
			// Logged and skipped rather than returned: the other clients' rows
			// are independent, and losing them too would only compound this.
			log.Printf("mtproto: cannot record traffic for %s: %v", name, err)
		}
	}
}

// RunFlushLoop flushes on an interval until stop is closed. Mirrors
// awg.RunSweepLoop.
func RunFlushLoop(src eventSource, sink TrafficSink, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			// One last flush, so bytes from the final partial interval are not
			// lost on a clean shutdown.
			FlushTotals(src.Events(), sink, time.Now())

			return
		case now := <-ticker.C:
			FlushTotals(src.Events(), sink, now)
		}
	}
}

// SweepExpired disables every client past its expiry and reports whether
// anything changed, so the caller knows to rebuild the proxy.
//
// Expiry disables rather than deletes: the client stays on the page with its
// traffic history, and an admin can extend it instead of issuing a new secret
// and redistributing a link.
func SweepExpired(s *Store, now time.Time) bool {
	changed := false

	for _, c := range s.List() {
		// Already-disabled clients are skipped, or every tick would rewrite the
		// file and trigger a pointless rebuild for as long as they exist.
		if !c.Enabled || c.ExpiresAt == 0 || now.Unix() < c.ExpiresAt {
			continue
		}

		c.Enabled = false

		if err := s.Put(c); err != nil {
			log.Printf("mtproto: cannot disable expired client %s: %v", c.Name, err)

			continue
		}

		changed = true
	}

	return changed
}

// RunExpiryLoop sweeps on an interval, rebuilding whenever something lapsed.
func RunExpiryLoop(s *Store, rebuild func() error, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case now := <-ticker.C:
			if !SweepExpired(s, now) {
				continue
			}

			if err := rebuild(); err != nil {
				log.Printf("mtproto: cannot rebuild after expiry: %v", err)
			}
		}
	}
}
