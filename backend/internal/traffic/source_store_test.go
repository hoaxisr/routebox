package traffic

import (
	"path/filepath"
	"testing"
)

// Issue #40: AWG peers get the per-user treatment of /monitor/users. Their bytes
// are not in user_traffic (sing-box accounts inbound USERS; an AWG peer is not
// one) — they are in traffic_minute under the peer's tunnel IP, the same rows
// the Breakdown panel reads.
func TestQuerySourceTotalsAndHistory(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	const peer, other = "10.10.64.2", "10.10.64.3"
	// Two buckets for the peer, split across domains/chains so the query has to
	// collapse them; one row for a different source that must never leak in.
	if err := s.Upsert(60, peer, "a.example", "direct", 10, 20); err != nil {
		t.Fatal(err)
	}
	if err := s.Upsert(60, peer, "b.example", "proxy", 1, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.Upsert(120, peer, "a.example", "direct", 100, 200); err != nil {
		t.Fatal(err)
	}
	if err := s.Upsert(120, other, "a.example", "direct", 999, 999); err != nil {
		t.Fatal(err)
	}
	// Outside the window on both ends.
	if err := s.Upsert(30, peer, "a.example", "direct", 7, 7); err != nil {
		t.Fatal(err)
	}
	if err := s.Upsert(600, peer, "a.example", "direct", 8, 8); err != nil {
		t.Fatal(err)
	}

	up, down, err := s.QuerySourceTotals(60, 120, peer)
	if err != nil {
		t.Fatalf("QuerySourceTotals: %v", err)
	}
	if up != 111 || down != 222 {
		t.Fatalf("totals = %d/%d, want 111/222", up, down)
	}

	hist, err := s.QuerySourceHistory(60, 120, peer)
	if err != nil {
		t.Fatalf("QuerySourceHistory: %v", err)
	}
	want := []UserHistoryRow{
		{BucketTs: 60, Upload: 11, Download: 22},
		{BucketTs: 120, Upload: 100, Download: 200},
	}
	if len(hist) != len(want) {
		t.Fatalf("history = %+v, want %+v", hist, want)
	}
	for i := range want {
		if hist[i] != want[i] {
			t.Fatalf("history[%d] = %+v, want %+v", i, hist[i], want[i])
		}
	}

	// An unknown source is empty, not an error — a peer that never transferred.
	up, down, err = s.QuerySourceTotals(0, 999, "10.10.64.99")
	if err != nil || up != 0 || down != 0 {
		t.Fatalf("unknown source = %d/%d, err=%v; want 0/0/nil", up, down, err)
	}
}

// The sing-box AWG backend has no interface to ask for a handshake, so the peer
// roster reads liveness from here instead: the newest bucket each tunnel IP
// moved bytes in.
func TestLastSeenBySource(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	const live, stale = "10.10.64.2", "10.10.64.3"
	for _, r := range []struct {
		bucket int64
		source string
	}{
		{600, live}, {900, live}, // the newest of several buckets wins
		{120, stale}, // before the cutoff: must not appear at all
	} {
		if err := s.Upsert(r.bucket, r.source, "a.example", "direct", 1, 1); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.LastSeenBySource(300)
	if err != nil {
		t.Fatalf("LastSeenBySource: %v", err)
	}
	if got[live] != 900 {
		t.Errorf("last seen for %s = %d, want its newest bucket 900", live, got[live])
	}
	if _, ok := got[stale]; ok {
		t.Errorf("%s is older than the cutoff and must be absent, got %+v", stale, got)
	}
}

func TestLastSeenBySourceOnAnEmptyStore(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	// A panel that has never sampled must report an empty map, not an error —
	// the roster still has to render.
	got, err := s.LastSeenBySource(0)
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v, err=%v; want an empty map and no error", got, err)
	}
}

func TestHistoryStep(t *testing.T) {
	const day = int64(86400)
	cases := []struct {
		window, want int64
	}{
		{0, 60},          // degenerate
		{-1, 60},         // degenerate
		{3600, 60},       // 1h: minute buckets
		{day, 60},        // 24h: still exactly minute buckets (1440 points)
		{7 * day, 420},   // week: 7 min
		{30 * day, 1800}, // month: 30 min
	}
	for _, tc := range cases {
		got := HistoryStep(tc.window)
		if got != tc.want {
			t.Errorf("HistoryStep(%d) = %d, want %d", tc.window, got, tc.want)
		}
		if got%60 != 0 {
			t.Errorf("HistoryStep(%d) = %d is not a whole number of minutes", tc.window, got)
		}
		if tc.window > 0 && tc.window/got > maxHistoryPoints {
			t.Errorf("HistoryStep(%d) = %d yields %d points, over the %d cap",
				tc.window, got, tc.window/got, maxHistoryPoints)
		}
	}
}

// A long range must not return one point per minute: the peers endpoint ships
// every peer's series in a single response.
func TestQuerySourceHistoryAllSources(t *testing.T) {
	s := openTestStore(t)
	for _, r := range []struct {
		ts   int64
		src  string
		up   int64
		down int64
	}{{60, "10.0.0.2", 1, 2}, {60, "192.168.1.7", 10, 20}, {120, "192.168.1.7", 100, 200}} {
		if err := s.Upsert(r.ts, r.src, "a.example", "direct", r.up, r.down); err != nil {
			t.Fatal(err)
		}
	}
	hist, err := s.QuerySourceHistory(60, 120, "")
	if err != nil {
		t.Fatalf("QuerySourceHistory: %v", err)
	}
	want := []UserHistoryRow{{BucketTs: 60, Upload: 11, Download: 22}, {BucketTs: 120, Upload: 100, Download: 200}}
	if len(hist) != 2 || hist[0] != want[0] || hist[1] != want[1] {
		t.Fatalf("history = %+v, want %+v", hist, want)
	}
}

func TestQuerySourceHistoryCoarsensLongRanges(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	const src = "10.10.64.2"
	const window = int64(30 * 86400)
	// One bucket every 10 minutes across a month: 4320 minute-buckets.
	var want int64
	for ts := int64(0); ts < window; ts += 600 {
		if err := s.Upsert(ts, src, "a.example", "direct", 1, 2); err != nil {
			t.Fatal(err)
		}
		want++
	}

	hist, err := s.QuerySourceHistory(0, window, src)
	if err != nil {
		t.Fatalf("QuerySourceHistory: %v", err)
	}
	if len(hist) == 0 || int64(len(hist)) > maxHistoryPoints {
		t.Fatalf("history points = %d, want 1..%d", len(hist), maxHistoryPoints)
	}
	// Coarsening must not lose bytes: the series still sums to the totals.
	var up, down int64
	for _, r := range hist {
		up += r.Upload
		down += r.Download
	}
	if up != want || down != 2*want {
		t.Fatalf("series sums to %d/%d, want %d/%d — coarsening dropped bytes", up, down, want, 2*want)
	}
	for i := 1; i < len(hist); i++ {
		if hist[i].BucketTs <= hist[i-1].BucketTs {
			t.Fatalf("series not strictly ascending at %d: %+v", i, hist[i-1:i+1])
		}
	}
}

// The dashboard's route graph (#110) needs download per final outbound: the
// first hop of the stored chain, which sing-box lists leaf first.
func TestQueryLeafHistory(t *testing.T) {
	s := openTestStore(t)
	for _, r := range []struct {
		ts    int64
		src   string
		chain string
		down  int64
	}{
		{60, "10.0.0.2", "direct", 5},
		{60, "10.0.0.3", "vless-nl → proxy", 7},
		{60, "10.0.0.4", "vless-nl", 1},
		{120, "10.0.0.2", "-", 3},
	} {
		if err := s.Upsert(r.ts, r.src, "a.example", r.chain, 0, r.down); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.QueryLeafHistory(60, 120)
	if err != nil {
		t.Fatalf("QueryLeafHistory: %v", err)
	}
	want := []LeafHistoryRow{
		{BucketTs: 60, Leaf: "direct", Download: 5},
		{BucketTs: 60, Leaf: "vless-nl", Download: 8},
		{BucketTs: 120, Leaf: "-", Download: 3},
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
