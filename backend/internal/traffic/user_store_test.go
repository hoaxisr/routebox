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
