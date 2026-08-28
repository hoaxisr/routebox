package config

import "fmt"

// DNS fallback (#68, #70): re-ask another server when the previous one answers
// with an error rcode, down a chain of any length. sing-box has no single switch
// for it — it is a shaped tail of the rule list, appended in this order:
//
//	{"action": "evaluate", "server": "<primary>"}                                                      -- ask, keep the answer, do not stop
//	{"match_response": true, "response_rcode": "<code>", "action": "evaluate", "server": "<fallback>"} -- one per code, per NON-LAST fallback
//	{"match_response": true, "response_rcode": "<code>", "action": "route",    "server": "<last>"}     -- one per code
//	{"match_response": true, "action": "respond"}                                                      -- otherwise hand back what we got
//
// Every fallback but the last is another `evaluate`, which REPLACES the response
// the following rules match on; the router awaits it before matching, so a
// fallback that answers ends the chain there and `respond` returns its answer.
// Only the last one is a terminal `route`, because after it there is nothing left
// to try. Verified against 1.14.0-rc.1-awgm.14 with three local resolvers: the
// chain stops at the first server that does not return a listed rcode.
//
// It lives at the TAIL so the operator's own rules, which are terminal, still win.
// response_rcode is a single value, not a list (the fork rejects an array), hence
// one rule per code per hop.
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
// start is -1 when the rules do not end in one. fallbacks is in chain order: the
// terminal `route` server is the last of them, so a single-fallback tail — every
// block written before #70 — reads back as a one-element chain.
func findFallbackBlock(rules []interface{}) (start int, primary string, fallbacks, rcodes []string) {
	fail := func() (int, string, []string, []string) { return -1, "", nil, nil }
	i := len(rules) - 1
	if i < 1 {
		return fail()
	}
	// tail: the catch-all respond
	r := asRule(rules[i])
	if len(r) != 2 || r["match_response"] != true || r["action"] != "respond" {
		return fail()
	}
	i--

	// One hop up the chain: the run of per-rcode rules with `action`, all naming
	// the same server. Returns the server and the codes in config order, or ok=false
	// when the rule at i is not the end of such a run.
	hop := func(action string) (server string, codes []string, ok bool) {
		for ; i >= 0; i-- {
			r := asRule(rules[i])
			code, _ := r["response_rcode"].(string)
			srv, _ := r["server"].(string)
			if len(r) != 4 || r["match_response"] != true || r["action"] != action || code == "" || srv == "" {
				break
			}
			// Reading must accept no more than writing can reproduce. The fork matches
			// on rcodes this panel does not offer, and a block carrying one used to read
			// back as an ordinary fallback — which the panel then re-sent on every
			// unrelated DNS change, and applyDnsFallback rejected every one of them.
			// Every save on the page failed until the operator's own rules were deleted.
			if !fallbackRcodes[code] {
				return "", nil, false
			}
			if server != "" && srv != server {
				break // a different server: this is the previous hop, leave i on it
			}
			server = srv
			codes = append([]string{code}, codes...)
		}
		return server, codes, len(codes) > 0
	}

	// the terminal route hop, then every evaluate hop above it, innermost first
	last, rcodes, ok := hop("route")
	if !ok {
		return fail()
	}
	fallbacks = []string{last}
	for {
		server, codes, ok := hop("evaluate")
		if !ok {
			break
		}
		// A hop that matches on a different set of codes than the terminal one is
		// not something this panel can write, and re-sending it would rewrite the
		// operator's intent. Same reason the rcode whitelist is enforced above.
		if !sameStrings(codes, rcodes) {
			return fail()
		}
		fallbacks = append([]string{server}, fallbacks...)
	}
	if i < 0 {
		return fail()
	}

	// head: the evaluate that fed them
	r = asRule(rules[i])
	primary, _ = r["server"].(string)
	if len(r) != 2 || r["action"] != "evaluate" || primary == "" {
		return fail()
	}
	// A server appearing twice in the chain is a loop the panel never writes; a
	// primary that is also a fallback re-asks itself.
	seen := map[string]bool{primary: true}
	for _, f := range fallbacks {
		if seen[f] {
			return fail()
		}
		seen[f] = true
	}
	return i, primary, fallbacks, rcodes
}

