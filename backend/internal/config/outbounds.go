package config

import "fmt"

// ListOutbounds returns all outbounds from the working config (draft or active)
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

// GetOutbound returns outbound by tag from the working config (draft or active)
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

// CreateOutbound adds new outbound to draft with validation
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

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	draftArr := m.getDraftArray("outbounds")
	m.setDraftValue("outbounds", append(draftArr, outbound))

	return m.saveDraftToDisk()
}

// UpdateOutbound updates existing outbound in draft with validation
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

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	draftArr := m.getDraftArray("outbounds")
	draftIdx := findByTag(draftArr, tag)
	if draftIdx >= 0 {
		draftArr[draftIdx] = outbound
		m.setDraftValue("outbounds", draftArr)
	}

	return m.saveDraftToDisk()
}

// DeleteOutbound removes outbound by tag from draft
func (m *Manager) DeleteOutbound(tag string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	arr := m.getArray("outbounds")
	idx := findByTag(arr, tag)
	if idx < 0 {
		return fmt.Errorf("outbound '%s' not found", tag)
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	draftArr := m.getDraftArray("outbounds")
	draftIdx := findByTag(draftArr, tag)
	if draftIdx >= 0 {
		m.setDraftValue("outbounds", append(draftArr[:draftIdx], draftArr[draftIdx+1:]...))
	}

	return m.saveDraftToDisk()
}
