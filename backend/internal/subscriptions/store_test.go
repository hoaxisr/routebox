package subscriptions

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Manager {
	t.Helper()
	return NewManager(filepath.Join(t.TempDir(), "subscriptions.toml"))
}

func TestAddListGet(t *testing.T) {
	m := newTestStore(t)
	sub, err := m.Add("Home VPN", "https://example.com/sub", 12)
	if err != nil {
		t.Fatal(err)
	}
	if sub.ID != "home-vpn" {
		t.Fatalf("ID = %q, want home-vpn", sub.ID)
	}
	if sub.Name != "Home VPN" || sub.URL != "https://example.com/sub" || sub.IntervalHrs != 12 {
		t.Fatalf("unexpected sub: %+v", sub)
	}
	if sub.LastUpdated != 0 || sub.NodeCount != 0 || sub.LastError != "" {
		t.Fatalf("fresh sub should have zero result fields: %+v", sub)
	}
	if all := m.List(); len(all) != 1 || all[0].ID != "home-vpn" {
		t.Fatalf("List = %+v", all)
	}
	if got, ok := m.Get("home-vpn"); !ok || got.Name != "Home VPN" {
		t.Fatalf("Get = %+v, ok=%v", got, ok)
	}
	if _, ok := m.Get("missing"); ok {
		t.Fatal("Get(missing) should be false")
	}
}

func TestAddDuplicateName(t *testing.T) {
	m := newTestStore(t)
	if _, err := m.Add("VPN", "https://a", 6); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Add("VPN", "https://b", 6); err == nil {
		t.Fatal("expected error on duplicate name")
	}
	if _, err := m.Add("v p n", "https://c", 6); err == nil {
		t.Fatal("expected error on slug collision")
	}
}

func TestUpdate(t *testing.T) {
	m := newTestStore(t)
	if _, err := m.Add("VPN", "https://a", 6); err != nil {
		t.Fatal(err)
	}
	if err := m.Update("vpn", "https://b", 24); err != nil {
		t.Fatal(err)
	}
	got, _ := m.Get("vpn")
	if got.URL != "https://b" || got.IntervalHrs != 24 || got.Name != "VPN" {
		t.Fatalf("update wrong: %+v", got)
	}
	if err := m.Update("missing", "https://x", 6); err == nil {
		t.Fatal("Update(missing) should error")
	}
}

func TestDelete(t *testing.T) {
	m := newTestStore(t)
	if _, err := m.Add("VPN", "https://a", 6); err != nil {
		t.Fatal(err)
	}
	if err := m.Delete("vpn"); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Get("vpn"); ok {
		t.Fatal("sub should be gone after Delete")
	}
	if err := m.Delete("vpn"); err == nil {
		t.Fatal("Delete(missing) should error")
	}
}

func TestSetResult(t *testing.T) {
	m := newTestStore(t)
	if _, err := m.Add("VPN", "https://a", 6); err != nil {
		t.Fatal(err)
	}
	if err := m.SetResult("vpn", 7, ""); err != nil {
		t.Fatal(err)
	}
	got, _ := m.Get("vpn")
	if got.NodeCount != 7 || got.LastError != "" || got.LastUpdated == 0 {
		t.Fatalf("SetResult success not recorded: %+v", got)
	}
	prev := got.LastUpdated
	if err := m.SetResult("vpn", 0, "boom"); err != nil {
		t.Fatal(err)
	}
	got, _ = m.Get("vpn")
	if got.LastError != "boom" || got.NodeCount != 7 || got.LastUpdated != prev {
		t.Fatalf("SetResult error path wrong: %+v", got)
	}
}

func TestPersistReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subscriptions.toml")
	m := NewManager(path)
	if _, err := m.Add("Alpha", "https://a", 6); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Add("Beta", "https://b", 24); err != nil {
		t.Fatal(err)
	}
	if err := m.SetResult("alpha", 3, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("toml not written: %v", err)
	}
	m2 := NewManager(path)
	if err := m2.Load(); err != nil {
		t.Fatal(err)
	}
	if all := m2.List(); len(all) != 2 {
		t.Fatalf("reloaded %d, want 2", len(all))
	}
	if a, ok := m2.Get("alpha"); !ok || a.NodeCount != 3 || a.URL != "https://a" {
		t.Fatalf("reloaded alpha wrong: %+v", a)
	}
}