// sameStrings reports whether two string slices are equal element for element.
func sameStrings(a, b []string) bool {
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

// buildFallbackBlock renders the tail: evaluate the primary, then one hop per
// fallback — `evaluate` for all but the last, `route` for the last — then respond.
func buildFallbackBlock(primary string, fallbacks, rcodes []string) []interface{} {
	block := []interface{}{
		map[string]interface{}{"action": "evaluate", "server": primary},
	}
	for n, server := range fallbacks {
		action := "evaluate"
		if n == len(fallbacks)-1 {
			action = "route"
		}
		for _, code := range rcodes {
			block = append(block, map[string]interface{}{
				"match_response": true, "response_rcode": code,
				"action": action, "server": server,
			})
		}
	}
	return append(block, map[string]interface{}{"match_response": true, "action": "respond"})
}

// dnsServerTags is the set of dns.servers[].tag. Caller holds the lock.
func (m *Manager) dnsServerTags() map[string]bool {
	tags := make(map[string]bool)
	for _, s := range m.getDnsArray("servers") {
		if obj := asRule(s); obj != nil {
			if t, ok := obj["tag"].(string); ok {
				tags[t] = true
			}
		}
	}
	return tags
}

// readDnsFallback describes the configured fallback for GetDnsSettings. Caller holds the lock.
func (m *Manager) readDnsFallback() map[string]interface{} {
	start, primary, fallbacks, rcodes := findFallbackBlock(m.getDnsArray("rules"))
	if start < 0 {
		return map[string]interface{}{"enabled": false}
	}
	// Renaming a DNS server leaves the tail pointing at a tag that is gone (nothing
	// rewrites rule references on rename). Reporting that as a live fallback bricked
	// the page: it goes back out with every unrelated DNS change and the write side
	// rejects the unknown tag every time. A tail sing-box itself would refuse to
	// load reads as "no fallback", and the next save clears it out.
	tags := m.dnsServerTags()
	if !tags[primary] {
		return map[string]interface{}{"enabled": false}
	}
	servers := make([]interface{}, len(fallbacks))
	for i, f := range fallbacks {
		if !tags[f] {
			return map[string]interface{}{"enabled": false}
		}
		servers[i] = f
	}
	codes := make([]interface{}, len(rcodes))
	for i, c := range rcodes {
		codes[i] = c
	}
	return map[string]interface{}{
		"enabled": true, "primary": primary, "fallbacks": servers, "rcodes": codes,
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
	if primary == "" {
		return fmt.Errorf("DNS fallback needs a primary server")
	}
	rawServers, _ := in["fallbacks"].([]interface{})
	var fallbacks []string
	seen := map[string]bool{primary: true}
	for _, v := range rawServers {
		server, _ := v.(string)
		if server == "" {
			return fmt.Errorf("DNS fallback: every fallback needs a server")
		}
		// A repeat would ask the same server twice in a row, and a fallback equal to
		// the primary re-asks the server that just failed. Both are silently useless
		// rather than broken, which is worse.
		if seen[server] {
			return fmt.Errorf("DNS fallback: '%s' appears twice in the chain", server)
		}
		seen[server] = true
		fallbacks = append(fallbacks, server)
	}
	if len(fallbacks) == 0 {
		return fmt.Errorf("DNS fallback needs at least one fallback server")
	}
	tags := m.dnsServerTags()
	for _, tag := range append([]string{primary}, fallbacks...) {
		if !tags[tag] {
			return fmt.Errorf("DNS server '%s' does not exist", tag)
		}
	}

	raw, _ := in["rcodes"].([]interface{})
	var rcodes []string
	seenCode := make(map[string]bool)
	for _, v := range raw {
		code, _ := v.(string)
		if !fallbackRcodes[code] {
			return fmt.Errorf("DNS fallback: '%v' is not a response code we can match on", v)
		}
		if !seenCode[code] {
			seenCode[code] = true
			rcodes = append(rcodes, code)
		}
	}
	if len(rcodes) == 0 {
		return fmt.Errorf("DNS fallback needs at least one response code to fall back on")
	}

	dns["rules"] = append(rules, buildFallbackBlock(primary, fallbacks, rcodes)...)
	return nil
}
