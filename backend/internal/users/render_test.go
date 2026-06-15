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

func TestEffectiveRejectNames(t *testing.T) {
	const now = int64(1000)
	cases := []struct {
		name string
		list []PanelUser
		want []string
	}{
		{"nil list", nil, nil},
		{"empty list", []PanelUser{}, nil},
		{
			"all active -> empty",
			[]PanelUser{
				{Name: "a", Enabled: true},
				{Name: "b", Enabled: true, ExpiresAt: now + 1},
			},
			nil,
		},
		{
			"one disabled -> its names",
			[]PanelUser{
				{Name: "a", Enabled: true},
				{Name: "b", Enabled: false},
			},
			[]string{"b"},
		},
		{
			"one expired (past) -> its names",
			[]PanelUser{
				{Name: "a", Enabled: true},
				{Name: "b", Enabled: true, ExpiresAt: now - 1},
			},
			[]string{"b"},
		},
		{
			"boundary now==ExpiresAt -> expired, rejected",
			[]PanelUser{{Name: "edge", Enabled: true, ExpiresAt: now}},
			[]string{"edge"},
		},
		{
			"multi-binding name union from one inactive user",
			[]PanelUser{
				{Name: "u", Enabled: false, Bindings: []Binding{{Name: "u-vless"}, {Name: "u-naive"}}},
			},
			[]string{"u", "u-naive", "u-vless"}, // SORTED
		},
		{
			"dedup names across two inactive users sharing a name",
			[]PanelUser{
				{Name: "dup", Enabled: false},
				{Name: "dup", Enabled: false, Bindings: []Binding{{Name: "extra"}}},
			},
			[]string{"dup", "extra"},
		},
		{
			"blank names skipped",
			[]PanelUser{
				{Name: "", Enabled: false, Bindings: []Binding{{Name: ""}}},
				{Name: "real", Enabled: false},
			},
			[]string{"real"},
		},
		{
			"sorted output across multiple inactive users",
			[]PanelUser{
				{Name: "zoe", Enabled: false},
				{Name: "amy", Enabled: false},
				{Name: "mid", Enabled: false},
			},
			[]string{"amy", "mid", "zoe"},
		},
		{
			"mixed active/inactive -> only inactive names",
			[]PanelUser{
				{Name: "keep1", Enabled: true},
				{Name: "drop1", Enabled: false},
				{Name: "keep2", Enabled: true, ExpiresAt: now + 100},
				{Name: "drop2", Enabled: true, ExpiresAt: now - 100},
			},
			[]string{"drop1", "drop2"},
		},
		{
			"active user with same name as inactive user -> name still rejected (over-block, by-name)",
			[]PanelUser{
				{Name: "shared", Enabled: true},
				{Name: "shared", Enabled: false},
			},
			[]string{"shared"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EffectiveRejectNames(tc.list, now)
			if !equalStrings(got, tc.want) {
				t.Fatalf("EffectiveRejectNames = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestDuplicateNames(t *testing.T) {
	cases := []struct {
		name string
		list []PanelUser
		want []string
	}{
		{"none", []PanelUser{{ID: "1", Name: "a"}, {ID: "2", Name: "b"}}, nil},
		{"empty", nil, nil},
		{
			"one dup",
			[]PanelUser{
				{ID: "1", Name: "alice"},
				{ID: "2", Name: "bob"},
				{ID: "3", Name: "alice"}, // dup of #1
			},
			[]string{"alice"},
		},
		{
			"blanks ignored",
			[]PanelUser{
				{ID: "1", Name: ""},
				{ID: "2", Name: ""},
				{ID: "3", Name: "x"},
			},
			nil,
		},
		{
			"multiple dups, sorted",
			[]PanelUser{
				{ID: "1", Name: "zoe"},
				{ID: "2", Name: "zoe"},
				{ID: "3", Name: "amy"},
				{ID: "4", Name: "amy"},
				{ID: "5", Name: "solo"},
			},
			[]string{"amy", "zoe"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DuplicateNames(tc.list)
			if !equalStrings(got, tc.want) {
				t.Fatalf("DuplicateNames = %#v, want %#v", got, tc.want)
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
