package mtproto

import (
	"context"
	"sync"
	"time"

	"github.com/9seconds/mtg/v2/mtglib"
)

// Totals is one client's byte counters accumulated since the last drain.
type Totals struct {
	Upload   int64
	Download int64
}

// Connection is one live stream, as the panel shows it.
type Connection struct {
	StreamID  string    `json:"stream_id"`
	Client    string    `json:"client"`
	ClientIP  string    `json:"client_ip"`
	StartedAt time.Time `json:"started_at"`
}

// EventStream implements mtglib.EventStream, turning per-stream events into
// per-client totals and a live connection list.
//
// Attribution hangs entirely on EventClientMatched, which the fork emits the
// moment a secret authenticates a handshake. Streams that never match are
// domain-fronted to the masking site: they belong to no client, and their
// bytes are dropped rather than guessed at.
type EventStream struct {
	mu sync.Mutex

	// names maps a secret index to a client name, in the same order the
	// manager handed the secrets to mtglib. That ordering is the whole
	// contract; see Manager.secretsLocked.
	names []string

	streams map[string]*Connection
	totals  map[string]*Totals
}

// NewEventStream constructs a stream over a secret list, in mtglib order.
func NewEventStream(names []string) *EventStream {
	return &EventStream{
		names:   names,
		streams: map[string]*Connection{},
		totals:  map[string]*Totals{},
	}
}

// SetNames swaps the index-to-name mapping after a roster edit.
//
// Streams that already matched keep the client they authenticated as: they are
// still that client's connection, and re-pointing them at whoever now holds
// the index would bill the wrong person.
func (e *EventStream) SetNames(names []string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.names = names
}

// Send consumes one mtglib event.
//
// mtglib calls this on the connection's own goroutine, so it must not block.
func (e *EventStream) Send(_ context.Context, evt mtglib.Event) {
	e.mu.Lock()
	defer e.mu.Unlock()

	switch ev := evt.(type) {
	case mtglib.EventStart:
		// Recorded before the handshake, so the client IP is available by the
		// time a match arrives. Client stays empty until then, which is what
		// keeps fronted streams out of Connections.
		conn := e.streamLocked(ev.StreamID())
		if ev.RemoteIP != nil {
			conn.ClientIP = ev.RemoteIP.String()
		}

	case mtglib.EventClientMatched:
		if ev.SecretIdx < 0 || ev.SecretIdx >= len(e.names) {
			// The roster shrank between the handshake and this event. Leaving
			// the stream unattributed is the safe outcome.
			return
		}

		e.streamLocked(ev.StreamID()).Client = e.names[ev.SecretIdx]

	case mtglib.EventTraffic:
		conn, ok := e.streams[ev.StreamID()]
		if !ok || conn.Client == "" {
			return
		}

		total, ok := e.totals[conn.Client]
		if !ok {
			total = &Totals{}
			e.totals[conn.Client] = total
		}

		if ev.IsRead {
			total.Download += int64(ev.Traffic)
		} else {
			total.Upload += int64(ev.Traffic)
		}

	case mtglib.EventFinish:
		// Only the mapping goes; totals already accumulated are kept until the
		// next drain writes them out.
		delete(e.streams, ev.StreamID())
	}
}

// streamLocked returns the tracked stream, creating it if new. Caller holds mu.
func (e *EventStream) streamLocked(id string) *Connection {
	conn, ok := e.streams[id]
	if !ok {
		conn = &Connection{StreamID: id, StartedAt: time.Now()}
		e.streams[id] = conn
	}

	return conn
}

// DrainTotals returns the accumulated per-client counters and resets them, so
// the same bytes are never billed twice.
func (e *EventStream) DrainTotals() map[string]Totals {
	e.mu.Lock()
	defer e.mu.Unlock()

	out := make(map[string]Totals, len(e.totals))
	for name, total := range e.totals {
		out[name] = *total
	}

	e.totals = map[string]*Totals{}

	return out
}

// Connections returns the live matched streams. Fronted streams are omitted:
// they are not clients, and listing them would misrepresent the roster.
func (e *EventStream) Connections() []Connection {
	e.mu.Lock()
	defer e.mu.Unlock()

	out := make([]Connection, 0, len(e.streams))

	for _, conn := range e.streams {
		if conn.Client == "" {
			continue
		}

		out = append(out, *conn)
	}

	return out
}
