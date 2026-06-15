package users

import "testing"

func TestIsEffectivelyActive(t *testing.T) {
	const now = int64(1000)
	cases := []struct {
		name    string
		enabled bool
		expires int64
		want    bool
	}{
		{"enabled, never-expires (0)", true, 0, true},
		{"enabled, expires in future", true, now + 1, true},
		{"enabled, expires in past", true, now - 1, false},
		{"enabled, boundary now==ExpiresAt (expired)", true, now, false}, // now < ExpiresAt is FALSE at equality
		{"disabled, never-expires (0)", false, 0, false},
		{"disabled, expires in future", false, now + 1, false},
		{"disabled, expires in past", false, now - 1, false},
		{"disabled, boundary now==ExpiresAt", false, now, false},
		{"enabled, far-future expiry", true, now + 1_000_000, true},
		{"enabled, expiry one second future (boundary+1)", true, now + 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := PanelUser{Enabled: tc.enabled, ExpiresAt: tc.expires}
			if got := IsEffectivelyActive(u, now); got != tc.want {
				t.Fatalf("IsEffectivelyActive(enabled=%v expires=%d, now=%d) = %v, want %v",
					tc.enabled, tc.expires, now, got, tc.want)
			}
		})
	}
}
