package config

import "fmt"

// --- Route section helpers ---

// getRoute returns the route section, creating if needed
func (m *Manager) getRoute() map[string]interface{} {
	if route, ok := m.config["route"].(map[string]interface{}); ok {
		return route
	}
	route := make(map[string]interface{})
	m.config["route"] = route
	return route
}

// getRouteArray returns array from route section at given key
func (m *Manager) getRouteArray(key string) []interface{} {
	route := m.getRoute()
	if arr, ok := route[key].([]interface{}); ok {
		return arr
	}
	return []interface{}{}
}

// --- Rule Sets CRUD (tag-based) ---

// ListRuleSets returns all rule sets
func (m *Manager) ListRuleSets() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	arr := m.getRouteArray("rule_set")
	result := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		if obj, ok := item.(map[string]interface{}); ok {
			result = append(result, obj)
		}
	}
	return result
}

// CreateRuleSet adds new rule set with validation
func (m *Manager) CreateRuleSet(rs map[string]interface{}) error {
	// Validate
	errs := validateRuleSet(rs, 0)
	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", errs[0])
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	tag, _ := rs["tag"].(string)
	route := m.getRoute()

	arr := m.getRouteArray("rule_set")
	if findByTag(arr, tag) >= 0 {
		return fmt.Errorf("rule set with tag '%s' already exists", tag)
	}

	route["rule_set"] = append(arr, rs)
	return nil
}

// DeleteRuleSet removes rule set by tag
func (m *Manager) DeleteRuleSet(tag string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	route := m.getRoute()
	arr := m.getRouteArray("rule_set")
	idx := findByTag(arr, tag)
	if idx < 0 {
		return fmt.Errorf("rule set '%s' not found", tag)
	}

	// Check if any rule references this rule set
	rules := m.getRouteArray("rules")
	for i, rule := range rules {
		if obj, ok := rule.(map[string]interface{}); ok {
			if ruleSets, ok := obj["rule_set"].([]interface{}); ok {
				for _, rs := range ruleSets {
					if rsTag, ok := rs.(string); ok && rsTag == tag {
						return fmt.Errorf("cannot delete rule set '%s': referenced by rule[%d]", tag, i)
					}
				}
			}
		}
	}

	route["rule_set"] = append(arr[:idx], arr[idx+1:]...)
	return nil
}

// --- Route Rules CRUD (index-based) ---

// ListRules returns all route rules in order
func (m *Manager) ListRules() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	arr := m.getRouteArray("rules")
	result := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		if obj, ok := item.(map[string]interface{}); ok {
			result = append(result, obj)
		}
	}
	return result
}

// CreateRule appends new rule with validation
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

	route := m.getRoute()
	arr := m.getRouteArray("rules")
	route["rules"] = append(arr, rule)
	return nil
}

// UpdateRule updates rule at index with validation
func (m *Manager) UpdateRule(index int, rule map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	route := m.getRoute()
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

	arr[index] = rule
	route["rules"] = arr
	return nil
}

// DeleteRule removes rule at index
func (m *Manager) DeleteRule(index int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	route := m.getRoute()
	arr := m.getRouteArray("rules")
	if index < 0 || index >= len(arr) {
		return fmt.Errorf("rule index %d out of range", index)
	}

	route["rules"] = append(arr[:index], arr[index+1:]...)
	return nil
}

// ReorderRules moves rule from one index to another
func (m *Manager) ReorderRules(from, to int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	route := m.getRoute()
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

	// Extract the rule being moved
	rule := arr[from]

	// Remove from original position
	arr = append(arr[:from], arr[from+1:]...)

	// Adjust 'to' if it was after 'from'
	if to > from {
		to--
	}

	// Insert at new position
	newArr := make([]interface{}, 0, len(arr)+1)
	newArr = append(newArr, arr[:to]...)
	newArr = append(newArr, rule)
	newArr = append(newArr, arr[to:]...)

	route["rules"] = newArr
	return nil
}

// --- Route Settings ---

// GetRouteSettings returns final outbound and auto_detect_interface
func (m *Manager) GetRouteSettings() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	route := m.getRoute()
	result := make(map[string]interface{})

	if final, ok := route["final"].(string); ok {
		result["final"] = final
	}
	if autoDetect, ok := route["auto_detect_interface"].(bool); ok {
		result["auto_detect_interface"] = autoDetect
	}

	return result
}

// UpdateRouteSettings updates final outbound and auto_detect_interface
func (m *Manager) UpdateRouteSettings(settings map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	route := m.getRoute()

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
		// AWG/WireGuard endpoints can be used directly as final outbound
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
		route["final"] = final
	}

	if autoDetect, ok := settings["auto_detect_interface"].(bool); ok {
		route["auto_detect_interface"] = autoDetect
	}

	return nil
}
