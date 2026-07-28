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
		got := historyStep(tc.window)
		if got != tc.want {
			t.Errorf("historyStep(%d) = %d, want %d", tc.window, got, tc.want)
		}
		if got%60 != 0 {
			t.Errorf("historyStep(%d) = %d is not a whole number of minutes", tc.window, got)
		}
		if tc.window > 0 && tc.window/got > maxHistoryPoints {
			t.Errorf("historyStep(%d) = %d yields %d points, over the %d cap",
				tc.window, got, tc.window/got, maxHistoryPoints)
		}
	}
}

// A long range must not return one point per minute: the peers endpoint ships
// every peer's series in a single response.
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
