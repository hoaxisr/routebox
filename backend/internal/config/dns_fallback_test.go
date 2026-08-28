package config

import (
	"reflect"
	"strings"
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
		"enabled": true, "primary": "primary", "fallbacks": []interface{}{"backup"},
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
		"enabled": true, "primary": "primary", "fallbacks": []interface{}{"backup"},
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

// The tail is a wire contract with the fork, not an internal shape: these exact
// keys, one STRING response_rcode per rule (an array is rejected), evaluate first,
// terminal respond last. Reading it back through our own parser proves nothing
// about that, so pin the JSON.
func TestDnsFallbackEmitsExactRuleShape(t *testing.T) {
	m := fallbackManager(t, `{`+twoServers+`}`)
	err := m.UpdateDnsSettings(map[string]interface{}{"fallback": map[string]interface{}{
		"enabled": true, "primary": "primary", "fallbacks": []interface{}{"backup"},
		"rcodes": []interface{}{"NXDOMAIN", "SERVFAIL"},
	}})
	if err != nil {
		t.Fatalf("UpdateDnsSettings: %v", err)
	}
	want := []interface{}{
		map[string]interface{}{"action": "evaluate", "server": "primary"},
		map[string]interface{}{"match_response": true, "response_rcode": "NXDOMAIN", "action": "route", "server": "backup"},
		map[string]interface{}{"match_response": true, "response_rcode": "SERVFAIL", "action": "route", "server": "backup"},
		map[string]interface{}{"match_response": true, "action": "respond"},
	}
	if got := m.getDnsArray("rules"); !reflect.DeepEqual(got, want) {
		t.Fatalf("tail = %#v\nwant %#v", got, want)
	}
}

const threeServers = `"servers": [{"tag": "primary", "type": "udp", "server": "1.1.1.1"}, {"tag": "backup", "type": "udp", "server": "8.8.8.8"}, {"tag": "last", "type": "udp", "server": "9.9.9.9"}]`

// #70: a chain longer than one hop. Every fallback but the last is an `evaluate`,
// which replaces the response the rules below it match on, so the chain stops at
// the first server that answers; only the last is a terminal `route`. Verified
// against the real binary with three local resolvers — this pins the shape that
// behaviour depends on.
func TestDnsFallbackChainEmitsExactRuleShape(t *testing.T) {
	m := fallbackManager(t, `{`+threeServers+`}`)
	err := m.UpdateDnsSettings(map[string]interface{}{"fallback": map[string]interface{}{
		"enabled": true, "primary": "primary", "fallbacks": []interface{}{"backup", "last"},
		"rcodes": []interface{}{"NXDOMAIN", "SERVFAIL"},
	}})
	if err != nil {
		t.Fatalf("UpdateDnsSettings: %v", err)
	}
	want := []interface{}{
		map[string]interface{}{"action": "evaluate", "server": "primary"},
		map[string]interface{}{"match_response": true, "response_rcode": "NXDOMAIN", "action": "evaluate", "server": "backup"},
		map[string]interface{}{"match_response": true, "response_rcode": "SERVFAIL", "action": "evaluate", "server": "backup"},
		map[string]interface{}{"match_response": true, "response_rcode": "NXDOMAIN", "action": "route", "server": "last"},
		map[string]interface{}{"match_response": true, "response_rcode": "SERVFAIL", "action": "route", "server": "last"},
		map[string]interface{}{"match_response": true, "action": "respond"},
	}
	if got := m.getDnsArray("rules"); !reflect.DeepEqual(got, want) {
		t.Fatalf("tail = %#v\nwant %#v", got, want)
	}

	got := m.GetDnsSettings()["fallback"].(map[string]interface{})
	wantRead := map[string]interface{}{
		"enabled": true, "primary": "primary", "fallbacks": []interface{}{"backup", "last"},
		"rcodes": []interface{}{"NXDOMAIN", "SERVFAIL"},
	}
	if !reflect.DeepEqual(got, wantRead) {
		t.Fatalf("read back %#v, want %#v", got, wantRead)
	}
}

// Every block written before #70 has a single terminal route hop and no evaluate
// hops. It must keep reading back — as a one-element chain — or the first save
// after an upgrade would silently drop the operator's fallback.
func TestDnsFallbackReadsPreChainBlock(t *testing.T) {
	m := fallbackManager(t, `{`+twoServers+`, "rules": [
		{"action": "evaluate", "server": "primary"},
		{"match_response": true, "response_rcode": "SERVFAIL", "action": "route", "server": "backup"},
		{"match_response": true, "action": "respond"}]}`)
	got := m.GetDnsSettings()["fallback"].(map[string]interface{})
	want := map[string]interface{}{
		"enabled": true, "primary": "primary", "fallbacks": []interface{}{"backup"},
		"rcodes": []interface{}{"SERVFAIL"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("read back %#v, want %#v", got, want)
	}
}

// A server twice in the chain asks it twice in a row for nothing, and a fallback
// equal to the primary re-asks the server that just failed. Both are useless
// rather than broken, which is the kind of thing nobody notices.
func TestDnsFallbackRejectsRepeatedServer(t *testing.T) {
	for name, chain := range map[string][]interface{}{
		"a fallback repeated":         {"backup", "backup"},
		"a fallback equal to primary": {"backup", "primary"},
	} {
		t.Run(name, func(t *testing.T) {
			m := fallbackManager(t, `{`+threeServers+`}`)
			err := m.UpdateDnsSettings(map[string]interface{}{"fallback": map[string]interface{}{
				"enabled": true, "primary": "primary", "fallbacks": chain,
				"rcodes": []interface{}{"NXDOMAIN"},
			}})
			if err == nil {
				t.Fatal("accepted a chain that asks one server twice")
			}
			if !strings.Contains(err.Error(), "twice") {
				t.Fatalf("rejected for some other reason: %v", err)
			}
		})
	}
}

// A panel tab open across an upgrade still sends the pre-#70 single "fallback"
// string. It has to keep working, or every save on the DNS page fails until the
// operator reloads — and nothing on the page says that is what to do.
func TestDnsFallbackAcceptsPreChainPayload(t *testing.T) {
	m := fallbackManager(t, `{`+twoServers+`}`)
	err := m.UpdateDnsSettings(map[string]interface{}{"fallback": map[string]interface{}{
		"enabled": true, "primary": "primary", "fallback": "backup",
		"rcodes": []interface{}{"SERVFAIL"},
	}})
	if err != nil {
		t.Fatalf("UpdateDnsSettings: %v", err)
	}
	got := m.GetDnsSettings()["fallback"].(map[string]interface{})
	want := map[string]interface{}{
		"enabled": true, "primary": "primary", "fallbacks": []interface{}{"backup"},
		"rcodes": []interface{}{"SERVFAIL"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("read back %#v, want %#v", got, want)
	}
}

// A block the panel cannot rewrite must not read back as one it owns. It used to:
// the panel then re-sent it with every unrelated DNS change and the write side
// rejected all of them, so nothing on the page could be saved at all.
func TestDnsFallbackIgnoresBlocksItCannotRewrite(t *testing.T) {
	cases := map[string]string{
		"an rcode the panel does not offer": `[{"action": "evaluate", "server": "primary"},
			{"match_response": true, "response_rcode": "NOERROR", "action": "route", "server": "backup"},
			{"match_response": true, "action": "respond"}]`,
		"primary and fallback are the same": `[{"action": "evaluate", "server": "backup"},
			{"match_response": true, "response_rcode": "NXDOMAIN", "action": "route", "server": "backup"},
			{"match_response": true, "action": "respond"}]`,
	}
	for name, rules := range cases {
		t.Run(name, func(t *testing.T) {
			m := fallbackManager(t, `{`+twoServers+`, "rules": `+rules+`}`)
			got := m.GetDnsSettings()["fallback"].(map[string]interface{})
			if got["enabled"] != false {
				t.Fatalf("read back as ours: %#v", got)
			}
			// ...and an unrelated settings change must still save.
			if err := m.UpdateDnsSettings(map[string]interface{}{"disable_cache": true, "fallback": got}); err != nil {
				t.Fatalf("an unrelated DNS setting could not be saved: %v", err)
			}
		})
	}
}

// Renaming a server the fallback uses is an ordinary panel action, and nothing
// rewrites rule references. The dangling tail must not read back as live, or every
// later DNS save is rejected for a tag the operator cannot see.
func TestDnsFallbackSurvivesServerRename(t *testing.T) {
	m := fallbackManager(t, `{`+twoServers+`}`)
	err := m.UpdateDnsSettings(map[string]interface{}{"fallback": map[string]interface{}{
		"enabled": true, "primary": "primary", "fallbacks": []interface{}{"backup"}, "rcodes": []interface{}{"NXDOMAIN"},
	}})
	if err != nil {
		t.Fatalf("UpdateDnsSettings: %v", err)
	}
	if err := m.UpdateDnsServer("backup", map[string]interface{}{"tag": "renamed", "type": "udp", "server": "8.8.8.8"}); err != nil {
		t.Fatalf("UpdateDnsServer: %v", err)
	}

	got := m.GetDnsSettings()["fallback"].(map[string]interface{})
	if got["enabled"] != false {
		t.Fatalf("a tail pointing at a deleted tag read back as live: %#v", got)
	}
	// The page sends the whole settings object back on the next change; it must land.
	if err := m.UpdateDnsSettings(map[string]interface{}{"disable_expire": true, "fallback": got}); err != nil {
		t.Fatalf("an unrelated DNS setting could not be saved after the rename: %v", err)
	}
	if rules := m.getDnsArray("rules"); len(rules) != 0 {
		t.Fatalf("the dangling tail should have been cleared, got %#v", rules)
	}
}

// Enabling on top of hand-written response rules would put the generated tail
// behind somebody's terminal `respond` — active in the panel, dead at runtime,
// and invisible because the rule list hides it.
func TestDnsFallbackRefusesToShareWithHandWrittenRules(t *testing.T) {
	m := fallbackManager(t, `{`+twoServers+`, "rules": [{"action": "evaluate", "server": "primary"}, {"domain": ["example.com"], "server": "backup"}]}`)
	err := m.UpdateDnsSettings(map[string]interface{}{"fallback": map[string]interface{}{
		"enabled": true, "primary": "primary", "fallbacks": []interface{}{"backup"}, "rcodes": []interface{}{"NXDOMAIN"},
	}})
	if err == nil {
		t.Fatalf("want a refusal, got rules %#v", m.getDnsArray("rules"))
	}
	if n := len(m.getDnsArray("rules")); n != 2 {
		t.Fatalf("the operator's rules were touched: %#v", m.getDnsArray("rules"))
	}
}

// Deleting a server the tail points at must fail — today it does only because the
// generic reference scan happens to see the generated rules.
func TestDeleteDnsServerBlockedByFallbackTail(t *testing.T) {
	m := fallbackManager(t, `{`+twoServers+`}`)
	err := m.UpdateDnsSettings(map[string]interface{}{"fallback": map[string]interface{}{
		"enabled": true, "primary": "primary", "fallbacks": []interface{}{"backup"}, "rcodes": []interface{}{"NXDOMAIN"},
	}})
	if err != nil {
		t.Fatalf("UpdateDnsSettings: %v", err)
	}
	for _, tag := range []string{"primary", "backup"} {
		err := m.DeleteDnsServer(tag)
		if err == nil {
			t.Fatalf("deleting %q, which the fallback uses, must fail", tag)
		}
		// The rule index is useless here: the panel hides those rules.
		if !strings.Contains(err.Error(), "fallback") {
			t.Fatalf("error should name the fallback, got: %v", err)
		}
	}
}

// Re-saving must rewrite the tail, not stack a second copy on top of the first.
func TestDnsFallbackRewritesInPlace(t *testing.T) {
	m := fallbackManager(t, `{`+twoServers+`}`)
	on := func(t *testing.T, codes ...interface{}) {
		t.Helper()
		err := m.UpdateDnsSettings(map[string]interface{}{"fallback": map[string]interface{}{
			"enabled": true, "primary": "primary", "fallbacks": []interface{}{"backup"}, "rcodes": codes,
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
		"no rcodes":       {"enabled": true, "primary": "primary", "fallbacks": []interface{}{"backup"}, "rcodes": []interface{}{}},
		"unknown rcode":   {"enabled": true, "primary": "primary", "fallbacks": []interface{}{"backup"}, "rcodes": []interface{}{"WHATEVER"}},
		"missing primary": {"enabled": true, "fallbacks": []interface{}{"backup"}, "rcodes": []interface{}{"NXDOMAIN"}},
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
		"enabled": true, "primary": "primary", "fallbacks": []interface{}{"backup"}, "rcodes": []interface{}{"NXDOMAIN"},
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
