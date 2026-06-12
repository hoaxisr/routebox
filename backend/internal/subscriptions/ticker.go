package subscriptions

import (
	"log"
	"time"
)

// RefreshFunc refreshes one subscription. Injected so the loop is testable.
type RefreshFunc func(Subscription, ConfigMerger) (nodeCount, skipped int, err error)

// RunRefreshLoop refreshes due subscriptions each tick. Errors land in the
// store's LastError and never stop the loop. Blocks until stop closes.
func RunRefreshLoop(store *Manager, cfg ConfigMerger, refresher RefreshFunc, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	tick := func() {
		now := time.Now().Unix()
		for _, s := range store.List() {
			if s.IntervalHrs <= 0 {
				continue
			}
			if now-s.LastUpdated < int64(s.IntervalHrs)*3600 {
				continue
			}
			n, _, err := refresher(s, cfg)
			errMsg := ""
			if err != nil {
				errMsg = err.Error()
				log.Printf("subscriptions: refresh %s: %v", s.ID, err)
			}
			if serr := store.SetResult(s.ID, n, errMsg); serr != nil {
				log.Printf("subscriptions: record result %s: %v", s.ID, serr)
			}
		}
	}
	for {
		select {
		case <-ticker.C:
			tick()
		case <-stop:
			return
		}
	}
}
