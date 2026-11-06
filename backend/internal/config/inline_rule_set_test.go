package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeDomainSuffix(t *testing.T) {
	cases := map[string]string{
		"youtube.com":                 "youtube.com",
		" YouTube.COM ":               "youtube.com",
		"*.youtube.com":               "youtube.com",
		"https://www.youtube.com/w?v": "www.youtube.com",
		"youtube.com/":                "youtube.com",
		"youtube.com:443":             "youtube.com",
		"youtube.com.":                "youtube.com",
		".youtube.com":                ".youtube.com", // sing-box: subdomains only, deliberate
		"кремль.рф":                   "xn--e1ajeds9e.xn--p1ai",
		"https://":                    "",
	}
	for in, want := range cases {
		if got := NormalizeDomainSuffix(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeInlineRuleSets(t *testing.T) {
	cfg := parseJSON(t, `{"route":{"rule_set":[
		{"tag":"a","type":"inline","rules":[
			{"domain_suffix":["*.YouTube.com","youtube.com","https://ggpht.com/"]},
			{"type":"logical","mode":"or","rules":[{"domain_suffix":"X.com"}]}
		]},
		{"tag":"r","type":"remote","url":"https://x/y.srs","rules":[{"domain_suffix":["*.Keep.me"]}]}
	]}}`)
	if notes := NormalizeInlineRuleSets(cfg); len(notes) != 1 {
		t.Fatalf("notes: %v", notes)
	}
	sets := ruleSets(cfg)
	rules := sets[0]["rules"].([]interface{})
	got := rules[0].(map[string]interface{})["domain_suffix"]
	if want := []interface{}{"youtube.com", "ggpht.com"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
	nested := rules[1].(map[string]interface{})["rules"].([]interface{})[0].(map[string]interface{})
	if want := []interface{}{"x.com"}; !reflect.DeepEqual(nested["domain_suffix"], want) {
		t.Errorf("nested: %v", nested["domain_suffix"])
	}
	if got := sets[1]["rules"].([]interface{})[0].(map[string]interface{})["domain_suffix"]; !reflect.DeepEqual(got, []interface{}{"*.Keep.me"}) {
		t.Errorf("non-inline touched: %v", got)
	}
	if notes := NormalizeInlineRuleSets(cfg); len(notes) != 0 {
		t.Errorf("not idempotent: %v", notes)
	}
}

func TestLoadNormalizesInlineRuleSetsOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	active := `{"outbounds":[{"type":"direct","tag":"direct"}],"route":{"rule_set":[{"tag":"yt","type":"inline","rules":[{"domain_suffix":["*.youtube.com"]}]}]}}`
	if err := os.WriteFile(path, []byte(active), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(path); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "*.") || !strings.Contains(string(b), `"youtube.com"`) {
		t.Errorf("on disk: %s", b)
	}
}

func TestCreateAndUpdateRuleSetNormalizeInlineDomains(t *testing.T) {
	m := newTestManager(t)
	rs := func(d string) map[string]interface{} {
		return map[string]interface{}{"tag": "yt", "type": "inline", "rules": []interface{}{
			map[string]interface{}{"domain_suffix": []interface{}{d}},
		}}
	}
	first := func() interface{} {
		return m.ListRuleSets()[0]["rules"].([]interface{})[0].(map[string]interface{})["domain_suffix"]
	}
	if err := m.CreateRuleSet(rs("*.YouTube.com")); err != nil {
		t.Fatal(err)
	}
	if got := first(); !reflect.DeepEqual(got, []interface{}{"youtube.com"}) {
		t.Errorf("create: %v", got)
	}
	if err := m.UpdateRuleSet("yt", rs("https://ggpht.com/")); err != nil {
		t.Fatal(err)
	}
	if got := first(); !reflect.DeepEqual(got, []interface{}{"ggpht.com"}) {
		t.Errorf("update: %v", got)
	}
}

func TestValidateInlineRuleSets(t *testing.T) {
	cases := []struct{ name, set, want string }{
		{"valid", `{"tag":"yt","type":"inline","rules":[{"domain_suffix":["youtube.com"]}]}`, ""},
		{"remote is not ours", `{"tag":"yt","type":"remote","url":"https://x/y.srs"}`, ""},
		{"logical", `{"tag":"yt","type":"inline","rules":[{"type":"logical","mode":"or","rules":[{"domain":["a.b"]}]}]}`, ""},
		{"no rules", `{"tag":"yt","type":"inline","rules":[]}`, `"yt"`},
		{"empty rule", `{"tag":"yt","type":"inline","rules":[{}]}`, `"yt"`},
		{"empty list", `{"tag":"yt","type":"inline","rules":[{"domain_suffix":[]}]}`, `"yt"`},
		{"invert only", `{"tag":"yt","type":"inline","rules":[{"invert":true}]}`, `"yt"`},
		{"unmatchable", `{"tag":"yt","type":"inline","rules":[{"domain_suffix":["*youtube*"]}]}`, `*youtube*`},
		{"unmatchable nested", `{"tag":"yt","type":"inline","rules":[{"type":"logical","mode":"or","rules":[{"domain_suffix":["a b"]}]}]}`, `a b`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			errs := ValidateInlineRuleSets(parseJSON(t, `{"route":{"rule_set":[`+c.set+`]}}`))
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
