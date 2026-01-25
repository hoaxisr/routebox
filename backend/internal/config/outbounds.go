package config

import "fmt"

// ListOutbounds returns all outbounds
func (m *Manager) ListOutbounds() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	arr := m.getArray("outbounds")
	result := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		if obj, ok := item.(map[string]interface{}); ok {
			result = append(result, obj)
		}
	}
	return result
}

// GetOutbound returns outbound by tag
func (m *Manager) GetOutbound(tag string) (map[string]interface{}, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	arr := m.getArray("outbounds")
	idx := findByTag(arr, tag)
	if idx < 0 {
		return nil, false
	}
	if obj, ok := arr[idx].(map[string]interface{}); ok {
		return obj, true
	}
	return nil, false
}

// CreateOutbound adds new outbound with validation
func (m *Manager) CreateOutbound(outbound map[string]interface{}) error {
	// Validate outbound before adding
	errs := validateOutbound(outbound, 0)
	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", errs[0])
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	tag := outbound["tag"].(string)

	arr := m.getArray("outbounds")
	if findByTag(arr, tag) >= 0 {
		return fmt.Errorf("outbound with tag '%s' already exists", tag)
	}

	// Reference validation for selector/urltest/endpoint
	endpointTags := make(map[string]bool)
	for _, ep := range m.getArray("endpoints") {
		if obj, ok := ep.(map[string]interface{}); ok {
			if t, ok := obj["tag"].(string); ok {
				endpointTags[t] = true
			}
		}
	}
	outboundTags := make(map[string]bool)
	for _, ob := range arr {
		if obj, ok := ob.(map[string]interface{}); ok {
			if t, ok := obj["tag"].(string); ok {
				outboundTags[t] = true
			}
		}
	}
	refErrs := validateOutboundReferences(outbound, 0, endpointTags, outboundTags)
	if len(refErrs) > 0 {
		return fmt.Errorf("reference validation failed: %s", refErrs[0])
	}

	m.config["outbounds"] = append(arr, outbound)
	return nil
}

// UpdateOutbound updates existing outbound with validation
func (m *Manager) UpdateOutbound(tag string, outbound map[string]interface{}) error {
	// Validate outbound before updating
	errs := validateOutbound(outbound, 0)
	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", errs[0])
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	arr := m.getArray("outbounds")
	idx := findByTag(arr, tag)
	if idx < 0 {
		return fmt.Errorf("outbound '%s' not found", tag)
	}

	newTag, _ := outbound["tag"].(string)
	if newTag == "" {
		outbound["tag"] = tag
	}

	// Reference validation
	endpointTags := make(map[string]bool)
	for _, ep := range m.getArray("endpoints") {
		if obj, ok := ep.(map[string]interface{}); ok {
			if t, ok := obj["tag"].(string); ok {
				endpointTags[t] = true
			}
		}
	}
	outboundTags := make(map[string]bool)
	for i, ob := range arr {
		if i == idx {
			continue // skip current being updated
		}
		if obj, ok := ob.(map[string]interface{}); ok {
			if t, ok := obj["tag"].(string); ok {
				outboundTags[t] = true
			}
		}
	}
	refErrs := validateOutboundReferences(outbound, 0, endpointTags, outboundTags)
	if len(refErrs) > 0 {
		return fmt.Errorf("reference validation failed: %s", refErrs[0])
	}

	arr[idx] = outbound
	m.config["outbounds"] = arr
	return nil
}

// DeleteOutbound removes outbound by tag
func (m *Manager) DeleteOutbound(tag string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	arr := m.getArray("outbounds")
	idx := findByTag(arr, tag)
	if idx < 0 {
		return fmt.Errorf("outbound '%s' not found", tag)
	}

	m.config["outbounds"] = append(arr[:idx], arr[idx+1:]...)
	return nil
}
