package config

import (
	"reflect"
	"testing"
)

func fallbackManager(t *testing.T, dns string) *Manager {
	t.Helper()
	m, err := NewManager(writeV2Cfg(t, `{"outbounds": [{"type": "direct", "tag": "direct"}], "dns": `+dns+`}`))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m
}

const twoServers = `"servers": [{"tag": "primary", "type": "udp", "server": "1.1.1.1"}, {"tag": "backup", "type": "udp", "server": "8.8.8.8"}]`

// The round trip a panel user makes: switch it on, read it back, switch it off.
func TestDnsFallbackRoundTrip(t *testing.T) {
	m := fallbackManager(t, `{`+twoServers+`, "rules": [{"domain": ["example.com"], "server": "primary"}]}`)

	err := m.UpdateDnsSettings(map[string]interface{}{"fallback": map[string]interface{}{
		"enabled": true, "primary": "primary", "fallback": "backup",
		"rcodes": []interface{}{"NXDOMAIN", "SERVFAIL"},
	}})
	if err != nil {
		t.Fatalf("UpdateDnsSettings: %v", err)
	}

	rules := m.getDnsArray("rules")
	if len(rules) != 5 {
		t.Fatalf("want the operator's rule plus a 4-rule tail, got %d: %#v", len(rules), rules)
	}
	if first := rules[0].(map[string]interface{}); first["domain"] == nil {
		t.Fatalf("the operator's own rule must stay first, got %#v", first)
	}

	got := m.GetDnsSettings()["fallback"].(map[string]interface{})
	want := map[string]interface{}{
		"enabled": true, "primary": "primary", "fallback": "backup",
		"rcodes": []interface{}{"NXDOMAIN", "SERVFAIL"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("read back %#v, want %#v", got, want)
	}

	if err := m.UpdateDnsSettings(map[string]interface{}{"fallback": map[string]interface{}{"enabled": false}}); err != nil {
		t.Fatalf("UpdateDnsSettings (off): %v", err)
	}
	if rules := m.getDnsArray("rules"); len(rules) != 1 {
		t.Fatalf("switching it off must leave only the operator's rule, got %#v", rules)
	}
}

// Re-saving must rewrite the tail, not stack a second copy on top of the first.
func TestDnsFallbackRewritesInPlace(t *testing.T) {
	m := fallbackManager(t, `{`+twoServers+`}`)
	on := func(t *testing.T, codes ...interface{}) {
		t.Helper()
		err := m.UpdateDnsSettings(map[string]interface{}{"fallback": map[string]interface{}{
			"enabled": true, "primary": "primary", "fallback": "backup", "rcodes": codes,
		}})
		if err != nil {
			t.Fatalf("UpdateDnsSettings: %v", err)
		}
	}
	on(t, "NXDOMAIN", "SERVFAIL")
	on(t, "REFUSED")

	if rules := m.getDnsArray("rules"); len(rules) != 3 {
		t.Fatalf("want evaluate + one route + respond, got %d: %#v", len(rules), rules)
	}
	got := m.GetDnsSettings()["fallback"].(map[string]interface{})
	if !reflect.DeepEqual(got["rcodes"], []interface{}{"REFUSED"}) {
		t.Fatalf("rcodes = %#v, want just REFUSED", got["rcodes"])
	}
}

func TestDnsFallbackRejectsBadInput(t *testing.T) {
	cases := map[string]map[string]interface{}{
		"unknown server":  {"enabled": true, "primary": "primary", "fallback": "nope", "rcodes": []interface{}{"NXDOMAIN"}},
		"same server":     {"enabled": true, "primary": "primary", "fallback": "primary", "rcodes": []interface{}{"NXDOMAIN"}},
		"no rcodes":       {"enabled": true, "primary": "primary", "fallback": "backup", "rcodes": []interface{}{}},
		"unknown rcode":   {"enabled": true, "primary": "primary", "fallback": "backup", "rcodes": []interface{}{"WHATEVER"}},
		"missing primary": {"enabled": true, "fallback": "backup", "rcodes": []interface{}{"NXDOMAIN"}},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			m := fallbackManager(t, `{`+twoServers+`}`)
			if err := m.UpdateDnsSettings(map[string]interface{}{"fallback": in}); err == nil {
				t.Fatal("want an error, got nil")
			}
			if rules := m.getDnsArray("rules"); len(rules) != 0 {
				t.Fatalf("a rejected payload must write nothing, got %#v", rules)
			}
		})
	}
}

// A new rule appended after the terminal `respond` would never run, so it has to
// land in front of the generated tail.
func TestCreateDnsRuleLandsBeforeFallback(t *testing.T) {
	m := fallbackManager(t, `{`+twoServers+`}`)
	err := m.UpdateDnsSettings(map[string]interface{}{"fallback": map[string]interface{}{
		"enabled": true, "primary": "primary", "fallback": "backup", "rcodes": []interface{}{"NXDOMAIN"},
	}})
	if err != nil {
		t.Fatalf("UpdateDnsSettings: %v", err)
	}

	if err := m.CreateDnsRule(map[string]interface{}{"domain": []interface{}{"example.com"}, "server": "primary"}); err != nil {
		t.Fatalf("CreateDnsRule: %v", err)
	}

	rules := m.getDnsArray("rules")
	if first := rules[0].(map[string]interface{}); first["domain"] == nil {
		t.Fatalf("the new rule must precede the fallback tail, got %#v", rules)
	}
	if start, _, _, _ := findFallbackBlock(rules); start != 1 {
		t.Fatalf("fallback tail starts at %d, want 1: %#v", start, rules)
	}
}

// Rules that merely look similar must not be mistaken for the generated tail.
func TestFindFallbackBlockIgnoresLookalikes(t *testing.T) {
	rule := func(kv ...interface{}) interface{} {
		r := map[string]interface{}{}
		for i := 0; i < len(kv); i += 2 {
			r[kv[i].(string)] = kv[i+1]
		}
		return r
	}
	respond := rule("match_response", true, "action", "respond")
	route := rule("match_response", true, "response_rcode", "NXDOMAIN", "action", "route", "server", "backup")

	cases := map[string][]interface{}{
		"respond alone":     {respond},
		"no evaluate ahead": {route, respond},
		"evaluate missing a server": {
			rule("action", "evaluate"), route, respond,
		},
		"route rules split across two servers": {
			rule("action", "evaluate", "server", "primary"),
			route,
			rule("match_response", true, "response_rcode", "SERVFAIL", "action", "route", "server", "other"),
			respond,
		},
		"plain rules": {rule("domain", []interface{}{"example.com"}, "server", "primary")},
	}
	for name, rules := range cases {
		t.Run(name, func(t *testing.T) {
			if start, _, _, _ := findFallbackBlock(rules); start >= 0 {
				t.Fatalf("claimed a fallback block at %d in %#v", start, rules)
			}
		})
	}
}
