package updates

import (
	"log"
	"time"
)

// RunDailyChecks periodically refreshes the release cache for all targets
// supported on this arch. enabled is read on every tick so toggling
// updates.auto_check takes effect without restart. Blocks until stop closes.
func RunDailyChecks(c *Checker, targets []Target, enabled func() bool, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	tick := func() {
		if !enabled() {
			return
		}
		for _, t := range targets {
			if _, ok := t.AssetSuffix(c.arch); !ok {
				continue
			}
			if _, err := c.Check(t); err != nil {
				log.Printf("updates: auto-check %s: %v", t.Name, err)
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
