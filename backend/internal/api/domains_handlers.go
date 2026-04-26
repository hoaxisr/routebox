package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// deepCopyRuleSet returns a deep copy of a rule-set map so callers can mutate
// freely without affecting the manager's internal config state.
func deepCopyRuleSet(rs map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range rs {
		out[k] = deepCopyValue(v)
	}
	return out
}

func deepCopyValue(v interface{}) interface{} {
	switch x := v.(type) {
	case map[string]interface{}:
		m := map[string]interface{}{}
		for k, vv := range x {
			m[k] = deepCopyValue(vv)
		}
		return m
	case []interface{}:
		s := make([]interface{}, len(x))
		for i, vv := range x {
			s[i] = deepCopyValue(vv)
		}
		return s
	default:
		return v
	}
}

// inlineRuleSets returns deep copies of all rule_set entries with type=inline
// from the draft config so handlers can mutate without touching live state.
func (h *Handler) inlineRuleSets() []map[string]interface{} {
	out := []map[string]interface{}{}
	for _, rs := range h.config.ListRuleSets() {
		if t, _ := rs["type"].(string); t == "inline" {
			out = append(out, deepCopyRuleSet(rs))
		}
	}
	return out
}

func (h *Handler) findInlineRuleSet(tag string) (map[string]interface{}, bool) {
	for _, rs := range h.config.ListRuleSets() {
		t, _ := rs["type"].(string)
		rsTag, _ := rs["tag"].(string)
		if t == "inline" && rsTag == tag {
			return deepCopyRuleSet(rs), true
		}
	}
	return nil, false
}

// DomainSetInfo is the listing shape returned to the UI.
type DomainSetInfo struct {
	Tag         string `json:"tag"`
	DomainCount int    `json:"domain_count"`
	RulesCount  int    `json:"rules_count"`
}

func countDomains(rs map[string]interface{}) (rules int, domains int) {
	rulesAny, _ := rs["rules"].([]interface{})
	rules = len(rulesAny)
	for _, r := range rulesAny {
		rule, _ := r.(map[string]interface{})
		ds, _ := rule["domain_suffix"].([]interface{})
		domains += len(ds)
		if d, _ := rule["domain"].([]interface{}); d != nil {
			domains += len(d)
		}
	}
	return
}

// ListDomainSets returns all inline rule-sets in the draft config.
func (h *Handler) ListDomainSets(w http.ResponseWriter, r *http.Request) {
	rss := h.inlineRuleSets()
	out := make([]DomainSetInfo, 0, len(rss))
	for _, rs := range rss {
		tag, _ := rs["tag"].(string)
		rulesCount, domainCount := countDomains(rs)
		out = append(out, DomainSetInfo{
			Tag:         tag,
			DomainCount: domainCount,
			RulesCount:  rulesCount,
		})
	}
	writeSuccess(w, out)
}

// CreateDomainSet creates a new empty inline rule-set.
func (h *Handler) CreateDomainSet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Tag string `json:"tag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}
	tag := strings.TrimSpace(body.Tag)
	if tag == "" {
		writeError(w, http.StatusBadRequest, "tag is required")
		return
	}

	for _, rs := range h.config.ListRuleSets() {
		if t, _ := rs["tag"].(string); t == tag {
			writeError(w, http.StatusConflict, fmt.Sprintf("tag '%s' is already used by an existing rule-set", tag))
			return
		}
	}

	rs := map[string]interface{}{
		"tag":   tag,
		"type":  "inline",
		"rules": []interface{}{},
	}
	if err := h.config.CreateRuleSet(rs); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeSuccess(w, map[string]string{
		"tag":     tag,
		"message": fmt.Sprintf("inline rule-set '%s' created", tag),
	})
}

// GetDomainSet returns the full inline rule-set body.
func (h *Handler) GetDomainSet(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")
	rs, ok := h.findInlineRuleSet(tag)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("inline rule-set '%s' not found", tag))
		return
	}
	writeSuccess(w, rs)
}

// SaveDomainSet replaces the rules array of an inline rule-set.
func (h *Handler) SaveDomainSet(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")
	var body struct {
		Tag   string                   `json:"tag"`
		Rules []map[string]interface{} `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}
	rs, ok := h.findInlineRuleSet(tag)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("inline rule-set '%s' not found", tag))
		return
	}
	rulesAny := make([]interface{}, len(body.Rules))
	for i, rule := range body.Rules {
		rulesAny[i] = rule
	}
	updated := map[string]interface{}{}
	for k, v := range rs {
		updated[k] = v
	}
	updated["rules"] = rulesAny
	if err := h.config.UpdateRuleSet(tag, updated); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeSuccess(w, map[string]string{"message": fmt.Sprintf("inline rule-set '%s' saved", tag)})
}

