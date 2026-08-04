package mtproto

import (
	"sync"
	"time"
)

// DefaultLogBufferSize is how many recent lines the panel can show. The proxy
// logs a few lines per connection, so this is a couple of minutes of activity
// on a busy server — enough to see what just happened, not a log store.
const DefaultLogBufferSize = 500

// subscriberQueue is how far one viewer may fall behind before its next line is
// dropped. Small on purpose: a stalled browser must cost memory, not lines.
const subscriberQueue = 64

// LogEntry is one line as the panel renders it.
type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// LogBuffer is a ring of the proxy's recent log lines plus a fan-out to live
// viewers.
//
// The proxy has no log file of its own — mtglib logs through an interface, and
// RouteBox's own lines go to the container's stdout, which the panel cannot
// read. This is what makes them visible in the UI.
type LogBuffer struct {
	mu      sync.Mutex
	entries []LogEntry
	size    int
	subs    map[chan LogEntry]struct{}
}

// NewLogBuffer constructs a buffer holding at most size entries.
func NewLogBuffer(size int) *LogBuffer {
	if size <= 0 {
		size = DefaultLogBufferSize
	}

	return &LogBuffer{
		entries: make([]LogEntry, 0, size),
		size:    size,
		subs:    map[chan LogEntry]struct{}{},
	}
}

// Add records one line and hands it to every live viewer.
//
// It never blocks. mtglib calls the logger on a connection's own goroutine, so
// a viewer that has stopped reading must not be able to stall the proxy: its
// line is dropped instead.
func (b *LogBuffer) Add(level, message string) {
	entry := LogEntry{Time: time.Now(), Level: level, Message: message}

	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.entries) == b.size {
		// Shift rather than grow: this runs for the life of the process.
		copy(b.entries, b.entries[1:])
		b.entries[len(b.entries)-1] = entry
	} else {
		b.entries = append(b.entries, entry)
	}

	for ch := range b.subs {
		select {
		case ch <- entry:
		default: // viewer is behind; drop this line for it
		}
	}
}

// Recent returns a copy of the buffered lines, oldest first. This is the
// backlog a viewer gets on connect; the channel carries only what comes after.
func (b *LogBuffer) Recent() []LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := make([]LogEntry, len(b.entries))
	copy(out, b.entries)

	return out
}

// Subscribe returns a channel of lines logged from now on, and a cancel that
// unsubscribes and closes it. Cancel is safe to call more than once.
func (b *LogBuffer) Subscribe() (<-chan LogEntry, func()) {
	ch := make(chan LogEntry, subscriberQueue)

	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	var once sync.Once

	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs, ch)
			b.mu.Unlock()

			// Closed only after it can no longer be sent to, which is why the
			// delete happens under the same lock Add holds.
			close(ch)
		})
	}

	return ch, cancel
}

func errText(err error) string {
	if err == nil {
		return "<nil>"
	}

	return err.Error()
}
