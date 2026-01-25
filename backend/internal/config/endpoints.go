package config

import "fmt"

// ListEndpoints returns all endpoints from the working config (draft or active)
func (m *Manager) ListEndpoints() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	arr := m.getArray("endpoints")
	result := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		if obj, ok := item.(map[string]interface{}); ok {
			result = append(result, obj)
		}
	}
	return result
}

// GetEndpoint returns endpoint by tag from the working config (draft or active)
func (m *Manager) GetEndpoint(tag string) (map[string]interface{}, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	arr := m.getArray("endpoints")
	idx := findByTag(arr, tag)
	if idx < 0 {
		return nil, false
	}
	if obj, ok := arr[idx].(map[string]interface{}); ok {
		return obj, true
	}
	return nil, false
}

// CreateEndpoint adds new endpoint to draft with validation
func (m *Manager) CreateEndpoint(endpoint map[string]interface{}) error {
	// Validate endpoint before adding
	errs := validateEndpoint(endpoint, 0)
	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", errs[0])
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	tag := endpoint["tag"].(string)

	// Check in working config first
	arr := m.getArray("endpoints")
	if findByTag(arr, tag) >= 0 {
		return fmt.Errorf("endpoint with tag '%s' already exists", tag)
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	draftArr := m.getDraftArray("endpoints")
	m.setDraftValue("endpoints", append(draftArr, endpoint))

	// Save draft to disk
	return m.saveDraftToDisk()
}

// UpdateEndpoint updates existing endpoint in draft with validation
func (m *Manager) UpdateEndpoint(tag string, endpoint map[string]interface{}) error {
	// Validate endpoint before updating
	errs := validateEndpoint(endpoint, 0)
	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", errs[0])
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check in working config first
	arr := m.getArray("endpoints")
	idx := findByTag(arr, tag)
	if idx < 0 {
		return fmt.Errorf("endpoint '%s' not found", tag)
	}

	// Update tag in endpoint if different
	newTag, _ := endpoint["tag"].(string)
	if newTag == "" {
		endpoint["tag"] = tag
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	draftArr := m.getDraftArray("endpoints")
	draftIdx := findByTag(draftArr, tag)
	if draftIdx >= 0 {
		draftArr[draftIdx] = endpoint
		m.setDraftValue("endpoints", draftArr)
	}

	// Save draft to disk
	return m.saveDraftToDisk()
}

// DeleteEndpoint removes endpoint by tag from draft
func (m *Manager) DeleteEndpoint(tag string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check in working config first
	arr := m.getArray("endpoints")
	idx := findByTag(arr, tag)
	if idx < 0 {
		return fmt.Errorf("endpoint '%s' not found", tag)
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	draftArr := m.getDraftArray("endpoints")
	draftIdx := findByTag(draftArr, tag)
	if draftIdx >= 0 {
		m.setDraftValue("endpoints", append(draftArr[:draftIdx], draftArr[draftIdx+1:]...))
	}

	// Save draft to disk
	return m.saveDraftToDisk()
}
