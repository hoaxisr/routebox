package traffic

import (
	"testing"
)

func TestComputeDeltas_NewConnection(t *testing.T) {
	s := NewSampler(nil)
	deltas := s.computeDeltas([]ConnectionSample{
		{ID: "c1", Source: "1.1.1.1", Domain: "a.com", Chain: "direct", Upload: 100, Download: 200},
	})
	if len(deltas) != 1 {
		t.Fatalf("len = %d, want 1", len(deltas))
	}
	if deltas[0].Upload != 100 || deltas[0].Download != 200 {
		t.Errorf("got upload=%d download=%d, want 100/200", deltas[0].Upload, deltas[0].Download)
	}
}

func TestComputeDeltas_KnownConnectionEmitsOnlyDelta(t *testing.T) {
	s := NewSampler(nil)
	s.computeDeltas([]ConnectionSample{
		{ID: "c1", Source: "1.1.1.1", Domain: "a.com", Chain: "direct", Upload: 100, Download: 200},
	})
	deltas := s.computeDeltas([]ConnectionSample{
		{ID: "c1", Source: "1.1.1.1", Domain: "a.com", Chain: "direct", Upload: 150, Download: 250},
	})
	if len(deltas) != 1 || deltas[0].Upload != 50 || deltas[0].Download != 50 {
		t.Errorf("got %+v, want upload=50 download=50", deltas)
	}
}

func TestComputeDeltas_ZeroDeltaSkipped(t *testing.T) {
	s := NewSampler(nil)
	s.computeDeltas([]ConnectionSample{
		{ID: "c1", Source: "1.1.1.1", Domain: "a.com", Chain: "direct", Upload: 100, Download: 200},
	})
	deltas := s.computeDeltas([]ConnectionSample{
		{ID: "c1", Source: "1.1.1.1", Domain: "a.com", Chain: "direct", Upload: 100, Download: 200},
	})
	if len(deltas) != 0 {
		t.Errorf("len = %d, want 0 (no delta)", len(deltas))
	}
}

func TestComputeDeltas_ClosedConnectionDropped(t *testing.T) {
	s := NewSampler(nil)
	s.computeDeltas([]ConnectionSample{
		{ID: "c1", Source: "x", Domain: "y", Chain: "z", Upload: 100, Download: 0},
	})
	// next tick has no c1 — it's closed; we don't emit anything for it but
	// must drop its last-seen state so a new c1 with same ID starts fresh.
	_ = s.computeDeltas([]ConnectionSample{})
	if _, ok := s.lastSeen["c1"]; ok {
		t.Errorf("expected c1 to be evicted from lastSeen")
	}
}

func TestComputeDeltas_AggregatesSameKey(t *testing.T) {
	s := NewSampler(nil)
	deltas := s.computeDeltas([]ConnectionSample{
		{ID: "c1", Source: "1.1.1.1", Domain: "a.com", Chain: "direct", Upload: 100, Download: 0},
		{ID: "c2", Source: "1.1.1.1", Domain: "a.com", Chain: "direct", Upload: 50, Download: 0},
	})
	// Two different connections, same (source, domain, chain) — sampler may emit
	// either one combined entry or two separate; the store's Upsert sums them.
	// Either contract is fine as long as totals are correct.
	var total int64
	for _, d := range deltas {
		total += d.Upload
	}
	if total != 150 {
		t.Errorf("total upload = %d, want 150", total)
	}
}

func TestComputeDeltas_HandlesCounterReset(t *testing.T) {
	s := NewSampler(nil)
	s.computeDeltas([]ConnectionSample{
		{ID: "c1", Source: "x", Domain: "y", Chain: "z", Upload: 1000, Download: 0},
	})
	// Counter goes backwards (process restart, sing-box re-counts) — treat as fresh.
	deltas := s.computeDeltas([]ConnectionSample{
		{ID: "c1", Source: "x", Domain: "y", Chain: "z", Upload: 100, Download: 0},
	})
	if len(deltas) != 1 || deltas[0].Upload != 100 {
		t.Errorf("got %+v, want one entry with upload=100 (reset handled)", deltas)
	}
}
