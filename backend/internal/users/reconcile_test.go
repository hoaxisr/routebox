package users

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// cfg builds an active config from inbound maps.
func cfg(inbounds ...map[string]interface{}) map[string]interface{} {
	arr := make([]interface{}, len(inbounds))
	for i, ib := range inbounds {
		arr[i] = ib
	}
	return map[string]interface{}{"inbounds": arr}
}

func vlessIn(tag string, users ...map[string]interface{}) map[string]interface{} {
	arr := make([]interface{}, len(users))
	for i, u := range users {
		arr[i] = u
	}
	return map[string]interface{}{"type": "vless", "tag": tag, "users": arr}
}

func newStore(t *testing.T) *Manager {
	t.Helper()
	return NewManager(filepath.Join(t.TempDir(), "users.toml"))
}

func TestReconcileImportsNewUser(t *testing.T) {
	m := newStore(t)
	active := cfg(vlessIn("vless-in", map[string]interface{}{"name": "alice", "uuid": "u-1", "flow": "xtls-rprx-vision"}))

	changed, err := m.Reconcile(active)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true on first import")
	}
	list := m.List()
	if len(list) != 1 {
		t.Fatalf("want 1 user, got %d", len(list))
	}
	if list[0].ID == "" {
		t.Fatalf("imported user must get an ID")
	}
	if !list[0].Enabled {
		t.Fatalf("imported user should default Enabled=true")
	}
	b := list[0].Bindings[0]
	if len(list[0].Bindings) != 1 || b.Credential != "u-1" || b.Protocol != "vless" || b.Flow != "xtls-rprx-vision" {
		t.Fatalf("unexpected binding: %#v", list[0].Bindings)
	}
}

func TestReconcileIdempotent(t *testing.T) {
	m := newStore(t)
	active := cfg(vlessIn("vless-in", map[string]interface{}{"name": "alice", "uuid": "u-1"}))
	if _, err := m.Reconcile(active); err != nil {
		t.Fatal(err)
	}
	changed, err := m.Reconcile(active)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatalf("second reconcile on identical active should be no-op (changed=false)")
	}
}

// No-file-rewrite-on-unchanged. NOTE on coarse-mtime filesystems: a 10ms sleep
// is inserted between the two reconciles so that, IF the second one wrongly
// rewrote the file, the mtime would differ even on second-resolution mtimes.
// (We assert equality, so the sleep cannot cause a false pass.)
func TestReconcileUnchangedDoesNotRewriteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.toml")
	m := NewManager(path)
	active := cfg(vlessIn("vless-in", map[string]interface{}{"name": "alice", "uuid": "u-1"}))
	if _, err := m.Reconcile(active); err != nil {
		t.Fatal(err)
	}
	info1, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, err := m.Reconcile(active); err != nil {
		t.Fatal(err)
	}
	info2, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Fatalf("unchanged reconcile must not rewrite users.toml")
	}
}

func TestReconcileUpdatesCachedFields(t *testing.T) {
	m := newStore(t)
	if _, err := m.Reconcile(cfg(vlessIn("vless-in",
		map[string]interface{}{"name": "alice", "uuid": "u-1", "flow": "f1"}))); err != nil {
		t.Fatal(err)
	}
	id := m.List()[0].ID
	// Rename + flow change in config; credential (uuid) unchanged -> same binding,
	// stable ID, refreshed cache fields.
	changed, err := m.Reconcile(cfg(vlessIn("vless-in",
		map[string]interface{}{"name": "alice-renamed", "uuid": "u-1", "flow": "f2"})))
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatalf("rename/flow change should mark changed")
	}
	list := m.List()
	if len(list) != 1 || list[0].ID != id {
		t.Fatalf("ID must be stable across cache update: %#v", list)
	}
	if list[0].Bindings[0].Name != "alice-renamed" || list[0].Bindings[0].Flow != "f2" {
		t.Fatalf("cached fields not updated: %#v", list[0].Bindings[0])
	}
}

func TestReconcileA1DeletesVanishedCredential(t *testing.T) {
	m := newStore(t)
	if _, err := m.Reconcile(cfg(vlessIn("vless-in",
		map[string]interface{}{"name": "alice", "uuid": "u-1"}))); err != nil {
		t.Fatal(err)
	}
	changed, err := m.Reconcile(cfg(vlessIn("vless-in"))) // credential removed
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatalf("vanished credential should mark changed")
	}
	if len(m.List()) != 0 {
		t.Fatalf("A1: orphaned user must be auto-deleted, got %#v", m.List())
	}
}

func TestReconcileEmptyActiveClearsRegistry(t *testing.T) {
	m := newStore(t)
	if _, err := m.Reconcile(cfg(vlessIn("vless-in", map[string]interface{}{"uuid": "u-1"}))); err != nil {
		t.Fatal(err)
	}
	changed, err := m.Reconcile(cfg()) // no inbounds at all
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatalf("clearing all server users should mark changed")
	}
	if len(m.List()) != 0 {
		t.Fatalf("registry must be empty after all credentials vanish: %#v", m.List())
	}
}

