package traffic

import (
	"testing"

	"routebox/backend/internal/v2stats"
)

func TestUserDeltas_FirstSnapshotIsFullVolume(t *testing.T) {
	s := NewUserSampler(nil)
	d := s.computeUserDeltas(map[string]v2stats.Counters{"alice": {Uplink: 100, Downlink: 200}})
	if d["alice"].Upload != 100 || d["alice"].Download != 200 {
		t.Fatalf("got %+v, want alice 100/200", d)
	}
}

func TestUserDeltas_GrowthEmitsDelta(t *testing.T) {
	s := NewUserSampler(nil)
	s.computeUserDeltas(map[string]v2stats.Counters{"alice": {Uplink: 100, Downlink: 200}})
	d := s.computeUserDeltas(map[string]v2stats.Counters{"alice": {Uplink: 150, Downlink: 260}})
	if d["alice"].Upload != 50 || d["alice"].Download != 60 {
		t.Fatalf("got %+v, want 50/60", d["alice"])
	}
}

func TestUserDeltas_NoChangeNoEntry(t *testing.T) {
	s := NewUserSampler(nil)
	s.computeUserDeltas(map[string]v2stats.Counters{"alice": {Uplink: 100, Downlink: 200}})
	d := s.computeUserDeltas(map[string]v2stats.Counters{"alice": {Uplink: 100, Downlink: 200}})
	if len(d) != 0 {
		t.Fatalf("got %+v, want empty", d)
	}
}

func TestUserDeltas_CounterResetCountsCurrentNeverNegative(t *testing.T) {
	s := NewUserSampler(nil)
	s.computeUserDeltas(map[string]v2stats.Counters{"alice": {Uplink: 1000, Downlink: 1000}})
	// sing-box restarted: counter dropped. Emit current value, never negative.
	d := s.computeUserDeltas(map[string]v2stats.Counters{"alice": {Uplink: 30, Downlink: 40}})
	if d["alice"].Upload != 30 || d["alice"].Download != 40 {
		t.Fatalf("got %+v, want 30/40 (reset)", d["alice"])
	}
	// next tick 30->130 must be a normal +100 delta against the NEW baseline.
	d = s.computeUserDeltas(map[string]v2stats.Counters{"alice": {Uplink: 130, Downlink: 40}})
	if d["alice"].Upload != 100 || d["alice"].Download != 0 {
		t.Fatalf("got %+v, want +100/0 after rebaseline", d["alice"])
	}
}

func TestUserDeltas_VanishedUserEvictedThenFreshOnReturn(t *testing.T) {
	s := NewUserSampler(nil)
	s.computeUserDeltas(map[string]v2stats.Counters{"alice": {Uplink: 100, Downlink: 100}})
	s.computeUserDeltas(map[string]v2stats.Counters{}) // alice gone
	if _, ok := s.lastSeen["alice"]; ok {
		t.Fatal("alice should be evicted from lastSeen")
	}
	// alice returns → treated as new (full value), not a giant delta vs old.
	d := s.computeUserDeltas(map[string]v2stats.Counters{"alice": {Uplink: 5, Downlink: 0}})
	if d["alice"].Upload != 5 {
		t.Fatalf("reappeared alice = %+v, want 5", d["alice"])
	}
}

func TestUserDeltas_NewUserAlongsideExisting(t *testing.T) {
	s := NewUserSampler(nil)
	s.computeUserDeltas(map[string]v2stats.Counters{"alice": {Uplink: 100}})
	d := s.computeUserDeltas(map[string]v2stats.Counters{
		"alice": {Uplink: 120},
		"bob":   {Uplink: 7},
	})
	if d["alice"].Upload != 20 {
		t.Errorf("alice = %+v, want +20", d["alice"])
	}
	if d["bob"].Upload != 7 {
		t.Errorf("bob = %+v, want full 7", d["bob"])
	}
}
