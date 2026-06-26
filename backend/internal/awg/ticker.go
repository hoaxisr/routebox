package awg

import "time"

// RunSweepLoop periodically invokes sweep so expired peers are suspended without an
// admin action. sweep is injected (the wiring layer passes Manager.SweepExpired).
// Blocks until stop is closed. Mirrors users.RunExpiryLoop.
func RunSweepLoop(sweep func(), interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			sweep()
		case <-stop:
			return
		}
	}
}