// DeleteDomainSet removes an inline rule-set; refuses if referenced by route or DNS rules.
func (h *Handler) DeleteDomainSet(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")
	if _, ok := h.findInlineRuleSet(tag); !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("inline rule-set '%s' not found", tag))
		return
	}
	usage := h.config.GetRuleSetsUsage()
	if u, ok := usage[tag]; ok {
		if len(u["route_rules"]) > 0 || len(u["dns_rules"]) > 0 {
			writeError(w, http.StatusConflict, fmt.Sprintf("rule-set '%s' is referenced by route or DNS rules — remove references first", tag))
			return
		}
	}
	if err := h.config.DeleteRuleSet(tag); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeSuccess(w, map[string]string{"message": fmt.Sprintf("inline rule-set '%s' deleted", tag)})
}

// AddDomain appends a single domain_suffix entry to the rule-set's first rule.
func (h *Handler) AddDomain(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")
	var body struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}
	domain := strings.TrimSpace(body.Domain)
	if domain == "" {
		writeError(w, http.StatusBadRequest, "domain is required")
		return
	}
	rs, ok := h.findInlineRuleSet(tag)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("inline rule-set '%s' not found", tag))
		return
	}
	rules, _ := rs["rules"].([]interface{})
	if len(rules) == 0 {
		rules = []interface{}{map[string]interface{}{
			"domain_suffix": []interface{}{domain},
		}}
	} else {
		first, _ := rules[0].(map[string]interface{})
		ds, _ := first["domain_suffix"].([]interface{})
		for _, existing := range ds {
			if s, _ := existing.(string); s == domain {
				writeError(w, http.StatusConflict, fmt.Sprintf("domain '%s' already exists", domain))
				return
			}
		}
		first["domain_suffix"] = append(ds, domain)
		rules[0] = first
	}
	updated := map[string]interface{}{}
	for k, v := range rs {
		updated[k] = v
	}
	updated["rules"] = rules
	if err := h.config.UpdateRuleSet(tag, updated); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeSuccess(w, map[string]string{"message": fmt.Sprintf("domain added to '%s'", tag)})
}

// RemoveDomain strips a domain from any domain_suffix entry; removes empty rules.
func (h *Handler) RemoveDomain(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")
	domain := chi.URLParam(r, "domain")
	rs, ok := h.findInlineRuleSet(tag)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("inline rule-set '%s' not found", tag))
		return
	}
	rules, _ := rs["rules"].([]interface{})
	newRules := make([]interface{}, 0, len(rules))
	for _, raw := range rules {
		rule, _ := raw.(map[string]interface{})
		ds, _ := rule["domain_suffix"].([]interface{})
		filtered := make([]interface{}, 0, len(ds))
		for _, e := range ds {
			if s, _ := e.(string); s != domain {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) > 0 {
			rule["domain_suffix"] = filtered
		} else {
			delete(rule, "domain_suffix")
		}
		if len(rule) > 0 {
			newRules = append(newRules, rule)
		}
	}
	updated := map[string]interface{}{}
	for k, v := range rs {
		updated[k] = v
	}
	updated["rules"] = newRules
	if err := h.config.UpdateRuleSet(tag, updated); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeSuccess(w, map[string]string{"message": fmt.Sprintf("domain removed from '%s'", tag)})
}

// ImportDomains bulk-adds domains, skipping duplicates.
func (h *Handler) ImportDomains(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")
	var body struct {
		Domains []string `json:"domains"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}
	if len(body.Domains) == 0 {
		writeError(w, http.StatusBadRequest, "domains list is required")
		return
	}
	rs, ok := h.findInlineRuleSet(tag)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("inline rule-set '%s' not found", tag))
		return
	}
	rules, _ := rs["rules"].([]interface{})
	var first map[string]interface{}
	if len(rules) > 0 {
		first, _ = rules[0].(map[string]interface{})
	} else {
		first = map[string]interface{}{}
		rules = []interface{}{first}
	}
	ds, _ := first["domain_suffix"].([]interface{})
	existing := make(map[string]bool, len(ds))
	for _, e := range ds {
		if s, _ := e.(string); s != "" {
			existing[s] = true
		}
	}
	added := 0
	for _, d := range body.Domains {
		d = strings.TrimSpace(d)
		if d == "" || existing[d] {
			continue
		}
		ds = append(ds, d)
		existing[d] = true
		added++
	}
	first["domain_suffix"] = ds
	rules[0] = first
	updated := map[string]interface{}{}
	for k, v := range rs {
		updated[k] = v
	}
	updated["rules"] = rules
	if err := h.config.UpdateRuleSet(tag, updated); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeSuccess(w, map[string]interface{}{
		"message": fmt.Sprintf("%d domains imported into '%s'", added, tag),
		"added":   added,
	})
}
