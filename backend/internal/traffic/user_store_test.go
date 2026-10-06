package traffic

import (
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUserStore_DeleteUsers(t *testing.T) {
	s := openTestStore(t)
	for _, u := range []string{"alice", "alice-sub", "bob"} {
		if err := s.UpsertUser(60, u, 100, 200); err != nil {
			t.Fatal(err)
		}
	}
	// Delete alice under both her names; bob must survive.
	if err := s.DeleteUsers([]string{"alice", "alice-sub"}); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"alice", "alice-sub"} {
		up, dn, err := s.QueryUserTotals(0, 1<<62, gone)
		if err != nil {
			t.Fatal(err)
		}
		if up != 0 || dn != 0 {
			t.Errorf("%s not purged: up=%d dn=%d", gone, up, dn)
		}
	}
	if up, _, _ := s.QueryUserTotals(0, 1<<62, "bob"); up != 100 {
		t.Errorf("bob wrongly purged: up=%d", up)
	}
	// Empty list is a no-op, not an error.
	if err := s.DeleteUsers(nil); err != nil {
		t.Fatalf("empty DeleteUsers should be a no-op: %v", err)
	}
}

func TestUserStore_TotalsAndHistory(t *testing.T) {
	s := openTestStore(t)
	if err := s.UpsertUser(60, "alice", 100, 200); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertUser(60, "alice", 50, 0); err != nil { // same bucket sums
		t.Fatal(err)
	}
	if err := s.UpsertUser(120, "alice", 10, 10); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertUser(120, "bob", 5, 5); err != nil {
		t.Fatal(err)
	}

	up, down, err := s.QueryUserTotals(0, 9999999999, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if up != 160 || down != 210 {
		t.Errorf("alice totals = %d/%d, want 160/210", up, down)
	}

	hist, err := s.QueryUserHistory(0, 9999999999, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 2 {
		t.Fatalf("history len = %d, want 2 buckets", len(hist))
	}
	if hist[0].BucketTs != 60 || hist[0].Upload != 150 || hist[0].Download != 200 {
		t.Errorf("bucket0 = %+v, want {60,150,200}", hist[0])
	}
	if hist[1].BucketTs != 120 || hist[1].Upload != 10 || hist[1].Download != 10 {
		t.Errorf("bucket1 = %+v, want {120,10,10}", hist[1])
	}
}

func TestUserStore_UnknownUserIsZero(t *testing.T) {
	s := openTestStore(t)
	up, down, err := s.QueryUserTotals(0, 9999999999, "ghost")
	if err != nil {
		t.Fatal(err)
	}
	if up != 0 || down != 0 {
		t.Errorf("ghost = %d/%d, want 0/0", up, down)
	}
	hist, _ := s.QueryUserHistory(0, 9999999999, "ghost")
	if len(hist) != 0 {
		t.Errorf("ghost history = %v, want empty", hist)
	}
}

func TestUserStore_PruneAndReset(t *testing.T) {
	s := openTestStore(t)
	_ = s.UpsertUser(60, "alice", 1, 1)
	_ = s.UpsertUser(7200, "alice", 20, 20)
	if err := s.PruneUserOlderThan(3600); err != nil {
		t.Fatal(err)
	}
	up, _, _ := s.QueryUserTotals(0, 9999999999, "alice")
	if up != 20 {
		t.Errorf("after prune upload = %d, want 20", up)
	}
	if err := s.ResetUsers(); err != nil {
		t.Fatal(err)
	}
	up, _, _ = s.QueryUserTotals(0, 9999999999, "alice")
	if up != 0 {
		t.Errorf("after reset upload = %d, want 0", up)
	}
}

func TestQueryKeysTotalsSumsAcrossKeys(t *testing.T) {
	s := openTestStore(t)
	_ = s.UpsertUser(60, "a", 10, 100)
	_ = s.UpsertUser(120, "b", 5, 50)
	_ = s.UpsertUser(120, "c", 1000, 1000) // not asked for
	up, down, err := s.QueryKeysTotals(0, 200, []string{"a", "b"})
	if err != nil || up != 15 || down != 150 {
		t.Fatalf("got %d/%d err=%v, want 15/150", up, down, err)
	}
	up, down, err = s.QueryKeysTotals(0, 200, nil)
	if err != nil || up != 0 || down != 0 {
		t.Fatalf("no keys: got %d/%d err=%v, want 0/0", up, down, err)
	}
}

func TestQueryKeysHistoryIsSteppedAndMerged(t *testing.T) {
	s := openTestStore(t)
	// Two keys in the same minute merge into one point.
	_ = s.UpsertUser(600, "a", 1, 10)
	_ = s.UpsertUser(600, "b", 2, 20)
	_ = s.UpsertUser(660, "a", 4, 40)
	h, err := s.QueryKeysHistory(0, 3600, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 2 || h[0].BucketTs != 600 || h[0].Upload != 3 || h[0].Download != 30 || h[1].Upload != 4 {
		t.Fatalf("got %+v", h)
	}
}

// A month of minute buckets for one key must come back bounded, or the
// consumers endpoint ships ~43k points per row.
func TestQueryKeysHistoryMonthIsBounded(t *testing.T) {
	s := openTestStore(t)
	const month = 30 * 86400
	for ts := int64(0); ts < month; ts += 60 {
		_ = s.UpsertUser(ts, "a", 1, 1)
	}
	h, err := s.QueryKeysHistory(0, month, []string{"a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(h) > 1441 {
		t.Fatalf("month history has %d points, want <= 1441", len(h))
	}
}
