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

	t.Run("turning it on from nothing writes the bool form", func(t *testing.T) {
		m := load(t, `{}`)
		if err := m.UpdateDnsSettings(map[string]interface{}{"optimistic": true}); err != nil {
			t.Fatalf("UpdateDnsSettings: %v", err)
		}
		if got := m.getDns()["optimistic"]; got != true {
			t.Fatalf("optimistic = %#v, want true", got)
		}
	})

	// The fork REFUSES TO START on this combination ("`optimistic` is conflict with
	// `disable_cache`"), so the panel must not be able to write it — greying the box
	// out only covers one of the two orders the operator can click them in.
	t.Run("never coexists with disable_cache or disable_expire", func(t *testing.T) {
		for _, off := range []string{"disable_cache", "disable_expire"} {
			m := load(t, `{"optimistic": true}`)
			if err := m.UpdateDnsSettings(map[string]interface{}{off: true}); err != nil {
				t.Fatalf("UpdateDnsSettings: %v", err)
			}
			if got, ok := m.getDns()["optimistic"]; ok {
				t.Fatalf("%s is on but optimistic survived as %#v — the box will not start", off, got)
			}
		}
	})

	t.Run("the conflict clears the flag but keeps a hand-written timeout", func(t *testing.T) {
		m := load(t, `{"optimistic": {"enabled": true, "timeout": "10s"}}`)
		if err := m.UpdateDnsSettings(map[string]interface{}{"disable_cache": true}); err != nil {
			t.Fatalf("UpdateDnsSettings: %v", err)
		}
		obj, ok := m.getDns()["optimistic"].(map[string]interface{})
		if !ok {
			t.Fatalf("optimistic = %#v", m.getDns()["optimistic"])
		}
		if enabled, set := obj["enabled"]; set && enabled == true {
			t.Fatalf("optimistic still enabled alongside disable_cache: %#v", obj)
		}
		if obj["timeout"] != "10s" {
			t.Fatalf("the timeout was thrown away: %#v", obj)
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
