package config

import "fmt"

// DNS fallback (#68): re-ask a second server when the first one answers with an
// error rcode. sing-box has no single switch for it — it is a shaped tail of the
// rule list, three kinds of rule appended in this order:
//
//	{"action": "evaluate", "server": "<primary>"}                                       -- ask, keep the answer, do not stop
//	{"match_response": true, "response_rcode": "<code>", "action": "route", "server": "<fallback>"}  -- one per code
//	{"match_response": true, "action": "respond"}                                       -- otherwise hand back what we got
//
// It lives at the TAIL so the operator's own rules, which are terminal, still win.
// response_rcode is a single value, not a list (the fork rejects an array), hence
// one route rule per code.
//
// The block carries no marker of its own — sing-box rejects unknown rule keys — so
// it is recognised by that exact shape and rewritten wholesale on every change.

// fallbackRcodes is what the panel offers. Anything miekg/dns knows would decode,
// but these are the failures worth re-asking about.
var fallbackRcodes = map[string]bool{
	"NXDOMAIN": true, "SERVFAIL": true, "REFUSED": true, "NOTIMP": true, "FORMERR": true,
}

// asRule is a rules[] element as a map, or nil for anything else.
func asRule(v interface{}) map[string]interface{} {
	obj, _ := v.(map[string]interface{})
	return obj
}

// findFallbackBlock reports where the generated tail starts and what it says.
// start is -1 when the rules do not end in one.
func findFallbackBlock(rules []interface{}) (start int, primary, fallback string, rcodes []string) {
	i := len(rules) - 1
	if i < 1 {
		return -1, "", "", nil
	}
	// tail: the catch-all respond
	r := asRule(rules[i])
	if len(r) != 2 || r["match_response"] != true || r["action"] != "respond" {
		return -1, "", "", nil
	}
	// walking up: the per-rcode route rules, all pointing at one server
	i--
	for ; i >= 0; i-- {
		r = asRule(rules[i])
		code, _ := r["response_rcode"].(string)
		server, _ := r["server"].(string)
		if len(r) != 4 || r["match_response"] != true || r["action"] != "route" || code == "" || server == "" {
			break
		}
		// Reading must accept no more than writing can reproduce. The fork matches
		// on rcodes this panel does not offer, and a block carrying one used to read
		// back as an ordinary fallback — which the panel then re-sent on every
		// unrelated DNS change, and applyDnsFallback rejected every one of them.
		// Every save on the page failed until the operator's own rules were deleted.
		if !fallbackRcodes[code] {
			return -1, "", "", nil
		}
		if fallback != "" && server != fallback {
			return -1, "", "", nil
		}
		fallback = server
		rcodes = append([]string{code}, rcodes...)
	}
	if len(rcodes) == 0 || i < 0 {
		return -1, "", "", nil
	}
	// head: the evaluate that fed them
	r = asRule(rules[i])
	primary, _ = r["server"].(string)
	if len(r) != 2 || r["action"] != "evaluate" || primary == "" || primary == fallback {
		return -1, "", "", nil
	}
	return i, primary, fallback, rcodes
}

// hasResponseRule reports whether a rule does response matching of its own. Used
// to refuse ownership of a config that already hand-writes this machinery.
func hasResponseRule(v interface{}) bool {
	r := asRule(v)
	_, matches := r["match_response"]
	return matches || r["action"] == "evaluate" || r["action"] == "respond"
}

// stripFallbackBlock returns rules with the generated tail removed, if present.
func stripFallbackBlock(rules []interface{}) []interface{} {
	if start, _, _, _ := findFallbackBlock(rules); start >= 0 {
		return rules[:start:start]
	}
	return rules
}

// buildFallbackBlock renders the three-part tail.
func buildFallbackBlock(primary, fallback string, rcodes []string) []interface{} {
	block := []interface{}{
		map[string]interface{}{"action": "evaluate", "server": primary},
	}
	for _, code := range rcodes {
		block = append(block, map[string]interface{}{
			"match_response": true, "response_rcode": code,
			"action": "route", "server": fallback,
		})
	}
	return append(block, map[string]interface{}{"match_response": true, "action": "respond"})
}

// readDnsFallback describes the configured fallback for GetDnsSettings. Caller holds the lock.
func (m *Manager) readDnsFallback() map[string]interface{} {
	start, primary, fallback, rcodes := findFallbackBlock(m.getDnsArray("rules"))
	if start < 0 {
		return map[string]interface{}{"enabled": false}
	}
	codes := make([]interface{}, len(rcodes))
	for i, c := range rcodes {
		codes[i] = c
	}
	return map[string]interface{}{
		"enabled": true, "primary": primary, "fallback": fallback, "rcodes": codes,
	}
}

// applyDnsFallback rewrites the generated tail from a settings payload. Caller
// holds the lock and has already ensured a draft exists.
func (m *Manager) applyDnsFallback(in map[string]interface{}) error {
	dns := m.getDraftDns()
	rules := stripFallbackBlock(m.getDraftDnsArray("rules"))

	if enabled, _ := in["enabled"].(bool); !enabled {
		dns["rules"] = rules
		return nil
	}

	// Whatever survived the strip is the operator's. If any of it already does
	// response matching, this generated tail would land behind their terminal
	// `respond` and never run, while the panel showed it as active — and the rule
	// list hides it, so there would be nothing to see. Refuse instead.
	for _, r := range rules {
		if hasResponseRule(r) {
			return fmt.Errorf("this config already has hand-written DNS response rules; " +
				"the panel will not manage a fallback alongside them")
		}
	}

	primary, _ := in["primary"].(string)
	fallback, _ := in["fallback"].(string)
	if primary == "" || fallback == "" {
		return fmt.Errorf("DNS fallback needs both a primary and a fallback server")
	}
	if primary == fallback {
		return fmt.Errorf("DNS fallback: the primary and the fallback server must differ")
	}
	tags := make(map[string]bool)
	for _, s := range m.getDnsArray("servers") {
		if obj := asRule(s); obj != nil {
			if t, ok := obj["tag"].(string); ok {
				tags[t] = true
			}
		}
	}
	for _, tag := range []string{primary, fallback} {
		if !tags[tag] {
			return fmt.Errorf("DNS server '%s' does not exist", tag)
		}
	}

	raw, _ := in["rcodes"].([]interface{})
	var rcodes []string
	seen := make(map[string]bool)
	for _, v := range raw {
		code, _ := v.(string)
		if !fallbackRcodes[code] {
			return fmt.Errorf("DNS fallback: '%v' is not a response code we can match on", v)
		}
		if !seen[code] {
			seen[code] = true
			rcodes = append(rcodes, code)
		}
	}
	if len(rcodes) == 0 {
		return fmt.Errorf("DNS fallback needs at least one response code to fall back on")
	}

	dns["rules"] = append(rules, buildFallbackBlock(primary, fallback, rcodes)...)
	return nil
}
