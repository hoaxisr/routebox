package config

import (
	"os"
	"path/filepath"
	"testing"
)

func newMgr(t *testing.T, seed string) *Manager {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(seed), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func specFixture() *AwgServerSpec {
	return &AwgServerSpec{
		PrivateKey: "K", Address: "10.10.0.1/24", ListenPort: 51820, MTU: 1408,
		Obf:   map[string]interface{}{"jc": 4},
		Peers: []AwgServerPeer{{PublicKey: "PUBA", PresharedKey: "PSKA", AllowedIP: "10.10.0.2/32"}},
	}
}

func TestSyncAwgEndpointActive_UpsertAndGate(t *testing.T) {
	m := newMgr(t, `{"log":{}}`)
	changed, err := m.SyncAwgEndpointActive("awg-server", specFixture())
	if err != nil || !changed {
		t.Fatalf("first upsert: changed=%v err=%v", changed, err)
	}
	eps := m.GetActive()["endpoints"].([]interface{})
	if len(eps) != 1 {
		t.Fatalf("want 1 endpoint, got %d", len(eps))
	}
	// Idempotent: same spec => no change, no reload.
	changed, err = m.SyncAwgEndpointActive("awg-server", specFixture())
	if err != nil || changed {
		t.Fatalf("second upsert should be no-op: changed=%v err=%v", changed, err)
	}
}

func TestSyncAwgEndpointActive_Remove(t *testing.T) {
	m := newMgr(t, `{"log":{}}`)
	m.SyncAwgEndpointActive("awg-server", specFixture())
	changed, err := m.SyncAwgEndpointActive("awg-server", nil)
	if err != nil || !changed {
		t.Fatalf("remove: changed=%v err=%v", changed, err)
	}
	if _, ok := m.GetActive()["endpoints"]; ok {
		t.Fatalf("endpoints should be dropped when empty")
	}
}

func TestSyncAwgEndpointActive_DeferOnDraft(t *testing.T) {
	m := newMgr(t, `{"log":{}}`)
	if err := m.EnsureDraft(); err != nil {
		t.Fatal(err)
	}
	changed, err := m.SyncAwgEndpointActive("awg-server", specFixture())
	if err != nil || changed {
		t.Fatalf("must defer while draft pending: changed=%v err=%v", changed, err)
	}
}
