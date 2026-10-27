package config

import (
	"strings"
	"testing"
)

// Group errors amnezia-box raises only on start (check passes them, reload
// dies): a missing member, a cycle, interval > idle_timeout.
func TestValidateGroups(t *testing.T) {
	cases := []struct {
		name, cfg, want string
	}{
		{"ok", `{"outbounds":[{"type":"direct","tag":"d"},{"type":"fallback","tag":"fb","outbounds":["d"],"interval":"1m"}]}`, ""},
		{"endpoint member ok", `{"endpoints":[{"type":"wireguard","tag":"wg"}],"outbounds":[{"type":"selector","tag":"s","outbounds":["wg"]}]}`, ""},
		{"missing member", `{"outbounds":[{"type":"selector","tag":"s","outbounds":["gone"]}]}`, `member "gone" does not exist`},
		{"cycle", `{"outbounds":[{"type":"selector","tag":"a","outbounds":["b"]},{"type":"fallback","tag":"b","outbounds":["a"]}]}`, "cycle: a -> b -> a"},
		{"interval over default idle", `{"outbounds":[{"type":"direct","tag":"d"},{"type":"fallback","tag":"fb","outbounds":["d"],"interval":"1h"}]}`, "longer than idle_timeout"},
		{"interval in days", `{"outbounds":[{"type":"direct","tag":"d"},{"type":"urltest","tag":"u","outbounds":["d"],"interval":"1d","idle_timeout":"2h"}]}`, "longer than idle_timeout"},
		{"interval under explicit idle", `{"outbounds":[{"type":"direct","tag":"d"},{"type":"urltest","tag":"u","outbounds":["d"],"interval":"1h","idle_timeout":"2h"}]}`, ""},
		{"selector has no interval", `{"outbounds":[{"type":"direct","tag":"d"},{"type":"selector","tag":"s","outbounds":["d"],"interval":"5h"}]}`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			errs := ValidateGroups(parseJSON(t, c.cfg))
			got := strings.Join(errs, "; ")
			if c.want == "" && got != "" {
				t.Fatalf("unexpected: %s", got)
			}
			if c.want != "" && !strings.Contains(got, c.want) {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestFallbackUnsupportedHint(t *testing.T) {
	out := explainCheckErrors([]string{"decode config at x: outbounds[1]: unknown outbound type: fallback"})
	if len(out) != 2 || !strings.Contains(out[1], "awgm.35") {
		t.Fatalf("got %v", out)
	}
	if out := explainCheckErrors([]string{"something else"}); len(out) != 1 {
		t.Fatalf("hint on unrelated error: %v", out)
	}
}
