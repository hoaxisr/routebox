package config

import "fmt"

// --- DNS section helpers ---

// getDns returns the dns section from working config (draft or active)
func (m *Manager) getDns() map[string]interface{} {
	config := m.getWorkingConfig()
	if dns, ok := config["dns"].(map[string]interface{}); ok {
		return dns
	}
	return make(map[string]interface{})
}

// getDnsArray returns array from dns section at given key (from working config)
func (m *Manager) getDnsArray(key string) []interface{} {
	dns := m.getDns()
	if arr, ok := dns[key].([]interface{}); ok {
		return arr
	}
	return []interface{}{}
}

// --- DNS Servers CRUD (tag-based) ---

// ListDnsServers returns all DNS servers
func (m *Manager) ListDnsServers() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	arr := m.getDnsArray("servers")
	result := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		if obj, ok := item.(map[string]interface{}); ok {
			result = append(result, obj)
		}
	}
	return result
}

// CreateDnsServer adds new DNS server to draft with validation
func (m *Manager) CreateDnsServer(server map[string]interface{}) error {
	errs := validateDnsServer(server, 0)
	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", errs[0])
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	tag, _ := server["tag"].(string)

	arr := m.getDnsArray("servers")
	if findByTag(arr, tag) >= 0 {
		return fmt.Errorf("DNS server with tag '%s' already exists", tag)
	}

	// Validate detour reference if present
	if detour, ok := server["detour"].(string); ok && detour != "" {
		outboundTags := make(map[string]bool)
		for _, ob := range m.getArray("outbounds") {
			if obj, ok := ob.(map[string]interface{}); ok {
				if t, ok := obj["tag"].(string); ok {
					outboundTags[t] = true
				}
			}
		}
		if !outboundTags[detour] {
			return fmt.Errorf("detour outbound '%s' does not exist", detour)
		}
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	dns := m.getDraftDns()
	draftArr := m.getDraftDnsArray("servers")
	dns["servers"] = append(draftArr, server)

	return m.saveDraftToDisk()
}

// UpdateDnsServer updates existing DNS server in draft with validation
func (m *Manager) UpdateDnsServer(tag string, server map[string]interface{}) error {
	errs := validateDnsServer(server, 0)
	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", errs[0])
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	arr := m.getDnsArray("servers")
	idx := findByTag(arr, tag)
	if idx < 0 {
		return fmt.Errorf("DNS server '%s' not found", tag)
	}

	// Validate detour reference if present
	if detour, ok := server["detour"].(string); ok && detour != "" {
		outboundTags := make(map[string]bool)
		for _, ob := range m.getArray("outbounds") {
			if obj, ok := ob.(map[string]interface{}); ok {
				if t, ok := obj["tag"].(string); ok {
					outboundTags[t] = true
				}
			}
		}
		if !outboundTags[detour] {
			return fmt.Errorf("detour outbound '%s' does not exist", detour)
		}
	}

	newTag, _ := server["tag"].(string)
	if newTag == "" {
		server["tag"] = tag
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	dns := m.getDraftDns()
	draftArr := m.getDraftDnsArray("servers")
	draftIdx := findByTag(draftArr, tag)
	if draftIdx >= 0 {
		draftArr[draftIdx] = server
		dns["servers"] = draftArr
	}

	return m.saveDraftToDisk()
}

// DeleteDnsServer removes DNS server by tag from draft
func (m *Manager) DeleteDnsServer(tag string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	dns := m.getDns()
	arr := m.getDnsArray("servers")
	idx := findByTag(arr, tag)
	if idx < 0 {
		return fmt.Errorf("DNS server '%s' not found", tag)
	}

	// Check if any DNS rule references this server
	rules := m.getDnsArray("rules")
	for i, rule := range rules {
		if obj, ok := rule.(map[string]interface{}); ok {
			if serverTag, ok := obj["server"].(string); ok && serverTag == tag {
				return fmt.Errorf("cannot delete DNS server '%s': referenced by dns.rules[%d]", tag, i)
			}
		}
	}

	// Check if it's the final server
	if final, ok := dns["final"].(string); ok && final == tag {
		return fmt.Errorf("cannot delete DNS server '%s': it is the default (final) server", tag)
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	draftDns := m.getDraftDns()
	draftArr := m.getDraftDnsArray("servers")
	draftIdx := findByTag(draftArr, tag)
	if draftIdx >= 0 {
		draftDns["servers"] = append(draftArr[:draftIdx], draftArr[draftIdx+1:]...)
	}

	return m.saveDraftToDisk()
}

// --- DNS Rules CRUD (index-based) ---

// ListDnsRules returns all DNS rules in order
func (m *Manager) ListDnsRules() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	arr := m.getDnsArray("rules")
	result := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		if obj, ok := item.(map[string]interface{}); ok {
			result = append(result, obj)
		}
	}
	return result
}

// CreateDnsRule appends new DNS rule to draft with validation
func (m *Manager) CreateDnsRule(rule map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Collect valid server tags
	serverTags := make(map[string]bool)
	for _, s := range m.getDnsArray("servers") {
		if obj, ok := s.(map[string]interface{}); ok {
			if t, ok := obj["tag"].(string); ok {
				serverTags[t] = true
			}
		}
	}

	// Collect valid rule_set tags
	ruleSetTags := make(map[string]bool)
	for _, rs := range m.getRouteArray("rule_set") {
		if obj, ok := rs.(map[string]interface{}); ok {
			if t, ok := obj["tag"].(string); ok {
				ruleSetTags[t] = true
			}
		}
	}

	errs := validateDnsRule(rule, 0, serverTags, ruleSetTags)
	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", errs[0])
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	dns := m.getDraftDns()
	draftArr := m.getDraftDnsArray("rules")
	dns["rules"] = append(draftArr, rule)

	return m.saveDraftToDisk()
}

// UpdateDnsRule updates DNS rule at index in draft with validation
func (m *Manager) UpdateDnsRule(index int, rule map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	arr := m.getDnsArray("rules")
	if index < 0 || index >= len(arr) {
		return fmt.Errorf("DNS rule index %d out of range", index)
	}

	// Collect valid server tags
	serverTags := make(map[string]bool)
	for _, s := range m.getDnsArray("servers") {
		if obj, ok := s.(map[string]interface{}); ok {
			if t, ok := obj["tag"].(string); ok {
				serverTags[t] = true
			}
		}
	}

	// Collect valid rule_set tags
	ruleSetTags := make(map[string]bool)
	for _, rs := range m.getRouteArray("rule_set") {
		if obj, ok := rs.(map[string]interface{}); ok {
			if t, ok := obj["tag"].(string); ok {
				ruleSetTags[t] = true
			}
		}
	}

	errs := validateDnsRule(rule, index, serverTags, ruleSetTags)
	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", errs[0])
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	dns := m.getDraftDns()
	draftArr := m.getDraftDnsArray("rules")
	if index < len(draftArr) {
		draftArr[index] = rule
		dns["rules"] = draftArr
	}

	return m.saveDraftToDisk()
}

// DeleteDnsRule removes DNS rule at index from draft
func (m *Manager) DeleteDnsRule(index int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	arr := m.getDnsArray("rules")
	if index < 0 || index >= len(arr) {
		return fmt.Errorf("DNS rule index %d out of range", index)
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	dns := m.getDraftDns()
	draftArr := m.getDraftDnsArray("rules")
	if index < len(draftArr) {
		dns["rules"] = append(draftArr[:index], draftArr[index+1:]...)
	}

	return m.saveDraftToDisk()
}

// ReorderDnsRules moves DNS rule from one index to another in draft
func (m *Manager) ReorderDnsRules(from, to int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	arr := m.getDnsArray("rules")
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
	dns := m.getDraftDns()
	draftArr := m.getDraftDnsArray("rules")

	rule := draftArr[from]
	draftArr = append(draftArr[:from], draftArr[from+1:]...)
	if to > from {
		to--
	}
	newArr := make([]interface{}, 0, len(draftArr)+1)
	newArr = append(newArr, draftArr[:to]...)
	newArr = append(newArr, rule)
	newArr = append(newArr, draftArr[to:]...)

	dns["rules"] = newArr

	return m.saveDraftToDisk()
}

// --- DNS Settings ---

// GetDnsSettings returns DNS configuration settings
func (m *Manager) GetDnsSettings() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	dns := m.getDns()
	result := make(map[string]interface{})

	if strategy, ok := dns["strategy"].(string); ok {
		result["strategy"] = strategy
	}
	if final, ok := dns["final"].(string); ok {
		result["final"] = final
	}
	if disableCache, ok := dns["disable_cache"].(bool); ok {
		result["disable_cache"] = disableCache
	}
	if disableExpire, ok := dns["disable_expire"].(bool); ok {
		result["disable_expire"] = disableExpire
	}
	if independentCache, ok := dns["independent_cache"].(bool); ok {
		result["independent_cache"] = independentCache
	}
	// New fields
	if cacheCapacity, ok := dns["cache_capacity"].(float64); ok {
		result["cache_capacity"] = int(cacheCapacity)
	}
	if reverseMapping, ok := dns["reverse_mapping"].(bool); ok {
		result["reverse_mapping"] = reverseMapping
	}
	if clientSubnet, ok := dns["client_subnet"].(string); ok {
		result["client_subnet"] = clientSubnet
	}

	return result
}

// UpdateDnsSettings updates DNS configuration settings in draft
func (m *Manager) UpdateDnsSettings(settings map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate final server exists
	if final, ok := settings["final"].(string); ok && final != "" {
		serverTags := make(map[string]bool)
		for _, s := range m.getDnsArray("servers") {
			if obj, ok := s.(map[string]interface{}); ok {
				if t, ok := obj["tag"].(string); ok {
					serverTags[t] = true
				}
			}
		}
		if !serverTags[final] {
			return fmt.Errorf("DNS server '%s' does not exist", final)
		}
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	dns := m.getDraftDns()

	if final, ok := settings["final"].(string); ok && final != "" {
		dns["final"] = final
	}
	if strategy, ok := settings["strategy"].(string); ok {
		if strategy != "" {
			dns["strategy"] = strategy
		} else {
			delete(dns, "strategy")
		}
	}
	if disableCache, ok := settings["disable_cache"].(bool); ok {
		dns["disable_cache"] = disableCache
	}
	if disableExpire, ok := settings["disable_expire"].(bool); ok {
		dns["disable_expire"] = disableExpire
	}
	if independentCache, ok := settings["independent_cache"].(bool); ok {
		dns["independent_cache"] = independentCache
	}
	// New fields
	if cacheCapacity, ok := settings["cache_capacity"].(float64); ok {
		if cacheCapacity > 0 {
			dns["cache_capacity"] = int(cacheCapacity)
		} else {
			delete(dns, "cache_capacity")
		}
	}
	if reverseMapping, ok := settings["reverse_mapping"].(bool); ok {
		dns["reverse_mapping"] = reverseMapping
	}
	if clientSubnet, ok := settings["client_subnet"].(string); ok {
		if clientSubnet != "" {
			dns["client_subnet"] = clientSubnet
		} else {
			delete(dns, "client_subnet")
		}
	}

	return m.saveDraftToDisk()
}
