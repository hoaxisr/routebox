package config

import "testing"

// The panel exposes `optimistic` as one on/off box (#65), but sing-box marshals
// the field two ways: a bare bool, or {enabled, timeout} when a timeout is set.
// Reading must understand both, and writing must not throw away a hand-written
// timeout — the operator would lose it just by touching an unrelated DNS setting.
func TestDnsSettingsOptimistic(t *testing.T) {
	load := func(t *testing.T, dns string) *Manager {
		t.Helper()
		m, err := NewManager(writeV2Cfg(t, `{"outbounds": [{"type": "direct", "tag": "direct"}], "dns": `+dns+`}`))
		if err != nil {
			t.Fatalf("NewManager: %v", err)
		}
		return m
	}

	t.Run("reads the bool form", func(t *testing.T) {
		m := load(t, `{"optimistic": true}`)
		if got := m.GetDnsSettings()["optimistic"]; got != true {
			t.Fatalf("optimistic = %v, want true", got)
		}
	})

	t.Run("reads the object form as its enabled flag", func(t *testing.T) {
		m := load(t, `{"optimistic": {"enabled": true, "timeout": "10s"}}`)
		if got := m.GetDnsSettings()["optimistic"]; got != true {
			t.Fatalf("optimistic = %v, want true", got)
		}
	})

	t.Run("absent stays absent", func(t *testing.T) {
		m := load(t, `{}`)
		if _, ok := m.GetDnsSettings()["optimistic"]; ok {
			t.Fatal("optimistic reported for a config that has no such key")
		}
	})

	t.Run("turning it off drops the key", func(t *testing.T) {
		m := load(t, `{"optimistic": true}`)
		if err := m.UpdateDnsSettings(map[string]interface{}{"optimistic": false}); err != nil {
			t.Fatalf("UpdateDnsSettings: %v", err)
		}
		if _, ok := m.GetDnsSettings()["optimistic"]; ok {
			t.Fatal("optimistic survived being switched off")
		}
	})

	t.Run("the object form keeps its timeout", func(t *testing.T) {
		m := load(t, `{"optimistic": {"enabled": false, "timeout": "10s"}}`)
		if err := m.UpdateDnsSettings(map[string]interface{}{"optimistic": true}); err != nil {
			t.Fatalf("UpdateDnsSettings: %v", err)
		}
		obj, ok := m.getDns()["optimistic"].(map[string]interface{})
		if !ok {
			t.Fatalf("optimistic is no longer an object: %#v", m.getDns()["optimistic"])
		}
		if obj["enabled"] != true || obj["timeout"] != "10s" {
			t.Fatalf("optimistic = %#v, want enabled with the timeout intact", obj)
		}
	})
}
