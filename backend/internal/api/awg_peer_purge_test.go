package api

import (
	"path/filepath"
	"testing"

	"routebox/backend/internal/awg"
	"routebox/backend/internal/traffic"
)

func TestPurgePeerTrafficDropsHistoryAndSource(t *testing.T) {
	store, err := traffic.OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_ = store.UpsertUser(60, awg.TrafficKey("PK"), 1, 2)
	_ = store.UpsertUser(60, awg.TrafficKey("OTHER"), 1, 2)
	_ = store.Upsert(60, "10.10.0.2", "a.com", "direct", 1, 2)

	purgePeerTraffic(store, "PK", "10.10.0.2/32")

	if up, down, _ := store.QueryKeysTotals(0, 120, []string{awg.TrafficKey("PK")}); up+down != 0 {
		t.Fatalf("peer history left: %d/%d", up, down)
	}
	if up, _, _ := store.QueryKeysTotals(0, 120, []string{awg.TrafficKey("OTHER")}); up != 1 {
		t.Fatal("another peer's history was purged")
	}
	if up, down, _ := store.QuerySourceTotals(0, 120, "10.10.0.2"); up+down != 0 {
		t.Fatal("tunnel-IP rows left")
	}
}
