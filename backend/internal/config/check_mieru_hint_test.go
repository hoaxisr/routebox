package config

import (
	"strings"
	"testing"
)

func TestMieruUnsupportedHint(t *testing.T) {
	t.Run("appends hint on unknown outbound type mieru", func(t *testing.T) {
		in := []string{"FATAL[0000] decode config: outbounds[2]: unknown outbound type: mieru"}
		out := mieruUnsupportedHint(in)
		if len(out) != 2 {
			t.Fatalf("expected hint appended, got %v", out)
		}
		if !strings.Contains(out[1], "does not support mieru") {
			t.Fatalf("hint missing: %v", out)
		}
	})
	t.Run("appends hint on unknown inbound type mieru", func(t *testing.T) {
		in := []string{"decode config: inbounds[0]: unknown inbound type: mieru"}
		out := mieruUnsupportedHint(in)
		if len(out) != 2 || !strings.Contains(out[1], "does not support mieru") {
			t.Fatalf("expected inbound-variant hint, got %v", out)
		}
	})
	t.Run("passes unrelated errors through untouched", func(t *testing.T) {
		in := []string{"decode config: outbounds[1]: missing server address"}
		out := mieruUnsupportedHint(in)
		if len(out) != 1 || out[0] != in[0] {
			t.Fatalf("unrelated errors mutated: %v", out)
		}
	})
	t.Run("nil in, nil out", func(t *testing.T) {
		if out := mieruUnsupportedHint(nil); out != nil {
			t.Fatalf("expected nil, got %v", out)
		}
	})
	t.Run("hint appended once even with multiple mieru errors", func(t *testing.T) {
		in := []string{
			"outbounds[1]: unknown outbound type: mieru",
			"outbounds[2]: unknown outbound type: mieru",
		}
		out := mieruUnsupportedHint(in)
		if len(out) != 3 {
			t.Fatalf("expected exactly one hint, got %v", out)
		}
	})
}
