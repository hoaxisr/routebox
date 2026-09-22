package config

import "fmt"

// Shared HTTP clients RouteBox declares for remote rule-set downloads.
// ruleSetHTTPClientTag replaces sing-box's old implicit client, which dialed
// through the DEFAULT outbound (route.final) — so it carries `detour: <final>`
// unless final is an empty direct outbound. ruleSetDirectHTTPClientTag is what
// an explicit `download_detour: <empty direct>` becomes: no detour = dial
// direct (sing-box rejects `detour` pointing at an empty direct outbound).
const (
	ruleSetHTTPClientTag       = "rule-sets"
	ruleSetDirectHTTPClientTag = "rule-sets-direct"
)

// Singbox115Migration gates migrateSingbox115. Its output (http_clients,
// certificate_provider, store_dns) exists since sing-box 1.14; main turns it
// off when the installed fork is older, so a config is never rewritten into
// fields the running binary rejects. Unknown/missing binary → stays on: the
// next install is a current fork.
var Singbox115Migration = true

// MigrateSingbox115 is migrateSingbox115 for raw configs the API takes, gated
// like Load. Returns the notes (nil when gated off or nothing changed).
func MigrateSingbox115(config map[string]interface{}) []string {
	if !Singbox115Migration {
		return nil
	}
	return migrateSingbox115(config)
}

// migrateSingbox115 rewrites what sing-box 1.15 refuses. Everything deprecated
// in 1.14 became a start-time Fatal one minor later (deprecated.Note.Impending),
// and `check` does not exercise rule-set downloads, so a config that passes
// preflight can still kill the process on start. Returns one note per change;
// empty = untouched. Idempotent.
func migrateSingbox115(config map[string]interface{}) []string {
	var notes []string
	route, _ := config["route"].(map[string]interface{})

	// Default client first: the direct client added below must never become
	// sing-box's "first entry" fallback for the other rule-sets.
	if ensureRuleSetHTTPClient(config) {
		notes = append(notes, fmt.Sprintf("http_clients[%q] + route.default_http_client added for remote rule-sets", ruleSetHTTPClientTag))
	}
	if route != nil {
		// rule_set: download_detour → http_client. The shared default client
		// may dial through route.final; an explicit direct download must not.
		sharedDetour := ruleSetSharedDetour(config)
		if arr, ok := route["rule_set"].([]interface{}); ok {
			for _, v := range arr {
				rs, _ := v.(map[string]interface{})
				if rs == nil {
					continue
				}
				if _, has := rs["download_detour"]; !has {
					continue
				}
				detour, _ := rs["download_detour"].(string)
				delete(rs, "download_detour")
				if rs["http_client"] == nil && detour != "" {
					switch {
					case !isEmptyDirectOutbound(config, detour):
						rs["http_client"] = map[string]interface{}{"detour": detour}
					case sharedDetour != "":
						rs["http_client"] = ruleSetDirectHTTPClientTag
						ensureHTTPClient(config, ruleSetDirectHTTPClientTag, "")
					}
				}
				notes = append(notes, fmt.Sprintf("rule_set %q: download_detour → http_client", rs["tag"]))
			}
		}
		if _, has := route["default_domain_strategy"]; has {
			delete(route, "default_domain_strategy")
			notes = append(notes, "route.default_domain_strategy removed (not a sing-box option)")
		}
	}

	// cache_file: store_rdrc / rdrc_timeout → store_dns.
	if exp, ok := config["experimental"].(map[string]interface{}); ok {
		if cf, ok := exp["cache_file"].(map[string]interface{}); ok {
			rdrc, _ := cf["store_rdrc"].(bool)
			_, hasRdrc := cf["store_rdrc"]
			_, hasTimeout := cf["rdrc_timeout"]
			if hasRdrc || hasTimeout {
				delete(cf, "store_rdrc")
				delete(cf, "rdrc_timeout")
				if rdrc {
					cf["store_dns"] = true
				}
				notes = append(notes, "cache_file.store_rdrc → store_dns")
			}
		}
	}

	// inbounds: inline tls.acme → tls.certificate_provider {type: acme}.
	if arr, ok := config["inbounds"].([]interface{}); ok {
		for _, v := range arr {
			ib, _ := v.(map[string]interface{})
			if ib == nil {
				continue
			}
			tls, _ := ib["tls"].(map[string]interface{})
			if tls == nil {
				continue
			}
			acme, _ := tls["acme"].(map[string]interface{})
			if acme == nil {
				continue
			}
			delete(tls, "acme")
			if tls["certificate_provider"] == nil {
				acme["type"] = "acme"
				tls["certificate_provider"] = acme
			}
			notes = append(notes, fmt.Sprintf("inbound %q: tls.acme → tls.certificate_provider", ib["tag"]))
		}
	}

	// dns.servers: legacy domain_strategy (fatal since 1.14).
	if dns, ok := config["dns"].(map[string]interface{}); ok {
		if arr, ok := dns["servers"].([]interface{}); ok {
			for _, v := range arr {
				srv, _ := v.(map[string]interface{})
				if srv == nil {
					continue
				}
				if _, has := srv["domain_strategy"]; has {
					delete(srv, "domain_strategy")
					notes = append(notes, fmt.Sprintf("dns server %q: domain_strategy removed", srv["tag"]))
				}
			}
		}
	}
	return notes
}

