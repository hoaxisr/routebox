package config

import "fmt"

// --- Route section helpers ---

// getRoute returns the route section from working config (draft or active)
func (m *Manager) getRoute() map[string]interface{} {
	config := m.getWorkingConfig()
	if route, ok := config["route"].(map[string]interface{}); ok {
		return route
	}
	return make(map[string]interface{})
}

// getRouteArray returns array from route section at given key (from working config)
func (m *Manager) getRouteArray(key string) []interface{} {
	route := m.getRoute()
	if arr, ok := route[key].([]interface{}); ok {
		return arr
	}
	return []interface{}{}
}

// hasTunAutoRedirect checks if any TUN inbound in the working config has auto_redirect enabled.
// Must be called with m.mu held.
func (m *Manager) hasTunAutoRedirect() bool {
	for _, ib := range m.getArray("inbounds") {
		if obj, ok := ib.(map[string]interface{}); ok {
			if ibType, _ := obj["type"].(string); ibType == "tun" {
				if ar, ok := obj["auto_redirect"].(bool); ok && ar {
					return true
				}
			}
		}
	}
	return false
}

// --- Rule Sets CRUD (tag-based) ---

// ListRuleSets returns all rule sets (deep-copied, like Get)
func (m *Manager) ListRuleSets() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	arr := m.getRouteArray("rule_set")
	result := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		if obj, ok := item.(map[string]interface{}); ok {
			result = append(result, m.deepCopy(obj))
		}
	}
	return result
}

