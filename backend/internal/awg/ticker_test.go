package awg

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestRunSweepLoopCallsThenStops(t *testing.T) {
	var n int32
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		RunSweepLoop(func() { atomic.AddInt32(&n, 1) }, 5*time.Millisecond, stop)
		close(done)
	}()

	// Wait until at least one sweep has run (poll, don't fixed-sleep).
	deadline := time.After(2 * time.Second)
	for atomic.LoadInt32(&n) < 1 {
		select {
		case <-deadline:
			t.Fatal("sweep never called")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	close(stop)
	select {
	case <-done: // loop returned — no further sweeps are possible
	case <-time.After(2 * time.Second):
		t.Fatal("RunSweepLoop did not stop")
	}

	got := atomic.LoadInt32(&n)
	time.Sleep(20 * time.Millisecond)
	if atomic.LoadInt32(&n) != got {
		t.Fatalf("sweep kept running after loop returned: %d != %d", atomic.LoadInt32(&n), got)
	}
}
