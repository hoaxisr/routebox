package awg

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestRunSweepLoopCallsThenStops(t *testing.T) {
	var n int32
	stop := make(chan struct{})
	go RunSweepLoop(func() { atomic.AddInt32(&n, 1) }, 5*time.Millisecond, stop)
	time.Sleep(30 * time.Millisecond)
	close(stop)
	got := atomic.LoadInt32(&n)
	if got < 1 {
		t.Fatalf("sweep never called: %d", got)
	}
	time.Sleep(20 * time.Millisecond)
	if atomic.LoadInt32(&n) != got {
		t.Fatal("sweep kept running after stop")
	}
}
