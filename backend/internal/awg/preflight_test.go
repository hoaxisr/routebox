package awg

import "testing"

func TestEgressProbeOK(t *testing.T) {
	cases := []struct {
		name   string
		local  bool
		dialOK map[string]bool
		want   bool
	}{
		{"no local v6", false, map[string]bool{}, false},
		{"local ok, both targets fail", true, map[string]bool{}, false},
		{"local ok, first blocked second ok", true, map[string]bool{ipv6PreflightTargets[1]: true}, true},
		{"local ok, first ok", true, map[string]bool{ipv6PreflightTargets[0]: true}, true},
	}
	for _, tc := range cases {
		p := egressProbe{
			hasGlobalV6: func() bool { return tc.local },
			dial:        func(target string) bool { return tc.dialOK[target] },
		}
		if got := p.ok(); got != tc.want {
			t.Errorf("%s: ok()=%v want %v", tc.name, got, tc.want)
		}
	}
}
