package awg

import (
	"context"
	"sync"
	"testing"
)

// SetConfigSync rewires cfgSync/applyFn/supportsFn under m.mu at runtime: the
// wiring layer calls it at startup and the settings handler calls it again on a
// backend switch, while the 30s sweep ticker and every peer op are reading those
// same fields. decommissionSingbox snapshots them under the lock; singboxSync
// and disableSingbox read them bare, which is a data race — and not only a
// formal one: the nil check and the call are two separate reads, so a rewire in
// between turns "cfgSync is not nil" into a nil dereference on a router that is
// serving traffic.
//
// Only the race detector can see this, so it is a -race test by construction.
func TestSingbox_ConfigHooksAreReadUnderTheLock(t *testing.T) {
	ctx := context.Background()
	m, fs, _ := newSingboxMgr(t)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				m.SetConfigSync(fs, func() error { return nil }, func() bool { return true })
			}
		}
	}()

	for i := 0; i < 300; i++ {
		_ = m.singboxSync()
		_ = m.disableSingbox(ctx)
	}
	close(stop)
	wg.Wait()
}
