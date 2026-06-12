package config

import "strings"

// ReplaceSubscriptionOutbounds atomically swaps one subscription's outbounds in
// the draft: every outbound with tag == groupTag or prefix nodePrefix is
// removed, then nodes and the urltest group are appended. Creates the draft
// from active if none; saveDraftToDisk bumps draftGen.
func (m *Manager) ReplaceSubscriptionOutbounds(groupTag, nodePrefix string, nodes []map[string]interface{}, group map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}
	kept := m.filterOutSubscriptionOutbounds(groupTag, nodePrefix)
	for _, n := range nodes {
		kept = append(kept, n)
	}
	kept = append(kept, group)
	m.setDraftValue("outbounds", kept)
	return m.saveDraftToDisk()
}

// RemoveSubscriptionOutbounds removes a subscription's group and nodes (Delete).
func (m *Manager) RemoveSubscriptionOutbounds(groupTag, nodePrefix string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}
	m.setDraftValue("outbounds", m.filterOutSubscriptionOutbounds(groupTag, nodePrefix))
	return m.saveDraftToDisk()
}

// filterOutSubscriptionOutbounds returns the draft outbounds minus any whose tag
// == groupTag or has nodePrefix. Caller holds the write lock with a draft ready.
func (m *Manager) filterOutSubscriptionOutbounds(groupTag, nodePrefix string) []interface{} {
	arr := m.getDraftArray("outbounds")
	kept := make([]interface{}, 0, len(arr))
	for _, item := range arr {
		if obj, ok := item.(map[string]interface{}); ok {
			if tag, ok := obj["tag"].(string); ok {
				if tag == groupTag || strings.HasPrefix(tag, nodePrefix) {
					continue
				}
			}
		}
		kept = append(kept, item)
	}
	return kept
}
