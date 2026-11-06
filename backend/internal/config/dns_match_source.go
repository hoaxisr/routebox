package config

import (
	"fmt"
	"regexp"
)

const dnsMatchSourceKey = "rule_set_ip_cidr_match_source"

// A DNS rule whose rule_set holds any ip_cidr puts sing-box 1.15 into legacy DNS
// mode, which is Fatal at start ("Legacy Address Filter Fields"). The content of
// a remote set is known only after the download, so `check` passes it. Setting
// this flag from the rule's shape avoids the mode for every set without reading
// any of them; awg-manager does the same (applyDNSRuleSetMatchSource).
//
// The IP part of the set then matches the client's address instead of the
// destination, which in a plain DNS rule is the answer and does not exist yet.
// With match_response the IPs match the answer, and the flag would break that,
// so it is removed there. Logical children are walked: sing-box reads them too.
func applyDNSRuleSetMatchSource(config map[string]interface{}) bool {
	dns, _ := config["dns"].(map[string]interface{})
	rules, _ := dns["rules"].([]interface{})
	return markDNSRules(rules, false)
}

func markDNSRules(rules []interface{}, matchesResponse bool) bool {
	changed := false
	for _, raw := range rules {
		rule, _ := raw.(map[string]interface{})
		if rule == nil {
			continue
		}
		response := matchesResponse || dnsRuleMatchesResponse(rule)
		if nested, ok := rule["rules"].([]interface{}); ok && markDNSRules(nested, response) {
			changed = true
		}
		_, has := rule[dnsMatchSourceKey]
		switch want := len(dnsRuleSetTags(rule)) > 0 && !response; {
		case want && rule[dnsMatchSourceKey] != true:
			rule[dnsMatchSourceKey] = true
			changed = true
		case !want && has:
			delete(rule, dnsMatchSourceKey)
			changed = true
		}
	}
	return changed
}

// match_response is a bool or the tag of an evaluate rule.
func dnsRuleMatchesResponse(rule map[string]interface{}) bool {
	switch v := rule["match_response"].(type) {
	case bool:
		return v
	case string:
		return v != ""
	}
	return false
}

// rule_set is a list, or a single string that sing-box reads as a list of one.
func dnsRuleSetTags(rule map[string]interface{}) []string {
	var tags []string
	switch v := rule["rule_set"].(type) {
	case string:
		if v != "" {
			tags = append(tags, v)
		}
	case []interface{}:
		for _, e := range v {
			if s, _ := e.(string); s != "" {
				tags = append(tags, s)
			}
		}
	}
	return tags
}

var (
	geoipTag  = regexp.MustCompile(`(?i)^geoip([-_:]|$)`)
	geoipURL  = regexp.MustCompile(`(?i)[?&]kind=geoip\b|/geoip(?:[-_/]|\.srs|\b)`)
	geoipFile = regexp.MustCompile(`(?i)geoip`)
)

// isGeoIPRuleSet goes by names: a remote set's content is unknown before its
// download. The same test as the DNS rule form (isDnsAddressFilterRuleSet).
func isGeoIPRuleSet(rs map[string]interface{}) bool {
	tag, _ := rs["tag"].(string)
	url, _ := rs["url"].(string)
	path, _ := rs["path"].(string)
	return geoipTag.MatchString(tag) || geoipURL.MatchString(url) || geoipFile.MatchString(path)
}

// ValidateDNSRuleSets refuses a purely IP set in a DNS rule without
// match_response. The flag above does not cover those: sing-box rejects them at
// start, and `check` cannot see it for a remote set. The form does not offer
// them, this is for the raw editor and for rules made before it stopped to.
func ValidateDNSRuleSets(config map[string]interface{}) []string {
	dns, _ := config["dns"].(map[string]interface{})
	rules, _ := dns["rules"].([]interface{})
	route, _ := config["route"].(map[string]interface{})
	sets, _ := route["rule_set"].([]interface{})
	geoip := map[string]bool{}
	for _, v := range sets {
		if rs, _ := v.(map[string]interface{}); rs != nil && isGeoIPRuleSet(rs) {
			tag, _ := rs["tag"].(string)
			geoip[tag] = true
		}
	}
	var errors []string
	var walk func(rules []interface{}, matchesResponse bool)
	walk = func(rules []interface{}, matchesResponse bool) {
		for _, raw := range rules {
			rule, _ := raw.(map[string]interface{})
			if rule == nil {
				continue
			}
			response := matchesResponse || dnsRuleMatchesResponse(rule)
			if nested, ok := rule["rules"].([]interface{}); ok {
				walk(nested, response)
			}
			if response {
				continue
			}
			for _, tag := range dnsRuleSetTags(rule) {
				if geoip[tag] {
					errors = append(errors, fmt.Sprintf("DNS rule: rule set %q holds IP addresses, which sing-box refuses at start in a DNS rule without match_response — remove it from the rule or put an 'evaluate' rule before it and enable match_response", tag))
				}
			}
		}
	}
	walk(rules, false)
	return errors
}
