package users

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestRunExpiryLoop_FiresOnTick(t *testing.T) {
	var calls int64
	stop := make(chan struct{})
	defer close(stop)
	go RunExpiryLoop(func() { atomic.AddInt64(&calls, 1) }, 5*time.Millisecond, stop)

	deadline := time.After(2 * time.Second)
	for {
		if atomic.LoadInt64(&calls) >= 1 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("sync was never invoked")
		case <-time.After(2 * time.Millisecond):
		}
	}
}

func TestRunExpiryLoop_StopsOnClose(t *testing.T) {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		RunExpiryLoop(func() {}, 5*time.Millisecond, stop)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	close(stop)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunExpiryLoop did not return after stop closed")
	}
}
