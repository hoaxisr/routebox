package config

import "fmt"

// ListInbounds returns all inbounds
func (m *Manager) ListInbounds() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	arr := m.getArray("inbounds")
	result := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		if obj, ok := item.(map[string]interface{}); ok {
			result = append(result, obj)
		}
	}
	return result
}

// GetInbound returns inbound by tag
func (m *Manager) GetInbound(tag string) (map[string]interface{}, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	arr := m.getArray("inbounds")
	idx := findByTag(arr, tag)
	if idx < 0 {
		return nil, false
	}
	if obj, ok := arr[idx].(map[string]interface{}); ok {
		return obj, true
	}
	return nil, false
}

// CreateInbound adds new inbound with validation
func (m *Manager) CreateInbound(inbound map[string]interface{}) error {
	// Validate inbound before adding
	errs := validateInbound(inbound, 0)
	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", errs[0])
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	tag := inbound["tag"].(string)

	arr := m.getArray("inbounds")
	if findByTag(arr, tag) >= 0 {
		return fmt.Errorf("inbound with tag '%s' already exists", tag)
	}

	m.config["inbounds"] = append(arr, inbound)
	return nil
}

// UpdateInbound updates existing inbound with validation
func (m *Manager) UpdateInbound(tag string, inbound map[string]interface{}) error {
	// Validate inbound before updating
	errs := validateInbound(inbound, 0)
	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", errs[0])
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	arr := m.getArray("inbounds")
	idx := findByTag(arr, tag)
	if idx < 0 {
		return fmt.Errorf("inbound '%s' not found", tag)
	}

	newTag, _ := inbound["tag"].(string)
	if newTag == "" {
		inbound["tag"] = tag
	}

	arr[idx] = inbound
	m.config["inbounds"] = arr
	return nil
}

// DeleteInbound removes inbound by tag
func (m *Manager) DeleteInbound(tag string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	arr := m.getArray("inbounds")
	idx := findByTag(arr, tag)
	if idx < 0 {
		return fmt.Errorf("inbound '%s' not found", tag)
	}

	m.config["inbounds"] = append(arr[:idx], arr[idx+1:]...)
	return nil
}
