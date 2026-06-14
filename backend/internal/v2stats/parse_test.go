package v2stats

import "testing"

func TestParseUserStat(t *testing.T) {
	cases := []struct {
		in       string
		wantName string
		wantUp   bool // true=uplink, false=downlink
		wantOK   bool
	}{
		{"user>>>alice>>>traffic>>>uplink", "alice", true, true},
		{"user>>>alice>>>traffic>>>downlink", "alice", false, true},
		{"user>>>bob smith>>>traffic>>>uplink", "bob smith", true, true},
		{"inbound>>>tun-in>>>traffic>>>uplink", "", false, false}, // not a user stat
		{"outbound>>>direct>>>traffic>>>downlink", "", false, false},
		{"user>>>alice>>>traffic>>>sideways", "", false, false}, // unknown direction
		{"user>>>alice>>>traffic", "", false, false},            // too few fields
		{"", "", false, false},
		{"garbage", "", false, false},
	}
	for _, c := range cases {
		name, up, ok := parseUserStat(c.in)
		if ok != c.wantOK || name != c.wantName || (ok && up != c.wantUp) {
			t.Errorf("parseUserStat(%q) = (%q,%v,%v), want (%q,%v,%v)",
				c.in, name, up, ok, c.wantName, c.wantUp, c.wantOK)
		}
	}
}
