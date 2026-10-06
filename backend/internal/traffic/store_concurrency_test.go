package traffic

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Several samplers write traffic.db from their own goroutines (Clash sampler,
// v2ray user sampler, mtproto flusher, AWG sweep observer). database/sql hands
// each one its own SQLite connection, so two writes landing at once used to
// surface as "database is locked (5) (SQLITE_BUSY)" and the delta was dropped.
func TestStore_ConcurrentWritersNeverFail(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	const workers, perWorker = 8, 200
	errs := make(chan error, workers*perWorker*2)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				bucket := int64(60 * (i % 7))
				if err := s.UpsertUser(bucket, "alice", 1, 2); err != nil {
					errs <- err
				}
				if err := s.Upsert(bucket, "10.0.0.5", "a.com", "direct", 3, 4); err != nil {
					errs <- err
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	failed := 0
	for err := range errs {
		failed++
		if failed <= 3 {
			t.Errorf("write failed: %v", err)
		}
	}
	if failed > 0 {
		t.Fatalf("%d of %d writes failed", failed, workers*perWorker*2)
	}

	const n = workers * perWorker
	up, down, err := s.QueryUserTotals(0, 9999999999, "alice")
	if err != nil {
		t.Fatalf("QueryUserTotals: %v", err)
	}
	if up != n*1 || down != n*2 {
		t.Errorf("user totals = %d/%d, want %d/%d", up, down, n*1, n*2)
	}
	rows, err := s.QueryAggregate(0, 9999999999, "10.0.0.5", "", "")
	if err != nil {
		t.Fatalf("QueryAggregate: %v", err)
	}
	var aup, adown int64
	for _, r := range rows {
		aup += r.Upload
		adown += r.Download
	}
	if aup != n*3 || adown != n*4 {
		t.Errorf("source totals = %d/%d, want %d/%d", aup, adown, n*3, n*4)
	}
}

// Deterministic form of the same defect: a second connection holds the write
// lock; the store's write must block until it is released and then succeed,
// never report SQLITE_BUSY straight away.
func TestStore_WriteWaitsForLockedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	other, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open second handle: %v", err)
	}
	defer other.Close()
	tx, err := other.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	// A real write inside the tx takes the RESERVED lock and holds it until commit.
	if _, err := tx.Exec(`INSERT INTO user_traffic (bucket_ts, user, upload, download) VALUES (0, 'holder', 1, 1)`); err != nil {
		t.Fatalf("holder write: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- s.UpsertUser(60, "alice", 10, 20) }()

	select {
	case err := <-done:
		t.Fatalf("store write returned while the database was locked: err=%v (want it to wait)", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("store write after release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("store write never completed after the lock was released")
	}
	up, down, err := s.QueryUserTotals(0, 9999999999, "alice")
	if err != nil {
		t.Fatalf("QueryUserTotals: %v", err)
	}
	if up != 10 || down != 20 {
		t.Errorf("totals = %d/%d, want 10/20", up, down)
	}
}
