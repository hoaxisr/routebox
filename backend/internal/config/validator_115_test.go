package config

import "testing"

func TestValidateRuleSetRejectsDownloadDetour(t *testing.T) {
	rs := map[string]interface{}{"tag": "ads", "type": "remote", "url": "u", "download_detour": "direct"}
	if errs := validateRuleSet(rs, 0); !hasErrContaining(errs, "download_detour") {
		t.Errorf("download_detour is fatal on sing-box 1.15 at start (check does not catch it): %v", errs)
	}
	rs = map[string]interface{}{"tag": "ads", "type": "remote", "url": "u", "http_client": map[string]interface{}{"detour": "proxy"}}
	if errs := validateRuleSet(rs, 0); len(errs) != 0 {
		t.Errorf("http_client is the replacement: %v", errs)
	}
}

func TestValidateDnsServerRejectsDomainStrategy(t *testing.T) {
	srv := map[string]interface{}{"tag": "d", "type": "tls", "server": "1.1.1.1", "domain_strategy": "ipv4_only"}
	if errs := validateDnsServer(srv, 0); !hasErrContaining(errs, "domain_strategy") {
		t.Errorf("legacy domain_strategy on a DNS server is fatal since 1.14: %v", errs)
	}
}

func TestValidateDnsRuleAddressFilterNeedsMatchResponse(t *testing.T) {
	servers := map[string]bool{"d": true}
	rule := map[string]interface{}{"ip_cidr": []interface{}{"1.1.1.1/32"}, "server": "d"}
	if errs := validateDnsRule(rule, 0, servers, nil); !hasErrContaining(errs, "match_response") {
		t.Errorf("ip_cidr without match_response: %v", errs)
	}
	rule = map[string]interface{}{"ip_is_private": true, "server": "d"}
	if errs := validateDnsRule(rule, 0, servers, nil); !hasErrContaining(errs, "match_response") {
		t.Errorf("ip_is_private without match_response: %v", errs)
	}
	rule = map[string]interface{}{"ip_cidr": []interface{}{"1.1.1.1/32"}, "match_response": true, "server": "d"}
	if errs := validateDnsRule(rule, 0, servers, nil); len(errs) != 0 {
		t.Errorf("with match_response it is a response matcher: %v", errs)
	}
}

func TestValidateDnsRuleDisableCacheOnlyForRoute(t *testing.T) {
	for _, action := range []string{"reject", "predefined"} {
		rule := map[string]interface{}{"domain": []interface{}{"x"}, "action": action, "disable_cache": true, "rcode": "NXDOMAIN"}
		if errs := validateDnsRule(rule, 0, nil, nil); !hasErrContaining(errs, "disable_cache") {
			t.Errorf("%s action has no disable_cache field (unknown field on check): %v", action, errs)
		}
	}
	servers := map[string]bool{"d": true}
	for _, rule := range []map[string]interface{}{
		{"domain": []interface{}{"x"}, "action": "route", "server": "d", "disable_cache": true},
		{"domain": []interface{}{"x"}, "server": "d", "disable_cache": true},
	} {
		if errs := validateDnsRule(rule, 0, servers, nil); len(errs) != 0 {
			t.Errorf("route action keeps disable_cache: %v", errs)
		}
	}
}

func TestValidateInboundRejectsInlineACME(t *testing.T) {
	ib := map[string]interface{}{"type": "trojan", "tag": "t", "listen": "::", "listen_port": float64(443),
		"users": []interface{}{map[string]interface{}{"name": "u", "password": "p"}},
		"tls":   map[string]interface{}{"enabled": true, "acme": map[string]interface{}{"domain": "ex.com"}}}
	if errs := validateInbound(ib, 0); !hasErrContaining(errs, "certificate_provider") {
		t.Errorf("inline tls.acme is fatal on 1.15 check: %v", errs)
	}
	ib["tls"] = map[string]interface{}{"enabled": true, "certificate_provider": map[string]interface{}{"type": "acme", "domain": []interface{}{"ex.com"}}}
	if errs := validateInbound(ib, 0); len(errs) != 0 {
		t.Errorf("certificate_provider is the replacement: %v", errs)
	}
}

func TestValidateEndpointFieldsPerType(t *testing.T) {
	base := func(typ string, extra map[string]interface{}) map[string]interface{} {
		ep := map[string]interface{}{
			"tag": "ep", "type": typ, "private_key": "k", "address": []interface{}{"10.0.0.2/32"},
			"peers": []interface{}{map[string]interface{}{"address": "1.2.3.4", "port": float64(1), "public_key": "p"}},
		}
		for k, v := range extra {
			ep[k] = v
		}
		return ep
	}
	// wireguard-only fields on an awg endpoint → unknown field on check.
	for _, f := range []string{"system", "name", "udp_timeout", "workers"} {
		ep := base("awg", map[string]interface{}{f: "x"})
		if errs := validateEndpoint(ep, 0); !hasErrContaining(errs, f) {
			t.Errorf("awg endpoint with %q must be rejected: %v", f, errs)
		}
	}
	if errs := validateEndpoint(base("wireguard", map[string]interface{}{"system": true, "workers": float64(2)}), 0); len(errs) != 0 {
		t.Errorf("wireguard accepts system/workers: %v", errs)
	}
	// Peer PSK key differs per type: awg → preshared_key, wireguard → pre_shared_key.
	peer := func(key string) []interface{} {
		return []interface{}{map[string]interface{}{"address": "1.2.3.4", "port": float64(1), "public_key": "p", key: "psk"}}
	}
	if errs := validateEndpoint(base("wireguard", map[string]interface{}{"peers": peer("preshared_key")}), 0); !hasErrContaining(errs, "pre_shared_key") {
		t.Errorf("wireguard peer with preshared_key must point at pre_shared_key: %v", errs)
	}
	if errs := validateEndpoint(base("awg", map[string]interface{}{"peers": peer("pre_shared_key")}), 0); !hasErrContaining(errs, "preshared_key") {
		t.Errorf("awg peer with pre_shared_key must point at preshared_key: %v", errs)
	}
	if errs := validateEndpoint(base("wireguard", map[string]interface{}{"peers": peer("pre_shared_key")}), 0); len(errs) != 0 {
		t.Errorf("wireguard pre_shared_key ok: %v", errs)
	}
	if errs := validateEndpoint(base("awg", map[string]interface{}{"peers": peer("preshared_key")}), 0); len(errs) != 0 {
		t.Errorf("awg preshared_key ok: %v", errs)
	}
}
