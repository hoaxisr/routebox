package subscriptions

import (
	"sync"
	"testing"
	"time"
)

func TestRunRefreshLoopTriggersDueSub(t *testing.T) {
	m := NewManager("")
	if _, err := m.Add("Due", "https://due", 6); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Add("Disabled", "https://off", 0); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	refreshed := map[string]int{}
	refresher := func(s Subscription, _ ConfigMerger) (int, int, error) {
		mu.Lock()
		refreshed[s.ID]++
		mu.Unlock()
		return 3, 0, nil
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		RunRefreshLoop(m, nil, refresher, 10*time.Millisecond, stop)
		close(done)
	}()
	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		got := refreshed["due"]
		mu.Unlock()
		if got >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("due subscription was never refreshed")
		case <-time.After(5 * time.Millisecond):
		}
	}
	close(stop)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("loop did not exit on stop")
	}
	mu.Lock()
	defer mu.Unlock()
	if refreshed["disabled"] != 0 {
		t.Fatalf("disabled must not refresh: %d", refreshed["disabled"])
	}
	if s, _ := m.Get("due"); s.NodeCount != 3 || s.LastUpdated == 0 {
		t.Fatalf("SetResult not applied: %+v", s)
	}
}
