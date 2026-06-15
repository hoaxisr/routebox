package config

import (
	"reflect"
	"testing"
)

func TestBuildRejectRule(t *testing.T) {
	t.Run("empty names -> nil", func(t *testing.T) {
		if got := buildRejectRule(nil); got != nil {
			t.Fatalf("nil names -> %#v, want nil", got)
		}
		if got := buildRejectRule([]string{}); got != nil {
			t.Fatalf("empty names -> %#v, want nil", got)
		}
	})
	t.Run("names -> reject rule shape", func(t *testing.T) {
		got := buildRejectRule([]string{"alice", "bob"})
		want := map[string]interface{}{
			"auth_user": []interface{}{"alice", "bob"},
			"action":    "reject",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v\nwant %#v", got, want)
		}
		if len(got) != 2 {
			t.Fatalf("rule has %d keys, want exactly 2 (auth_user, action)", len(got))
		}
	})
	t.Run("single name", func(t *testing.T) {
		got := buildRejectRule([]string{"solo"})
		want := map[string]interface{}{
			"auth_user": []interface{}{"solo"},
			"action":    "reject",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v\nwant %#v", got, want)
		}
	})
}

func TestManagedRejectRule(t *testing.T) {
	cases := []struct {
		name string
		rule map[string]interface{}
		want bool
	}{
		{
			"managed: auth_user + action reject, no extra keys",
			map[string]interface{}{"auth_user": []interface{}{"a"}, "action": "reject"},
			true,
		},
		{
			"managed: multiple names",
			map[string]interface{}{"auth_user": []interface{}{"a", "b"}, "action": "reject"},
			true,
		},
		{
			"NOT managed: empty auth_user list",
			map[string]interface{}{"auth_user": []interface{}{}, "action": "reject"},
			false,
		},
		{
			"NOT managed: missing auth_user",
			map[string]interface{}{"action": "reject"},
			false,
		},
		{
			"NOT managed: action not reject",
			map[string]interface{}{"auth_user": []interface{}{"a"}, "action": "route"},
			false,
		},
		{
			"NOT managed: missing action",
			map[string]interface{}{"auth_user": []interface{}{"a"}},
			false,
		},
		{
			"NOT managed: extra match key (user-authored auth_user rule)",
			map[string]interface{}{"auth_user": []interface{}{"a"}, "action": "reject", "domain": []interface{}{"x.com"}},
			false,
		},
		{
			"NOT managed: outbound action with auth_user (user rule)",
			map[string]interface{}{"auth_user": []interface{}{"a"}, "outbound": "block"},
			false,
		},
		{
			"NOT managed: empty rule",
			map[string]interface{}{},
			false,
		},
		{
			"NOT managed: nil rule",
			nil,
			false,
		},
		{
			"NOT managed: auth_user wrong type",
			map[string]interface{}{"auth_user": "a", "action": "reject"},
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := managedRejectRule(tc.rule); got != tc.want {
				t.Fatalf("managedRejectRule(%#v) = %v, want %v", tc.rule, got, tc.want)
			}
		})
	}
}
