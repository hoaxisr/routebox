package traffic

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

// fetchSnapshot must authenticate with the configured Clash API secret; a
// secret-protected server accepts the request and the snapshot decodes.
func TestFetchSnapshot_SendsBearerSecret(t *testing.T) {
	var mu sync.Mutex
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotAuth = r.Header.Get("Authorization")
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer s3cr3t" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"message":"Unauthorized"}`))
			return
		}
		w.Write([]byte(`{"connections":[{"id":"c1","upload":10,"download":20,"chains":["direct"],"metadata":{"sourceIP":"1.1.1.1","host":"a.com"}}]}`))
	}))
	defer srv.Close()

	s := NewSampler(nil)
	addr := strings.TrimPrefix(srv.URL, "http://")
	snap, err := s.fetchSnapshot(addr, "s3cr3t")
	if err != nil {
		t.Fatalf("fetchSnapshot with secret: %v", err)
	}
	mu.Lock()
	auth := gotAuth
	mu.Unlock()
	if auth != "Bearer s3cr3t" {
		t.Fatalf("Authorization = %q, want %q", auth, "Bearer s3cr3t")
	}
	if len(snap) != 1 || snap[0].ID != "c1" {
		t.Fatalf("snapshot = %+v, want one connection c1", snap)
	}
}

// With no secret configured, no Authorization header may be sent.
func TestFetchSnapshot_NoSecretSendsNoAuth(t *testing.T) {
	var mu sync.Mutex
	gotAuth := "unset"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotAuth = r.Header.Get("Authorization")
		mu.Unlock()
		w.Write([]byte(`{"connections":[]}`))
	}))
	defer srv.Close()

	s := NewSampler(nil)
	addr := strings.TrimPrefix(srv.URL, "http://")
	if _, err := s.fetchSnapshot(addr, ""); err != nil {
		t.Fatalf("fetchSnapshot without secret: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if gotAuth != "" {
		t.Fatalf("Authorization = %q, want none", gotAuth)
	}
}

// A non-200 response (e.g. 401 from a secret-protected server) must surface as
// an error — not decode into an empty snapshot that records zeros forever.
func TestFetchSnapshot_Non200IsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"Unauthorized"}`))
	}))
	defer srv.Close()

	s := NewSampler(nil)
	addr := strings.TrimPrefix(srv.URL, "http://")
	snap, err := s.fetchSnapshot(addr, "")
	if err == nil {
		t.Fatalf("fetchSnapshot on 401 returned nil error (snapshot %+v); want loud failure", snap)
	}
}

// noteSampleErr must log only on transitions: first failure, a different
// failure, or a failure after a successful sample — never on identical repeats
// (a persistent 401 would otherwise spam one line per 60s tick).
func TestNoteSampleErr_DedupesRepeats(t *testing.T) {
	s := NewSampler(nil)
	err401 := fmt.Errorf("clash /connections: status 401")
	err500 := fmt.Errorf("clash /connections: status 500")

	if !s.noteSampleErr(err401) {
		t.Error("first error must log")
	}
	if s.noteSampleErr(err401) {
		t.Error("identical repeat must not log")
	}
	if !s.noteSampleErr(err500) {
		t.Error("different error must log")
	}
	if s.noteSampleErr(err500) {
		t.Error("identical repeat of new error must not log")
	}
	// Successful sample resets the dedupe state (doSample sets lastSampleErr = "").
	s.lastSampleErr = ""
	if !s.noteSampleErr(err500) {
		t.Error("failure after success must log again")
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