// CreateRuleSet adds new rule set to draft with validation
func (m *Manager) CreateRuleSet(rs map[string]interface{}) error {
	// Validate
	errs := validateRuleSet(rs, 0)
	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", errs[0])
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	tag, _ := rs["tag"].(string)

	arr := m.getRouteArray("rule_set")
	if findByTag(arr, tag) >= 0 {
		return fmt.Errorf("rule set with tag '%s' already exists", tag)
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	route := m.getDraftRoute()
	draftArr := m.getDraftRouteArray("rule_set")
	route["rule_set"] = append(draftArr, rs)

	return m.saveDraftToDisk()
}

// DeleteRuleSet removes rule set by tag from draft
func (m *Manager) DeleteRuleSet(tag string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	arr := m.getRouteArray("rule_set")
	idx := findByTag(arr, tag)
	if idx < 0 {
		return fmt.Errorf("rule set '%s' not found", tag)
	}

	// Check if any route rule references this rule set
	rules := m.getRouteArray("rules")
	for i, rule := range rules {
		if obj, ok := rule.(map[string]interface{}); ok {
			if ruleSets, ok := obj["rule_set"].([]interface{}); ok {
				for _, rs := range ruleSets {
					if rsTag, ok := rs.(string); ok && rsTag == tag {
						return fmt.Errorf("cannot delete rule set '%s': referenced by route rule[%d]", tag, i)
					}
				}
			}
		}
	}

	// Check if any DNS rule references this rule set
	dnsRules := m.getDnsArray("rules")
	for i, rule := range dnsRules {
		if obj, ok := rule.(map[string]interface{}); ok {
			if ruleSets, ok := obj["rule_set"].([]interface{}); ok {
				for _, rs := range ruleSets {
					if rsTag, ok := rs.(string); ok && rsTag == tag {
						return fmt.Errorf("cannot delete rule set '%s': referenced by dns rule[%d]", tag, i)
					}
				}
			}
		}
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	route := m.getDraftRoute()
	draftArr := m.getDraftRouteArray("rule_set")
	draftIdx := findByTag(draftArr, tag)
	if draftIdx >= 0 {
		route["rule_set"] = append(draftArr[:draftIdx], draftArr[draftIdx+1:]...)
	}

	return m.saveDraftToDisk()
}

// UpdateRuleSet updates an existing rule set by tag in draft
func (m *Manager) UpdateRuleSet(tag string, rs map[string]interface{}) error {
	// Validate
	errs := validateRuleSet(rs, 0)
	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", errs[0])
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	arr := m.getRouteArray("rule_set")
	idx := findByTag(arr, tag)
	if idx < 0 {
		return fmt.Errorf("rule set '%s' not found", tag)
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft — replace the rule set at the same index
	route := m.getDraftRoute()
	draftArr := m.getDraftRouteArray("rule_set")
	draftIdx := findByTag(draftArr, tag)
	if draftIdx < 0 {
		return fmt.Errorf("rule set '%s' not found in draft", tag)
	}

	// Preserve the tag (don't allow tag changes through this endpoint)
	rs["tag"] = tag
	draftArr[draftIdx] = rs
	route["rule_set"] = draftArr

	return m.saveDraftToDisk()
}

// --- Route Rules CRUD (index-based) ---

// ListRules returns all route rules in order (deep-copied, like Get)
func (m *Manager) ListRules() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	arr := m.getRouteArray("rules")
	result := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		if obj, ok := item.(map[string]interface{}); ok {
			result = append(result, m.deepCopy(obj))
		}
	}
	return result
}

// CreateRule appends new rule to draft with validation
func (m *Manager) CreateRule(rule map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Collect valid references
	outboundTags := make(map[string]bool)
	for _, ob := range m.getArray("outbounds") {
		if obj, ok := ob.(map[string]interface{}); ok {
			if t, ok := obj["tag"].(string); ok {
				outboundTags[t] = true
			}
		}
	}
	// AWG/WireGuard endpoints can be used as outbounds in routes
	endpointTags := make(map[string]bool)
	for _, ep := range m.getArray("endpoints") {
		if obj, ok := ep.(map[string]interface{}); ok {
			if t, ok := obj["tag"].(string); ok {
				endpointTags[t] = true
			}
		}
	}
	ruleSetTags := make(map[string]bool)
	for _, rs := range m.getRouteArray("rule_set") {
		if obj, ok := rs.(map[string]interface{}); ok {
			if t, ok := obj["tag"].(string); ok {
				ruleSetTags[t] = true
			}
		}
	}

	errs := validateRule(rule, 0, outboundTags, endpointTags, ruleSetTags)
	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", errs[0])
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	route := m.getDraftRoute()
	draftArr := m.getDraftRouteArray("rules")
	route["rules"] = append(draftArr, rule)

	return m.saveDraftToDisk()
}

// UpdateRule updates rule at index in draft with validation
func (m *Manager) UpdateRule(index int, rule map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	arr := m.getRouteArray("rules")
	if index < 0 || index >= len(arr) {
		return fmt.Errorf("rule index %d out of range", index)
	}

	// Collect valid references
	outboundTags := make(map[string]bool)
	for _, ob := range m.getArray("outbounds") {
		if obj, ok := ob.(map[string]interface{}); ok {
			if t, ok := obj["tag"].(string); ok {
				outboundTags[t] = true
			}
		}
	}
	// AWG/WireGuard endpoints can be used as outbounds in routes
	endpointTags := make(map[string]bool)
	for _, ep := range m.getArray("endpoints") {
		if obj, ok := ep.(map[string]interface{}); ok {
			if t, ok := obj["tag"].(string); ok {
				endpointTags[t] = true
			}
		}
	}
	ruleSetTags := make(map[string]bool)
	for _, rs := range m.getRouteArray("rule_set") {
		if obj, ok := rs.(map[string]interface{}); ok {
			if t, ok := obj["tag"].(string); ok {
				ruleSetTags[t] = true
			}
		}
	}

	errs := validateRule(rule, index, outboundTags, endpointTags, ruleSetTags)
	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", errs[0])
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	route := m.getDraftRoute()
	draftArr := m.getDraftRouteArray("rules")
	if index < len(draftArr) {
		draftArr[index] = rule
		route["rules"] = draftArr
	}

	return m.saveDraftToDisk()
}

// DeleteRule removes rule at index from draft
func (m *Manager) DeleteRule(index int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	arr := m.getRouteArray("rules")
	if index < 0 || index >= len(arr) {
		return fmt.Errorf("rule index %d out of range", index)
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	route := m.getDraftRoute()
	draftArr := m.getDraftRouteArray("rules")
	if index < len(draftArr) {
		route["rules"] = append(draftArr[:index], draftArr[index+1:]...)
	}

	return m.saveDraftToDisk()
}

// ReorderRules moves rule from one index to another in draft
func (m *Manager) ReorderRules(from, to int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	arr := m.getRouteArray("rules")
	if from < 0 || from >= len(arr) {
		return fmt.Errorf("'from' index %d out of range", from)
	}
	if to < 0 || to >= len(arr) {
		return fmt.Errorf("'to' index %d out of range", to)
	}
	if from == to {
		return nil
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	route := m.getDraftRoute()
	draftArr := m.getDraftRouteArray("rules")

	// Extract the rule being moved
	rule := draftArr[from]

	// Remove from original position
	draftArr = append(draftArr[:from], draftArr[from+1:]...)

	// 'to' is the index the rule ENDS UP at, so it needs no adjustment after the
	// removal. Decrementing it for downward moves (as this used to) made every
	// such drag land one slot short — "drag one down" became a silent no-op —
	// and made the last position unreachable by any input, while the panel's
	// optimistic update and drop indicator both meant the plain reading.

	// Insert at new position
	newArr := make([]interface{}, 0, len(draftArr)+1)
	newArr = append(newArr, draftArr[:to]...)
	newArr = append(newArr, rule)
	newArr = append(newArr, draftArr[to:]...)

	route["rules"] = newArr

	return m.saveDraftToDisk()
}

// --- Route Settings ---

// routeSettingsKeys lists all route-level settings fields we read/write
var routeSettingsKeys = []string{
	"final", "auto_detect_interface",
	"default_interface", "default_mark",
	"default_domain_resolver",
	"default_domain_strategy",
	"default_network_strategy", "default_network_type",
	"default_fallback_network_type", "default_fallback_delay",
}

// GetRouteSettings returns route-level settings
func (m *Manager) GetRouteSettings() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	route := m.getRoute()
	result := make(map[string]interface{})

	for _, key := range routeSettingsKeys {
		if val, ok := route[key]; ok {
			result[key] = val
		}
	}

	return result
}

// UpdateRouteSettings merges provided settings into draft (null-safe: only overwrites keys present in payload)
func (m *Manager) UpdateRouteSettings(settings map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate final outbound exists (can be an outbound or endpoint tag)
	if final, ok := settings["final"].(string); ok && final != "" {
		outboundTags := make(map[string]bool)
		for _, ob := range m.getArray("outbounds") {
			if obj, ok := ob.(map[string]interface{}); ok {
				if t, ok := obj["tag"].(string); ok {
					outboundTags[t] = true
				}
			}
		}
		endpointTags := make(map[string]bool)
		for _, ep := range m.getArray("endpoints") {
			if obj, ok := ep.(map[string]interface{}); ok {
				if t, ok := obj["tag"].(string); ok {
					endpointTags[t] = true
				}
			}
		}
		if !outboundTags[final] && !endpointTags[final] {
			return fmt.Errorf("final outbound '%s' does not exist", final)
		}
	}

	// Validate default_domain_strategy if provided
	if strategy, ok := settings["default_domain_strategy"].(string); ok && strategy != "" {
		valid := map[string]bool{"prefer_ipv4": true, "prefer_ipv6": true, "ipv4_only": true, "ipv6_only": true}
		if !valid[strategy] {
			return fmt.Errorf("invalid default_domain_strategy: %s", strategy)
		}
	}

	// Validate default_network_strategy if provided
	if strategy, ok := settings["default_network_strategy"].(string); ok && strategy != "" {
		valid := map[string]bool{"default": true, "hybrid": true, "fallback": true}
		if !valid[strategy] {
			return fmt.Errorf("invalid default_network_strategy: %s", strategy)
		}
	}

	// Check default_interface + default_network_strategy conflict
	// Need to consider both incoming settings and existing route values
	currentRoute := m.getRoute()
	effectiveInterface := ""
	effectiveStrategy := ""
	if di, ok := settings["default_interface"].(string); ok {
		effectiveInterface = di
	} else if di, ok := currentRoute["default_interface"].(string); ok {
		effectiveInterface = di
	}
	if ns, ok := settings["default_network_strategy"].(string); ok {
		effectiveStrategy = ns
	} else if ns, ok := currentRoute["default_network_strategy"].(string); ok {
		effectiveStrategy = ns
	}
	if effectiveInterface != "" && effectiveStrategy != "" {
		return fmt.Errorf("default_interface conflicts with default_network_strategy — cannot use both simultaneously")
	}

	// Validate default_mark if provided
	if mark, ok := settings["default_mark"]; ok {
		if markNum, ok := mark.(float64); ok && markNum < 0 {
			return fmt.Errorf("default_mark must be >= 0")
		}
		// Check auto_redirect conflict
		if markNum, ok := mark.(float64); ok && markNum != 0 {
			if m.hasTunAutoRedirect() {
				return fmt.Errorf("default_mark conflicts with tun auto_redirect — they use overlapping firewall marks (see sing-box docs)")
			}
		}
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Merge only provided keys into draft (null-safe)
	route := m.getDraftRoute()
	for _, key := range routeSettingsKeys {
		if val, exists := settings[key]; exists {
			if val == nil || val == "" {
				delete(route, key)
			} else {
				route[key] = val
			}
		}
	}

	return m.saveDraftToDisk()
}

// --- Rule Sets Usage ---

// GetRuleSetsUsage scans route.rules and dns.rules to find which rules reference each rule set tag.
// Returns a map: { "tag": { "route_rules": [0, 3], "dns_rules": [1] }, ... }
func (m *Manager) GetRuleSetsUsage() map[string]map[string][]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	usage := make(map[string]map[string][]int)

	// Initialize entries for all known rule set tags
	for _, item := range m.getRouteArray("rule_set") {
		if obj, ok := item.(map[string]interface{}); ok {
			if tag, ok := obj["tag"].(string); ok {
				usage[tag] = map[string][]int{
					"route_rules": {},
					"dns_rules":   {},
				}
			}
		}
	}

	// Scan route rules
	routeRules := m.getRouteArray("rules")
	for i, rule := range routeRules {
		if obj, ok := rule.(map[string]interface{}); ok {
			if ruleSets, ok := obj["rule_set"].([]interface{}); ok {
				for _, rs := range ruleSets {
					if rsTag, ok := rs.(string); ok {
						if entry, exists := usage[rsTag]; exists {
							entry["route_rules"] = append(entry["route_rules"], i)
						}
					}
				}
			}
		}
	}

	// Scan DNS rules
	dnsRules := m.getDnsArray("rules")
	for i, rule := range dnsRules {
		if obj, ok := rule.(map[string]interface{}); ok {
			if ruleSets, ok := obj["rule_set"].([]interface{}); ok {
				for _, rs := range ruleSets {
					if rsTag, ok := rs.(string); ok {
						if entry, exists := usage[rsTag]; exists {
							entry["dns_rules"] = append(entry["dns_rules"], i)
						}
					}
				}
			}
		}
	}

	return usage
}
