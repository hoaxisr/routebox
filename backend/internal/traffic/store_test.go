package traffic

import (
	"path/filepath"
	"testing"
)

func TestStore_OpenAndQueryEmpty(t *testing.T) {
	tmp := t.TempDir()
	s, err := OpenStore(filepath.Join(tmp, "t.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	rows, err := s.QueryAggregate(0, 9999999999, "", "", "")
	if err != nil {
		t.Fatalf("QueryAggregate: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("len = %d, want 0", len(rows))
	}
}

func TestStore_UpsertAndQuery(t *testing.T) {
	tmp := t.TempDir()
	s, err := OpenStore(filepath.Join(tmp, "t.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	if err := s.Upsert(60, "1.1.1.1", "a.com", "direct", 100, 200); err != nil {
		t.Fatalf("Upsert 1: %v", err)
	}
	if err := s.Upsert(60, "1.1.1.1", "a.com", "direct", 50, 100); err != nil {
		t.Fatalf("Upsert 2: %v", err)
	}
	rows, err := s.QueryAggregate(0, 9999999999, "", "", "")
	if err != nil {
		t.Fatalf("QueryAggregate: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len = %d, want 1", len(rows))
	}
	if rows[0].Upload != 150 {
		t.Errorf("Upload = %d, want 150", rows[0].Upload)
	}
	if rows[0].Download != 300 {
		t.Errorf("Download = %d, want 300", rows[0].Download)
	}
}

func TestStore_QueryWithFilter(t *testing.T) {
	tmp := t.TempDir()
	s, _ := OpenStore(filepath.Join(tmp, "t.db"))
	defer s.Close()
	s.Upsert(60, "1.1.1.1", "a.com", "direct", 100, 0)
	s.Upsert(60, "2.2.2.2", "a.com", "direct", 100, 0)

	rows, _ := s.QueryAggregate(0, 9999999999, "1.1.1.1", "", "")
	if len(rows) != 1 || rows[0].Source != "1.1.1.1" {
		t.Errorf("got %+v, want one row for 1.1.1.1", rows)
	}
}

func TestStore_PruneOlderThan(t *testing.T) {
	tmp := t.TempDir()
	s, _ := OpenStore(filepath.Join(tmp, "t.db"))
	defer s.Close()
	s.Upsert(60, "x", "a", "c", 1, 0)
	s.Upsert(120, "x", "a", "c", 1, 0)
	if err := s.PruneOlderThan(100); err != nil {
		t.Fatalf("PruneOlderThan: %v", err)
	}
	rows, _ := s.QueryAggregate(0, 9999999999, "", "", "")
	if len(rows) != 1 || rows[0].Upload != 1 {
		t.Errorf("after prune got %+v, want one row at ts=120", rows)
	}
}
