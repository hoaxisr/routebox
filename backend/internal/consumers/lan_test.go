package consumers

import (
	"context"
	"net/netip"
	"testing"

	"routebox/backend/internal/clients"
	"routebox/backend/internal/traffic"
)

func lanSource(conns *[]traffic.ConnectionSample) *LanSource {
	return &LanSource{
		Enabled: func() bool { return true },
		Clients: func() []clients.Entry {
			return []clients.Entry{{IP: "192.168.1.5", Name: "TV"}, {IP: "10.10.0.2"}, {IP: "8.8.8.8"}}
		},
		AwgPrefixes: func() []netip.Prefix {
			return []netip.Prefix{netip.MustParsePrefix("10.10.0.0/24"), netip.MustParsePrefix("fd12:3456:789a::/64")}
		},
		Fetch: func() ([]traffic.ConnectionSample, error) { return *conns, nil },
	}
}

func TestLanSourceExcludesAwgSubnet(t *testing.T) {
	conns := []traffic.ConnectionSample{
		{ID: "1", Source: "192.168.1.5", Upload: 10, Download: 20},
		{ID: "2", Source: "10.10.0.2", Upload: 99, Download: 99},
	}
	src := lanSource(&conns)
	rows, err := src.List(0, 120)
	if err != nil || len(rows) != 1 || rows[0].ID != "192.168.1.5" || rows[0].Name != "TV" {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	c, _ := src.Counters(context.Background())
	if _, ok := c["10.10.0.2"]; ok || c["192.168.1.5"] != (Counter{Up: 10, Down: 20}) {
		t.Fatalf("counters = %v", c)
	}
}

// A connection that closes between two reads must not make the source's
// counter go down — that would read as a reset and zero the rate.
func TestLanSourceCountersAreMonotonicAcrossClosedConnections(t *testing.T) {
	conns := []traffic.ConnectionSample{{ID: "1", Source: "192.168.1.5", Upload: 100}}
	src := lanSource(&conns)
	_, _ = src.Counters(context.Background())
	conns = []traffic.ConnectionSample{{ID: "2", Source: "192.168.1.5", Upload: 30}}
	c, _ := src.Counters(context.Background())
	if c["192.168.1.5"].Up != 130 {
		t.Fatalf("up = %d, want 130", c["192.168.1.5"].Up)
	}
}

func TestLanSourceOffInVpsMode(t *testing.T) {
	conns := []traffic.ConnectionSample{{ID: "1", Source: "192.168.1.5", Upload: 1}}
	src := lanSource(&conns)
	src.Enabled = func() bool { return false }
	rows, _ := src.List(0, 1)
	c, err := src.Counters(context.Background())
	if rows != nil || c != nil || err != nil {
		t.Fatalf("rows=%v c=%v err=%v", rows, c, err)
	}
}

// A known connection whose own counter went down (Clash reused the id) must
// not be re-counted from zero: it contributes nothing that read.
func TestLanSourceReusedConnectionIDDoesNotDoubleCount(t *testing.T) {
	conns := []traffic.ConnectionSample{{ID: "1", Source: "192.168.1.5", Upload: 100, Download: 50}}
	src := lanSource(&conns)
	_, _ = src.Counters(context.Background())
	conns = []traffic.ConnectionSample{{ID: "1", Source: "192.168.1.5", Upload: 40, Download: 60}}
	c, _ := src.Counters(context.Background())
	if c["192.168.1.5"] != (Counter{Up: 100, Down: 60}) {
		t.Fatalf("counter = %+v, want {100 60}", c["192.168.1.5"])
	}
}

// A peer reported in the IPv4-mapped form is still a peer, not a LAN device.
func TestLanSourceExcludesMappedAwgAddress(t *testing.T) {
	conns := []traffic.ConnectionSample{{ID: "1", Source: "::ffff:10.10.0.2", Upload: 5}}
	src := lanSource(&conns)
	c, _ := src.Counters(context.Background())
	if len(c) != 0 {
		t.Fatalf("counters = %v, want none", c)
	}
}

// With the IPv6 broker on, a peer also carries an address from the server's
// ULA /64. IsLocalClientIP accepts any unicast v6, so without the prefix the
// peer's v6 traffic would show twice: in its awg row and as a lan:<fd..> row.
func TestLanSourceExcludesUlaPeerAddress(t *testing.T) {
	store := openStore(t)
	_ = store.Upsert(60, "fd12:3456:789a::a0a:2", "example.com", "direct", 7, 70)
	conns := []traffic.ConnectionSample{
		{ID: "1", Source: "fd12:3456:789a::a0a:2", Upload: 5, Download: 6},
		{ID: "2", Source: "fd00:dead:beef::5", Upload: 1, Download: 2}, // some other ULA: a LAN device
	}
	src := lanSource(&conns)
	src.Store = store
	src.Clients = func() []clients.Entry {
		return []clients.Entry{{IP: "fd12:3456:789a::a0a:2", Name: "peer-v6"}, {IP: "fd00:dead:beef::5", Name: "nas"}}
	}
	rows, err := src.List(0, 120)
	if err != nil || len(rows) != 1 || rows[0].ID != "fd00:dead:beef::5" {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	c, err := src.Counters(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c["fd12:3456:789a::a0a:2"]; ok || c["fd00:dead:beef::5"] != (Counter{Up: 1, Down: 2}) {
		t.Fatalf("counters = %v", c)
	}
}

func TestLanSourceListReadsSourceHistory(t *testing.T) {
	store := openStore(t)
	_ = store.Upsert(60, "192.168.1.5", "example.com", "direct", 10, 20)
	_ = store.Upsert(60, "10.10.0.2", "example.com", "direct", 99, 99)
	var conns []traffic.ConnectionSample
	src := lanSource(&conns)
	src.Store = store
	rows, err := src.List(0, 120)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	r := rows[0]
	if r.Upload != 10 || r.Download != 20 || len(r.History) != 1 || r.History[0].Upload != 10 || r.Address != "192.168.1.5" {
		t.Fatalf("row = %+v", r)
	}
}
