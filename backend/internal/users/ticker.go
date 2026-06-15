package users

import "time"

// RunExpiryLoop periodically invokes sync so users expire without an admin
// action. sync is injected (the api glue, which already defers on a pending draft
// and reloads only when the managed rule actually changed) — the loop does NOT
// gate on "changed", it simply calls. Errors are sync's own concern (it logs).
// Blocks until stop is closed. Mirrors subscriptions.RunRefreshLoop.
func RunExpiryLoop(sync func(), interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			sync()
		case <-stop:
			return
		}
	}
}
