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

func TestUserNames(t *testing.T) {
	cases := []struct {
		name string
		u    PanelUser
		want []string
	}{
		{"name only, no bindings", PanelUser{Name: "alice"}, []string{"alice"}},
		{"blank name only", PanelUser{Name: ""}, nil},
		{
			"name + distinct binding names",
			PanelUser{Name: "alice", Bindings: []Binding{{Name: "alice-vless"}, {Name: "alice-naive"}}},
			[]string{"alice", "alice-vless", "alice-naive"},
		},
		{
			"dedup own name vs binding name",
			PanelUser{Name: "bob", Bindings: []Binding{{Name: "bob"}, {Name: "bob2"}}},
			[]string{"bob", "bob2"},
		},
		{
			"blank binding names skipped",
			PanelUser{Name: "carol", Bindings: []Binding{{Name: ""}, {Name: "c2"}, {Name: ""}}},
			[]string{"carol", "c2"},
		},
		{
			"blank own name, binding names used",
			PanelUser{Name: "", Bindings: []Binding{{Name: "x"}, {Name: "y"}}},
			[]string{"x", "y"},
		},
		{
			"all blank",
			PanelUser{Name: "", Bindings: []Binding{{Name: ""}}},
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := userNames(tc.u)
			if !equalStrings(got, tc.want) {
				t.Fatalf("userNames(%+v) = %#v, want %#v", tc.u, got, tc.want)
			}
		})
	}
}

// equalStrings compares two string slices treating nil and empty as equal.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
