package config

import (
	"os"
	"path/filepath"
	"testing"
)

func newSubMergeManager(t *testing.T) *Manager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"outbounds":[{"tag":"direct","type":"direct"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func outboundTags(m *Manager) map[string]bool {
	tags := map[string]bool{}
	for _, ob := range m.ListOutbounds() {
		if t, ok := ob["tag"].(string); ok {
			tags[t] = true
		}
	}
	return tags
}

func TestReplaceSubscriptionOutbounds(t *testing.T) {
	m := newSubMergeManager(t)
	prefix := "Home · "
	nodes := []map[string]interface{}{{"tag": "Home · A", "type": "shadowsocks"}, {"tag": "Home · B", "type": "vless"}}
	group := map[string]interface{}{"type": "urltest", "tag": "Home", "outbounds": []interface{}{"Home · A", "Home · B"}}
	if err := m.ReplaceSubscriptionOutbounds("Home", prefix, nodes, group); err != nil {
		t.Fatal(err)
	}
	tags := outboundTags(m)
	for _, want := range []string{"direct", "Home", "Home · A", "Home · B"} {
		if !tags[want] {
			t.Fatalf("missing outbound %q; got %v", want, tags)
		}
	}
	if g, ok := m.GetOutbound("Home"); !ok || g["type"] != "urltest" {
		t.Fatalf("group shape wrong: %+v ok=%v", g, ok)
	}
	if !m.HasDraft() {
		t.Fatal("replace must create a draft")
	}
	nodes2 := []map[string]interface{}{{"tag": "Home · C", "type": "vless"}}
	group2 := map[string]interface{}{"type": "urltest", "tag": "Home", "outbounds": []interface{}{"Home · C"}}
	if err := m.ReplaceSubscriptionOutbounds("Home", prefix, nodes2, group2); err != nil {
		t.Fatal(err)
	}
	tags = outboundTags(m)
	if tags["Home · A"] || tags["Home · B"] {
		t.Fatalf("stale nodes survived: %v", tags)
	}
	if !tags["Home · C"] || !tags["Home"] || !tags["direct"] {
		t.Fatalf("expected C+group+direct: %v", tags)
	}
}

func TestRemoveSubscriptionOutbounds(t *testing.T) {
	m := newSubMergeManager(t)
	prefix := "Home · "
	nodes := []map[string]interface{}{{"tag": "Home · A", "type": "vless"}}
	group := map[string]interface{}{"type": "urltest", "tag": "Home", "outbounds": []interface{}{"Home · A"}}
	if err := m.ReplaceSubscriptionOutbounds("Home", prefix, nodes, group); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveSubscriptionOutbounds("Home", prefix); err != nil {
		t.Fatal(err)
	}
	tags := outboundTags(m)
	if tags["Home"] || tags["Home · A"] {
		t.Fatalf("not removed: %v", tags)
	}
	if !tags["direct"] {
		t.Fatal("foreign outbound must survive")
	}
}
