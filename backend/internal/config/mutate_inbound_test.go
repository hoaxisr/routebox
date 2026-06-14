package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// newManagerWithInbound writes a config.json with one vless inbound (one user)
// and returns a loaded Manager.
func newManagerWithInbound(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := `{
	  "inbounds": [
	    {"type":"vless","tag":"vless-in","listen_port":443,
	     "users":[{"name":"alice","uuid":"u-1"}]}
	  ],
	  "outbounds": [{"type":"direct","tag":"direct"}]
	}`
	if err := os.WriteFile(path, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func inboundUserCount(t *testing.T, cfg map[string]interface{}, tag string) int {
	t.Helper()
	inbounds, _ := cfg["inbounds"].([]interface{})
	for _, ib := range inbounds {
		obj, ok := ib.(map[string]interface{})
		if !ok {
			continue
		}
		if tg, _ := obj["tag"].(string); tg == tag {
			us, _ := obj["users"].([]interface{})
			return len(us)
		}
	}
	t.Fatalf("inbound %q not found", tag)
	return -1
}

// TestMutateInboundStagesToDraftNotActive proves MutateInbound stages into the
// draft only: the active (on-disk) config is unchanged, the working/draft shows
// the change.
func TestMutateInboundStagesToDraftNotActive(t *testing.T) {
	m := newManagerWithInbound(t)

	err := m.MutateInbound("vless-in", func(inbound map[string]interface{}) error {
		existing, _ := inbound["users"].([]interface{})
		inbound["users"] = append(existing, map[string]interface{}{"name": "bob", "uuid": "u-2"})
		return nil
	})
	if err != nil {
		t.Fatalf("MutateInbound: %v", err)
	}

	if got := inboundUserCount(t, m.GetActive(), "vless-in"); got != 1 {
		t.Fatalf("active must be unchanged (1 user), got %d", got)
	}
	if got := inboundUserCount(t, m.Get(), "vless-in"); got != 2 {
		t.Fatalf("draft must show 2 users, got %d", got)
	}
	if !m.HasDraft() {
		t.Fatal("expected a draft to exist after MutateInbound")
	}

	// Discard fully reverts: working returns to active (1 user), no draft.
	if err := m.DiscardDraft(); err != nil {
		t.Fatal(err)
	}
	if m.HasDraft() {
		t.Fatal("draft must be gone after discard")
	}
	if got := inboundUserCount(t, m.Get(), "vless-in"); got != 1 {
		t.Fatalf("after discard, working must be back to 1 user, got %d", got)
	}
}

// TestMutateInboundConcurrent runs N concurrent MutateInbound appends to the
// same inbound; all N must land (no lost update). Run with -race.
func TestMutateInboundConcurrent(t *testing.T) {
	m := newManagerWithInbound(t)

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			errs[i] = m.MutateInbound("vless-in", func(inbound map[string]interface{}) error {
				existing, _ := inbound["users"].([]interface{})
				inbound["users"] = append(existing, map[string]interface{}{
					"name": fmt.Sprintf("u%d", i),
					"uuid": fmt.Sprintf("uuid-%d", i),
				})
				return nil
			})
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("MutateInbound[%d]: %v", i, err)
		}
	}

	// Started with 1 user; all N appends must have landed -> 1+N.
	if got := inboundUserCount(t, m.Get(), "vless-in"); got != 1+n {
		t.Fatalf("expected %d users after %d concurrent appends, got %d (lost update?)", 1+n, n, got)
	}
	// Active stays untouched at 1.
	if got := inboundUserCount(t, m.GetActive(), "vless-in"); got != 1 {
		t.Fatalf("active must remain at 1 user, got %d", got)
	}
}

// TestMutateInboundUnknownTag returns ErrInboundNotFound and stages nothing.
func TestMutateInboundUnknownTag(t *testing.T) {
	m := newManagerWithInbound(t)
	called := false
	err := m.MutateInbound("nope", func(inbound map[string]interface{}) error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("expected error for unknown tag")
	}
	if called {
		t.Fatal("fn must not be called for an unknown tag")
	}
}

// TestMutateInboundFnErrorDoesNotPersist proves an fn error aborts the staged
// change: the inbound entry in the draft is not modified.
func TestMutateInboundFnErrorDoesNotPersist(t *testing.T) {
	m := newManagerWithInbound(t)
	sentinel := fmt.Errorf("boom")
	err := m.MutateInbound("vless-in", func(inbound map[string]interface{}) error {
		inbound["users"] = append(inbound["users"].([]interface{}),
			map[string]interface{}{"name": "ghost", "uuid": "u-x"})
		return sentinel
	})
	if err != sentinel {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	// The change must NOT have been written to the draft.
	if got := inboundUserCount(t, m.Get(), "vless-in"); got != 1 {
		t.Fatalf("fn error must not persist; working should have 1 user, got %d", got)
	}
}
