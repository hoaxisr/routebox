package mtproto

import (
	"context"
	"net"
	"testing"

	"github.com/9seconds/mtg/v2/mtglib"
)

func testStream() *EventStream {
	return NewEventStream([]string{"alice", "bob"})
}

// start/match/traffic/finish mirror the order mtglib emits events in, so the
// tests read the way the real stream behaves.
func start(es *EventStream, id string) {
	es.Send(context.Background(), mtglib.NewEventStart(id, net.IPv4(10, 0, 0, 1)))
}

func match(es *EventStream, id string, idx int) {
	es.Send(context.Background(), mtglib.NewEventClientMatched(id, idx))
}

func traffic(es *EventStream, id string, n uint, isRead bool) {
	es.Send(context.Background(), mtglib.NewEventTraffic(id, n, isRead))
}

func finish(es *EventStream, id string) {
	es.Send(context.Background(), mtglib.NewEventFinish(id))
}

func TestTrafficIsAttributedToTheMatchedClient(t *testing.T) {
	es := testStream()

	start(es, "s1")
	match(es, "s1", 0) // index 0 == alice
	traffic(es, "s1", 100, true)
	traffic(es, "s1", 40, false)

	got := es.DrainTotals()

	if got["alice"].Download != 100 || got["alice"].Upload != 40 {
		t.Errorf("alice = %+v, want 100 down / 40 up", got["alice"])
	}
}

func TestTwoClientsAreCountedSeparately(t *testing.T) {
	es := testStream()

	match(es, "s1", 0)
	match(es, "s2", 1)
	traffic(es, "s1", 10, true)
	traffic(es, "s2", 70, true)

	got := es.DrainTotals()

	if got["alice"].Download != 10 || got["bob"].Download != 70 {
		t.Errorf("got %+v, want alice 10 and bob 70", got)
	}
}

func TestTrafficForAnUnmatchedStreamIsDiscarded(t *testing.T) {
	es := testStream()

	// A domain-fronted stream: started, never matched, and still generating
	// traffic events from the fronting connection.
	start(es, "s2")
	traffic(es, "s2", 999, true)

	if got := es.DrainTotals(); len(got) != 0 {
		t.Errorf("got %+v, want nothing — fronted bytes belong to no client", got)
	}
}

func TestDrainTotalsResetsTheCounters(t *testing.T) {
	es := testStream()

	match(es, "s1", 0)
	traffic(es, "s1", 10, true)
	es.DrainTotals()

	// Draining twice must not bill the same bytes twice.
	if got := es.DrainTotals(); len(got) != 0 {
		t.Errorf("second drain returned %+v, want empty", got)
	}
}

func TestFinishStopsAttribution(t *testing.T) {
	es := testStream()

	match(es, "s1", 0)
	finish(es, "s1")
	traffic(es, "s1", 500, true)

	if got := es.DrainTotals(); len(got) != 0 {
		t.Errorf("got %+v, want nothing once the stream finished", got)
	}
}

func TestFinishKeepsAlreadyCountedBytes(t *testing.T) {
	es := testStream()

	match(es, "s1", 0)
	traffic(es, "s1", 33, true)
	finish(es, "s1")

	// The connection closing must not discard what it already transferred.
	if got := es.DrainTotals(); got["alice"].Download != 33 {
		t.Errorf("alice = %+v, want the 33 bytes it transferred before closing", got["alice"])
	}
}

func TestConnectionsListsMatchedStreamsOnly(t *testing.T) {
	es := testStream()

	start(es, "s1")
	match(es, "s1", 1) // bob
	start(es, "s2")    // fronted, never matched

	conns := es.Connections()

	if len(conns) != 1 || conns[0].Client != "bob" {
		t.Errorf("Connections = %+v, want one entry for bob", conns)
	}
}

func TestConnectionsCarriesTheClientIP(t *testing.T) {
	es := testStream()

	start(es, "s1")
	match(es, "s1", 0)

	conns := es.Connections()
	if len(conns) != 1 {
		t.Fatalf("Connections = %+v, want one", conns)
	}

	if conns[0].ClientIP != "10.0.0.1" {
		t.Errorf("ClientIP = %q, want 10.0.0.1", conns[0].ClientIP)
	}
}

func TestConnectionsDropsFinishedStreams(t *testing.T) {
	es := testStream()

	match(es, "s1", 0)
	finish(es, "s1")

	if conns := es.Connections(); len(conns) != 0 {
		t.Errorf("Connections = %+v, want empty after finish", conns)
	}
}

func TestAnOutOfRangeSecretIndexIsIgnored(t *testing.T) {
	es := testStream()

	// A rebuild can shrink the roster between the handshake and the event.
	// Dropping the stream is the safe outcome: the alternative is billing
	// whoever now occupies that index.
	match(es, "s1", 42)
	traffic(es, "s1", 10, true)

	if got := es.DrainTotals(); len(got) != 0 {
		t.Errorf("got %+v, want nothing for an unknown index", got)
	}

	if conns := es.Connections(); len(conns) != 0 {
		t.Errorf("Connections = %+v, want empty for an unknown index", conns)
	}
}

func TestANegativeSecretIndexIsIgnored(t *testing.T) {
	es := testStream()

	match(es, "s1", -1)
	traffic(es, "s1", 10, true)

	if got := es.DrainTotals(); len(got) != 0 {
		t.Errorf("got %+v, want nothing", got)
	}
}

func TestSetNamesLeavesLiveStreamsOnTheirOriginalClient(t *testing.T) {
	es := testStream()

	match(es, "s1", 0) // alice
	es.SetNames([]string{"carol", "dave"})
	traffic(es, "s1", 25, true)

	// The stream authenticated as alice; a later roster edit must not
	// retroactively bill carol for it.
	got := es.DrainTotals()
	if got["alice"].Download != 25 {
		t.Errorf("got %+v, want the bytes still on alice", got)
	}
}

func TestUnknownEventsAreIgnored(t *testing.T) {
	es := testStream()

	es.Send(context.Background(), mtglib.NewEventConcurrencyLimited())
	es.Send(context.Background(), mtglib.NewEventReplayAttack("s1"))

	if got := es.DrainTotals(); len(got) != 0 {
		t.Errorf("got %+v, want nothing", got)
	}
}
