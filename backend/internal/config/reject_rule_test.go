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
