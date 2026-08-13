package config

import "fmt"

// route.default_domain_resolver stops being optional at a threshold the option
// docs never spell out. Dialers resolve through it, and common/dialer/dialer.go
// only lets a missing resolver slide while there is exactly ONE DNS transport
// (it silently uses that one); from the SECOND dns.servers entry on it reports
// the `missing-domain-resolver` deprecation, which sing-box 1.13+ turns into
//
//	ERROR missing `route.default_domain_resolver` or `domain_resolver` in dial fields ...
//	FATAL to continuing using this feature, set ENABLE_DEPRECATED_MISSING_DOMAIN_RESOLVER=true
//
// and a FATAL is os.Exit(1) — at every start, reload and `check`. The trigger is
// not "an outbound addressed by domain": a plain `direct` outbound is enough,
// because it dials whatever it is handed, so in practice every RouteBox config
// past two DNS servers needs the pin.
//
// The panel is what walks operators across that line — the DNS fallback (#68)
// needs a second server by construction — so the panel carries the pin along:
// set it when the second server appears, follow it through renames, and never
// leave it naming a server that is gone.
const domainResolverKey = "default_domain_resolver"

// domainResolverRequired reports whether this many dns.servers put sing-box past
// the one-transport grace.
func domainResolverRequired(servers []interface{}) bool { return len(servers) >= 2 }

// domainResolverTag reads route.default_domain_resolver, which is either a server
// tag or {"server": tag, ...}. Empty means unset.
func domainResolverTag(route map[string]interface{}) string {
	switch v := route[domainResolverKey].(type) {
	case string:
		return v
	case map[string]interface{}:
		tag, _ := v["server"].(string)
		return tag
	}
	return ""
}

// setDomainResolverTag repoints an existing pin, preserving its object form (the
// object may carry strategy/client_subnet the operator set by hand).
func setDomainResolverTag(route map[string]interface{}, tag string) {
	if obj, ok := route[domainResolverKey].(map[string]interface{}); ok {
		obj["server"] = tag
		return
	}
	route[domainResolverKey] = tag
}

// errDomainResolverRequired explains the refusal in the panel's terms: sing-box's
// own message names a JSON field the operator never typed.
func errDomainResolverRequired(what string) error {
	return fmt.Errorf("%s: with two or more DNS servers sing-box requires a default domain resolver "+
		"(Routing → Settings → Default domain resolver) and exits at startup without one. "+
		"Point it at another DNS server first, or go back to a single server", what)
}

// pinDomainResolver sets route.default_domain_resolver when the draft has just
// crossed the two-server line without one. It picks the server sing-box was
// already using implicitly — dns.final, else the first — so outgoing connections
// resolve exactly as they did with a single server. Caller holds the lock and has
// ensured a draft exists.
func (m *Manager) pinDomainResolver() {
	servers := m.getDraftDnsArray("servers")
	if !domainResolverRequired(servers) {
		return
	}
	route := m.getDraftRoute()
	if domainResolverTag(route) != "" {
		return
	}
	tag := ""
	if final, ok := m.getDraftDns()["final"].(string); ok && findByTag(servers, final) >= 0 {
		tag = final
	} else if obj := asRule(servers[0]); obj != nil {
		tag, _ = obj["tag"].(string)
	}
	if tag != "" {
		route[domainResolverKey] = tag
	}
}
