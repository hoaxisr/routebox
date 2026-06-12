package settings

import (
	"encoding/json"
	"testing"
)

func TestUpdateNumericSettings(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		value   interface{}
		wantErr bool
		want    int
		get     func(s Settings) int
	}{
		{"float64 accepted (JSON decode)", "monitoring.poll_interval_ms", float64(1500), false, 1500,
			func(s Settings) int { return s.Monitoring.PollIntervalMs }},
		{"int accepted", "monitoring.max_closed_connections", 250, false, 250,
			func(s Settings) int { return s.Monitoring.MaxClosedConnections }},
		{"json.Number accepted", "advanced.ws_ping_interval_sec", json.Number("45"), false, 45,
			func(s Settings) int { return s.Advanced.WsPingIntervalSec }},
		{"float64 accepted", "advanced.ws_pong_timeout_sec", float64(15), false, 15,
			func(s Settings) int { return s.Advanced.WsPongTimeoutSec }},
		{"fractional float rejected", "monitoring.poll_interval_ms", float64(1.5), true, 0, nil},
		{"string rejected", "monitoring.proxies_refresh_ms", "fast", true, 0, nil},
		{"bool rejected for numeric", "security.session_timeout_minutes", true, true, 0, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &Manager{settings: Default()}
			err := m.Update(map[string]interface{}{tc.key: tc.value})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s=%v, got nil", tc.key, tc.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := tc.get(m.Get()); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestUpdateBoolFieldsUnaffected(t *testing.T) {
	m := &Manager{settings: Default()}
	if err := m.Update(map[string]interface{}{"geoip.enabled": false}); err != nil {
		t.Fatalf("bool update failed: %v", err)
	}
	if m.Get().GeoIP.Enabled {
		t.Fatal("geoip.enabled not applied")
	}
}
