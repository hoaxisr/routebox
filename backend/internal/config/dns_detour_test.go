package config

import "testing"

// DNS server detour must accept endpoint tags (AWG/WG endpoints act as
// outbounds in sing-box), same as route rules already do. Issue #7.
func TestDnsServerDetour_AcceptsEndpointTag(t *testing.T) {
	p := writeV2Cfg(t, `{
		"outbounds": [{"type": "direct", "tag": "direct"}],
		"endpoints": [{"type": "amneziawg", "tag": "awg-out"}],
		"dns": {"servers": [{"type": "udp", "tag": "existing", "server": "1.1.1.1"}]}
	}`)
	m, err := NewManager(p)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	t.Run("create with endpoint detour", func(t *testing.T) {
		err := m.CreateDnsServer(map[string]interface{}{
			"type": "udp", "tag": "dns-via-awg", "server": "1.1.1.1", "detour": "awg-out",
		})
		if err != nil {
			t.Fatalf("CreateDnsServer with endpoint detour: %v", err)
		}
	})

	t.Run("update with endpoint detour", func(t *testing.T) {
		err := m.UpdateDnsServer("existing", map[string]interface{}{
			"type": "udp", "tag": "existing", "server": "1.1.1.1", "detour": "awg-out",
		})
		if err != nil {
			t.Fatalf("UpdateDnsServer with endpoint detour: %v", err)
		}
	})

	t.Run("nonexistent detour still rejected", func(t *testing.T) {
		err := m.CreateDnsServer(map[string]interface{}{
			"type": "udp", "tag": "dns-bad", "server": "1.1.1.1", "detour": "no-such-tag",
		})
		if err == nil {
			t.Fatal("CreateDnsServer with unknown detour: want error, got nil")
		}
	})
}
