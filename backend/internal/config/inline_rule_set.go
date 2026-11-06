package config

import (
	"fmt"
	"net"
	"reflect"
	"strings"

	"golang.org/x/net/idna"
)

// NormalizeDomainSuffix turns what people paste into an entry sing-box can
// match. It compares domain_suffix literally against a lowercase ASCII name,
// so "*.x.com", "X.com", "https://x.com/" and "кремль.рф" would be accepted
// and silently never match. "*.x.com" becomes "x.com": the bare name matches
// too, and for routing that is what a wildcard means in practice. A leading
// dot stays: in sing-box it deliberately means subdomains only.
func NormalizeDomainSuffix(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if _, rest, ok := strings.Cut(s, "://"); ok {
		s = rest
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	s = strings.TrimSuffix(strings.TrimPrefix(s, "*."), ".")
	if ascii, err := idna.ToASCII(s); err == nil {
		s = ascii
	}
	return s
}

// normalizeHeadlessRules rewrites domain_suffix in place, logical rules
// included, dropping entries that end up empty or duplicated.
func normalizeHeadlessRules(rules []interface{}) bool {
	changed := false
	for _, raw := range rules {
		rule, _ := raw.(map[string]interface{})
		if nested, ok := rule["rules"].([]interface{}); ok && normalizeHeadlessRules(nested) {
			changed = true
		}
		var entries []interface{}
		switch v := rule["domain_suffix"].(type) {
		case string: // sing-box takes a single string as a list of one
			entries = []interface{}{v}
		case []interface{}:
			entries = v
		default:
			continue
		}
		out := make([]interface{}, 0, len(entries))
		seen := make(map[string]bool, len(entries))
		for _, e := range entries {
			s, ok := e.(string)
			if !ok {
				out = append(out, e)
				continue
			}
			if s = NormalizeDomainSuffix(s); s != "" && !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
		if !reflect.DeepEqual(rule["domain_suffix"], out) {
			rule["domain_suffix"] = out
			changed = true
		}
	}
	return changed
}

func normalizeInlineRuleSet(rs map[string]interface{}) bool {
	if t, _ := rs["type"].(string); t != "inline" {
		return false
	}
	rules, _ := rs["rules"].([]interface{})
	return normalizeHeadlessRules(rules)
}

// NormalizeInlineRuleSets fixes domain entries already in a config. Returns
// one note per changed set; empty = untouched. Idempotent.
func NormalizeInlineRuleSets(config map[string]interface{}) []string {
	route, _ := config["route"].(map[string]interface{})
	sets, _ := route["rule_set"].([]interface{})
	var notes []string
	for _, v := range sets {
		rs, _ := v.(map[string]interface{})
		if normalizeInlineRuleSet(rs) {
			notes = append(notes, fmt.Sprintf("rule_set %q: domain_suffix normalized", rs["tag"]))
		}
	}
	return notes
}

// headlessRuleEmpty reports a rule with no condition: sing-box refuses it at
// `check`, without naming the set.
func headlessRuleEmpty(rule map[string]interface{}) bool {
	for k, v := range rule {
		if k == "invert" {
			continue
		}
		switch x := v.(type) {
		case nil:
		case []interface{}:
			if len(x) > 0 {
				return false
			}
		case string:
			if x != "" {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func unmatchableDomains(rules []interface{}) []string {
	var bad []string
	for _, raw := range rules {
		rule, _ := raw.(map[string]interface{})
		nested, _ := rule["rules"].([]interface{})
		bad = append(bad, unmatchableDomains(nested)...)
		entries, _ := rule["domain_suffix"].([]interface{})
		for _, e := range entries {
			if s, _ := e.(string); strings.ContainsAny(s, " \t*:") {
				bad = append(bad, s)
			}
		}
	}
	return bad
}

// ValidateInlineRuleSets runs at apply, not on save: a set is legitimately
// empty between its creation and the first domain. Names the set, which
// sing-box's own error does not, and refuses entries that can never match.
func ValidateInlineRuleSets(config map[string]interface{}) []string {
	route, _ := config["route"].(map[string]interface{})
	sets, _ := route["rule_set"].([]interface{})
	var errors []string
	for _, v := range sets {
		rs, _ := v.(map[string]interface{})
		if t, _ := rs["type"].(string); t != "inline" {
			continue
		}
		tag, _ := rs["tag"].(string)
		rules, _ := rs["rules"].([]interface{})
		empty := len(rules) == 0
		for _, raw := range rules {
			if rule, _ := raw.(map[string]interface{}); headlessRuleEmpty(rule) {
				empty = true
			}
		}
		if empty {
			errors = append(errors, fmt.Sprintf("rule_set %q: an inline set needs at least one domain in every rule — add one or delete the set", tag))
		}
		for _, d := range unmatchableDomains(rules) {
			errors = append(errors, fmt.Sprintf("rule_set %q: domain %q can never match — use a plain name like example.com", tag, d))
		}
	}
	return errors
}