func TestReconcilePreservesMultiBinding(t *testing.T) {
	m := newStore(t)
	_ = m.Put(&PanelUser{
		ID: "fixed", Name: "alice", Enabled: true,
		Bindings: []Binding{
			{InboundTag: "vless-in", Credential: "u-1", Protocol: "vless", Name: "alice"},
			{InboundTag: "hy2-in", Credential: "p-1", Protocol: "hysteria2", Name: "alice"},
		},
	})
	active := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{"type": "vless", "tag": "vless-in", "users": []interface{}{
				map[string]interface{}{"name": "alice", "uuid": "u-1"}}},
			map[string]interface{}{"type": "hysteria2", "tag": "hy2-in", "users": []interface{}{
				map[string]interface{}{"name": "alice", "password": "p-1"}}},
		},
	}
	changed, err := m.Reconcile(active)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatalf("matching multi-binding should be no-op")
	}
	u, _ := m.Get("fixed")
	if len(u.Bindings) != 2 {
		t.Fatalf("multi-binding must be preserved, got %#v", u.Bindings)
	}
}

func TestReconcileDropsOneBindingKeepsUser(t *testing.T) {
	m := newStore(t)
	_ = m.Put(&PanelUser{
		ID: "fixed", Name: "alice", Enabled: true,
		Bindings: []Binding{
			{InboundTag: "vless-in", Credential: "u-1", Protocol: "vless", Name: "alice"},
			{InboundTag: "hy2-in", Credential: "p-1", Protocol: "hysteria2", Name: "alice"},
		},
	})
	active := cfg(vlessIn("vless-in", map[string]interface{}{"name": "alice", "uuid": "u-1"}))
	changed, err := m.Reconcile(active)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatalf("dropping a binding should mark changed")
	}
	u, ok := m.Get("fixed")
	if !ok || len(u.Bindings) != 1 || u.Bindings[0].InboundTag != "vless-in" {
		t.Fatalf("expected user kept with single surviving binding, got %#v", u)
	}
}

// MUST-FIX 5 (graft from B): two identical (tag, credential) users in active must
// NOT double-import. Within-pass imports must be registered into the binding
// index so the second duplicate is treated as a match.
func TestReconcileDuplicateCredentialCollapse(t *testing.T) {
	m := newStore(t)
	active := cfg(vlessIn("vless-in",
		map[string]interface{}{"name": "A", "uuid": "dup"},
		map[string]interface{}{"name": "B", "uuid": "dup"},
	))
	if _, err := m.Reconcile(active); err != nil {
		t.Fatal(err)
	}
	if len(m.List()) != 1 {
		t.Fatalf("duplicate (tag,credential) must collapse to one registry entry, got %d", len(m.List()))
	}
}

// Draft-vs-active isolation: the reconciler mirrors exactly the map it is handed
// (the caller passes GetActive()); a credential that exists only in some draft is
// never imported because the reconciler never sees it.
func TestReconcileMirrorsOnlyGivenMap(t *testing.T) {
	m := newStore(t)
	active := cfg(vlessIn("vless-in", map[string]interface{}{"uuid": "active-only"}))
	if _, err := m.Reconcile(active); err != nil {
		t.Fatal(err)
	}
	got := m.List()
	if len(got) != 1 || got[0].Bindings[0].Credential != "active-only" {
		t.Fatalf("reconciler must mirror exactly the passed map: %#v", got)
	}
}

func TestReconcileNaiveAndHysteria2(t *testing.T) {
	m := newStore(t)
	active := map[string]interface{}{"inbounds": []interface{}{
		map[string]interface{}{"type": "naive", "tag": "n", "users": []interface{}{
			map[string]interface{}{"username": "alice", "password": "pw"},
		}},
		map[string]interface{}{"type": "hysteria2", "tag": "h", "users": []interface{}{
			map[string]interface{}{"name": "Laptop", "password": "secret"},
		}},
	}}
	if _, err := m.Reconcile(active); err != nil {
		t.Fatal(err)
	}
	list := m.List()
	if len(list) != 2 {
		t.Fatalf("want 2 users, got %d: %#v", len(list), list)
	}
	byCred := map[string]Binding{}
	for _, u := range list {
		byCred[u.Bindings[0].Credential] = u.Bindings[0]
	}
	if b := byCred["alice"]; b.Protocol != "naive" || b.InboundTag != "n" {
		t.Fatalf("naive binding wrong: %#v", b)
	}
	if b := byCred["secret"]; b.Protocol != "hysteria2" || b.InboundTag != "h" {
		t.Fatalf("hysteria2 binding wrong: %#v", b)
	}
}