// isEmptyDirectOutbound mirrors sing-box's DirectDialer.IsEmpty: a direct
// outbound with no dial options at all. `detour` to one is rejected at start.
func isEmptyDirectOutbound(config map[string]interface{}, tag string) bool {
	arr, _ := config["outbounds"].([]interface{})
	for _, v := range arr {
		ob, _ := v.(map[string]interface{})
		if t, _ := ob["tag"].(string); t != tag {
			continue
		}
		if typ, _ := ob["type"].(string); typ != "direct" {
			return false
		}
		for k := range ob {
			if k != "tag" && k != "type" {
				return false
			}
		}
		return true
	}
	return false
}

// ruleSetSharedDetour is the detour of the shared rule-set client: the old
// implicit client used the default outbound, i.e. route.final.
func ruleSetSharedDetour(config map[string]interface{}) string {
	route, _ := config["route"].(map[string]interface{})
	final, _ := route["final"].(string)
	if final == "" || isEmptyDirectOutbound(config, final) {
		return ""
	}
	return final
}

// ensureHTTPClient appends {tag, detour?} to http_clients unless tag exists.
func ensureHTTPClient(config map[string]interface{}, tag, detour string) bool {
	clients, _ := config["http_clients"].([]interface{})
	for _, v := range clients {
		c, _ := v.(map[string]interface{})
		if t, _ := c["tag"].(string); t == tag {
			return false
		}
	}
	client := map[string]interface{}{"tag": tag}
	if detour != "" {
		client["detour"] = detour
	}
	config["http_clients"] = append(clients, client)
	return true
}

// ensureRuleSetHTTPClient gives remote rule-sets an explicit default client
// when the config declares none (the implicit one is fatal on 1.15): the
// shared client plus route.default_http_client. Returns true when added.
func ensureRuleSetHTTPClient(config map[string]interface{}) bool {
	route, _ := config["route"].(map[string]interface{})
	if route == nil {
		return false
	}
	arr, _ := route["rule_set"].([]interface{})
	hasRemote := false
	for _, v := range arr {
		rs, _ := v.(map[string]interface{})
		if t, _ := rs["type"].(string); t == "remote" {
			hasRemote = true
			break
		}
	}
	if !hasRemote {
		return false
	}
	if d, _ := route["default_http_client"].(string); d != "" {
		return false
	}
	if clients, _ := config["http_clients"].([]interface{}); len(clients) > 0 {
		return false // sing-box falls back to the first entry when no default is named
	}
	ensureHTTPClient(config, ruleSetHTTPClientTag, ruleSetSharedDetour(config))
	route["default_http_client"] = ruleSetHTTPClientTag
	return true
}
