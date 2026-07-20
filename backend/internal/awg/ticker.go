package awg

import "time"

// RunSweepLoop periodically invokes sweep so expired peers are suspended without an
// admin action. sweep is injected (the wiring layer passes Manager.SweepExpired).
// Blocks until stop is closed; once stop is closed, no further sweeps run.
// Mirrors users.RunExpiryLoop.
func RunSweepLoop(sweep func(), interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			// A tick and a close can race; re-check stop so a closed stop
			// always wins and no sweep runs after Stop was requested.
			select {
			case <-stop:
				return
			default:
			}
			sweep()
		}
	}
}
