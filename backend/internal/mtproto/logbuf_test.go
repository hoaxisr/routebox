package mtproto

import (
	"strings"
	"sync"
	"testing"
)

func TestBufferKeepsRecentEntries(t *testing.T) {
	b := NewLogBuffer(10)

	b.Add("info", "first")
	b.Add("warning", "second")

	got := b.Recent()
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}

	if got[0].Message != "first" || got[1].Message != "second" {
		t.Errorf("got %+v, want them in the order they arrived", got)
	}

	if got[0].Level != "info" || got[1].Level != "warning" {
		t.Errorf("levels = %q/%q", got[0].Level, got[1].Level)
	}
}

func TestBufferStampsEachEntry(t *testing.T) {
	b := NewLogBuffer(4)
	b.Add("info", "x")

	if b.Recent()[0].Time.IsZero() {
		t.Error("entry has no timestamp; the log view would have nothing to show")
	}
}

func TestBufferDropsTheOldestWhenFull(t *testing.T) {
	b := NewLogBuffer(3)

	for _, m := range []string{"a", "b", "c", "d", "e"} {
		b.Add("info", m)
	}

	got := b.Recent()
	if len(got) != 3 {
		t.Fatalf("len = %d, want the cap of 3", len(got))
	}

	// A ring, not a growing slice: this runs for the life of the process.
	if got[0].Message != "c" || got[2].Message != "e" {
		t.Errorf("got %v, want the last three", []string{got[0].Message, got[1].Message, got[2].Message})
	}
}

func TestRecentReturnsACopy(t *testing.T) {
	b := NewLogBuffer(4)
	b.Add("info", "original")

	got := b.Recent()
	got[0].Message = "tampered"

	if b.Recent()[0].Message != "original" {
		t.Error("mutating the returned slice changed the buffer")
	}
}

func TestSubscriberReceivesNewEntries(t *testing.T) {
	b := NewLogBuffer(10)

	ch, cancel := b.Subscribe()
	defer cancel()

	b.Add("error", "boom")

	select {
	case e := <-ch:
		if e.Message != "boom" || e.Level != "error" {
			t.Errorf("got %+v, want the error just added", e)
		}
	default:
		t.Fatal("subscriber received nothing")
	}
}

func TestSubscriberDoesNotSeeEntriesFromBeforeItSubscribed(t *testing.T) {
	b := NewLogBuffer(10)
	b.Add("info", "earlier")

	ch, cancel := b.Subscribe()
	defer cancel()

	select {
	case e := <-ch:
		t.Errorf("received %+v; the backlog comes from Recent, not the channel", e)
	default:
	}
}

func TestCancelStopsDelivery(t *testing.T) {
	b := NewLogBuffer(10)

	ch, cancel := b.Subscribe()
	cancel()

	b.Add("info", "after cancel")

	if _, open := <-ch; open {
		t.Error("channel should be closed after cancel")
	}
}

func TestCancelIsIdempotent(t *testing.T) {
	b := NewLogBuffer(10)

	_, cancel := b.Subscribe()
	cancel()
	cancel() // a double close would panic
}

func TestASlowSubscriberDoesNotBlockLogging(t *testing.T) {
	b := NewLogBuffer(10)

	// Subscribed and never read from. Every log call in the proxy runs on a
	// connection's own goroutine, so a stalled log viewer must not wedge it.
	_, cancel := b.Subscribe()
	defer cancel()

	done := make(chan struct{})

	go func() {
		defer close(done)

		for i := 0; i < 1000; i++ {
			b.Add("info", "flood")
		}
	}()

	<-done // hangs here if Add blocks on a full subscriber channel
}

func TestConcurrentAddAndReadAreSafe(t *testing.T) {
	b := NewLogBuffer(50)

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(2)

		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				b.Add("info", "x")
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = b.Recent()
			}
		}()
	}

	wg.Wait()
}

func TestLoggerWritesIntoTheBuffer(t *testing.T) {
	b := NewLogBuffer(10)
	l := newBufferedLogger(b)

	l.Info("started")
	l.Warning("careful")
	l.InfoError("cannot dial", errTest{})

	got := b.Recent()
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3", len(got))
	}

	if got[1].Level != "warning" {
		t.Errorf("level = %q, want warning", got[1].Level)
	}

	// The error has to reach the message, or the panel shows "cannot dial"
	// with no clue why.
	if !strings.Contains(got[2].Message, "test error") {
		t.Errorf("message = %q, want the error text included", got[2].Message)
	}
}

func TestLoggerDropsDebug(t *testing.T) {
	b := NewLogBuffer(10)
	l := newBufferedLogger(b)

	l.Debug("noisy")
	l.DebugError("also noisy", errTest{})

	// mtglib logs several debug lines per connection; keeping them would push
	// everything else out of a small ring within seconds.
	if got := b.Recent(); len(got) != 0 {
		t.Errorf("got %+v, want debug dropped", got)
	}
}

func TestBindReturnsTheSameLoggerSoTheBufferIsShared(t *testing.T) {
	b := NewLogBuffer(10)
	l := newBufferedLogger(b)

	l.Named("relay").BindStr("k", "v").BindInt("n", 1).Info("bound")

	if got := b.Recent(); len(got) != 1 {
		t.Errorf("got %d entries, want the bound logger to write to the same buffer", len(got))
	}
}

type errTest struct{}

func (errTest) Error() string { return "test error" }
