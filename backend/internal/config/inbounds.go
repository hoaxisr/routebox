package config

import "fmt"

// ListInbounds returns all inbounds from the working config (draft or active)
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

// GetInbound returns inbound by tag from the working config (draft or active)
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

// CreateInbound adds new inbound to draft with validation
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

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	draftArr := m.getDraftArray("inbounds")
	m.setDraftValue("inbounds", append(draftArr, inbound))

	return m.saveDraftToDisk()
}

// UpdateInbound updates existing inbound in draft with validation
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

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	draftArr := m.getDraftArray("inbounds")
	draftIdx := findByTag(draftArr, tag)
	if draftIdx >= 0 {
		draftArr[draftIdx] = inbound
		m.setDraftValue("inbounds", draftArr)
	}

	return m.saveDraftToDisk()
}

// MutateInbound stages a change to the named inbound atomically under the write
// lock: it ensures a draft exists, applies fn to a deep copy of the inbound (so
// the live active config is never mutated), writes the result into the draft,
// and persists. fn receives a private deep copy it may mutate freely; returning
// an error aborts and persists no further change (the draft is left as-is,
// matching UpdateInbound's behavior on its own error paths).
//
// This is the single-critical-section equivalent of GetInbound+UpdateInbound:
// two concurrent MutateInbound calls on the same inbound serialize, so neither
// can lose the other's update (no get-clone-update race across two locks).
func (m *Manager) MutateInbound(tag string, fn func(inbound map[string]interface{}) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure draft exists before modifying (snapshots active→draft once).
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	draftArr := m.getDraftArray("inbounds")
	idx := findByTag(draftArr, tag)
	if idx < 0 {
		return fmt.Errorf("inbound %q: %w", tag, ErrInboundNotFound)
	}
	orig, _ := draftArr[idx].(map[string]interface{})

	// Operate on a deep copy so fn cannot mutate the (possibly active-aliased)
	// original in place; only the staged clone is written back to the draft.
	clone := m.deepCopy(orig)
	if err := fn(clone); err != nil {
		return err // abort: do not persist the change
	}

	draftArr[idx] = clone
	m.setDraftValue("inbounds", draftArr)

	return m.saveDraftToDisk()
}

// DeleteInbound removes inbound by tag from draft
func (m *Manager) DeleteInbound(tag string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	arr := m.getArray("inbounds")
	idx := findByTag(arr, tag)
	if idx < 0 {
		return fmt.Errorf("inbound '%s' not found", tag)
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	draftArr := m.getDraftArray("inbounds")
	draftIdx := findByTag(draftArr, tag)
	if draftIdx >= 0 {
		m.setDraftValue("inbounds", append(draftArr[:draftIdx], draftArr[draftIdx+1:]...))
	}

	return m.saveDraftToDisk()
}
