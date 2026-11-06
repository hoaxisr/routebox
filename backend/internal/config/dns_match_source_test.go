package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func dnsRules(t *testing.T, cfg map[string]interface{}) []interface{} {
	t.Helper()
	return cfg["dns"].(map[string]interface{})["rules"].([]interface{})
}

func TestApplyDNSRuleSetMatchSource(t *testing.T) {
	cfg := parseJSON(t, `{"dns":{"rules":[
		{"rule_set":["zapret"],"server":"a"},
		{"domain_suffix":["x.y"],"server":"a","rule_set_ip_cidr_match_source":true},
		{"rule_set":["geo"],"match_response":true,"server":"a","rule_set_ip_cidr_match_source":true},
		{"rule_set":["geo"],"match_response":"resp","server":"a"},
		{"rule_set":"one","server":"a"},
		{"type":"logical","mode":"or","server":"a","rules":[{"rule_set":["zapret"]},{"domain":["x.y"]}]},
		{"type":"logical","mode":"or","match_response":true,"server":"a","rules":[{"rule_set":["geo"]}]}
	]}}`)
	if !applyDNSRuleSetMatchSource(cfg) {
		t.Fatal("nothing changed")
	}
	const key = "rule_set_ip_cidr_match_source"
	rules := dnsRules(t, cfg)
	rule := func(i int) map[string]interface{} { return rules[i].(map[string]interface{}) }
	nested := func(i, j int) map[string]interface{} {
		return rule(i)["rules"].([]interface{})[j].(map[string]interface{})
	}
	for name, want := range map[string]struct {
		r  map[string]interface{}
		on bool
	}{
		"plain rule_set":            {rule(0), true},
		"no rule_set, flag dropped": {rule(1), false},
		"match_response dropped":    {rule(2), false},
		"tagged match_response":     {rule(3), false},
		"single string":             {rule(4), true},
		"logical child":             {nested(5, 0), true},
		"logical child no rule_set": {nested(5, 1), false},
		"child of response logical": {nested(6, 0), false},
	} {
		if _, has := want.r[key]; has != want.on {
			t.Errorf("%s: flag present = %v", name, has)
		}
	}
	if applyDNSRuleSetMatchSource(cfg) {
		t.Error("not idempotent")
	}
}

func TestDraftAndLoadSetMatchSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	active := `{"outbounds":[{"type":"direct","tag":"direct"}],"dns":{"servers":[{"type":"local","tag":"l"}],"rules":[{"rule_set":["s"],"server":"l"}]}}`
	if err := os.WriteFile(path, []byte(active), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "rule_set_ip_cidr_match_source") {
		t.Errorf("Load left the file as it was: %s", b)
	}

	// Any draft write, whichever form or editor it came from.
	cfg := parseJSON(t, `{"dns":{"rules":[{"rule_set":["t"],"server":"l"}]}}`)
	if err := m.SetDraft(cfg); err != nil {
		t.Fatal(err)
	}
	got := dnsRules(t, m.Get())[0].(map[string]interface{})
	if got["rule_set_ip_cidr_match_source"] != true {
		t.Errorf("draft: %v", got)
	}
}

func TestValidateDNSRuleSets(t *testing.T) {
	sets := `"rule_set":[
		{"tag":"geoip-ru","type":"remote","url":"https://x/a.srs"},
		{"tag":"ru-nets","type":"remote","url":"https://x/geoip-ru.srs"},
		{"tag":"nets","type":"local","path":"/opt/geoip/ru.srs"},
		{"tag":"zapret","type":"remote","url":"https://x/antizapret.srs"}]`
	cases := []struct{ name, rule, want string }{
		{"by tag", `{"rule_set":["geoip-ru"],"server":"a"}`, `"geoip-ru"`},
		{"by url", `{"rule_set":["ru-nets"],"server":"a"}`, `"ru-nets"`},
		{"by path", `{"rule_set":["nets"],"server":"a"}`, `"nets"`},
		{"nested", `{"type":"logical","mode":"or","server":"a","rules":[{"rule_set":["geoip-ru"]}]}`, `"geoip-ru"`},
		{"with match_response is fine", `{"rule_set":["geoip-ru"],"match_response":true,"server":"a"}`, ""},
		{"mixed set is fine", `{"rule_set":["zapret"],"server":"a"}`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			errs := ValidateDNSRuleSets(parseJSON(t, `{"dns":{"rules":[`+c.rule+`]},"route":{`+sets+`}}`))
			if c.want == "" {
				if len(errs) != 0 {
					t.Fatalf("unexpected: %v", errs)
				}
				return
			}
			if len(errs) != 1 || !strings.Contains(errs[0], c.want) {
				t.Fatalf("want one error with %s, got %v", c.want, errs)
			}
		})
	}
}

// `check` passes all of this; only a real start shows the Fatal, so this runs
// the binary. A set with IP entries in a DNS rule, inside a logical rule too.
func TestMatchSourceKeepsSingBoxAlive(t *testing.T) {
	binary, err := exec.LookPath("amnezia-box")
	if err != nil {
		t.Skip("no amnezia-box binary on PATH")
	}
	for name, rule := range map[string]string{
		"plain":   `{"rule_set":["s"],"server":"l"}`,
		"logical": `{"type":"logical","mode":"or","server":"l","rules":[{"rule_set":["s"]},{"domain":["x.y"]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			cfg := parseJSON(t, `{"log":{"level":"error"},"dns":{"final":"l","servers":[{"type":"local","tag":"l"}],"rules":[`+rule+`]},
				"outbounds":[{"type":"direct","tag":"direct"}],
				"route":{"rule_set":[{"tag":"s","type":"inline","rules":[{"domain_suffix":["a.com"],"ip_cidr":["1.1.1.0/24"]}]}]}}`)
			applyDNSRuleSetMatchSource(cfg)
			data, err := encodeConfig(cfg)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "c.json")
			if err := os.WriteFile(path, data, 0644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(binary, "run", "-c", path)
			var stderr strings.Builder
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				t.Fatalf("exited: %v\n%s", err, stderr.String())
			case <-time.After(1500 * time.Millisecond):
				cmd.Process.Kill()
				<-done
			}
		})
	}
}
