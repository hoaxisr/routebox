package config

import "fmt"

// --- Log Settings ---

// getLog returns the log section from working config (draft or active)
func (m *Manager) getLog() map[string]interface{} {
	config := m.getWorkingConfig()
	if log, ok := config["log"].(map[string]interface{}); ok {
		return log
	}
	return make(map[string]interface{})
}

// GetLogSettings returns log configuration settings
func (m *Manager) GetLogSettings() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	log := m.getLog()
	result := make(map[string]interface{})

	if level, ok := log["level"].(string); ok {
		result["level"] = level
	} else {
		result["level"] = "info" // default
	}
	if timestamp, ok := log["timestamp"].(bool); ok {
		result["timestamp"] = timestamp
	}
	if output, ok := log["output"].(string); ok {
		result["output"] = output
	}

	return result
}

// UpdateLogSettings updates log configuration settings in draft
func (m *Manager) UpdateLogSettings(settings map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate level
	if level, ok := settings["level"].(string); ok {
		validLevels := map[string]bool{
			"trace": true, "debug": true, "info": true, "warn": true,
			"error": true, "fatal": true, "panic": true,
		}
		if !validLevels[level] {
			return fmt.Errorf("invalid log level '%s'", level)
		}
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	log := m.getDraftLog()

	if level, ok := settings["level"].(string); ok {
		log["level"] = level
	}

	if timestamp, ok := settings["timestamp"].(bool); ok {
		log["timestamp"] = timestamp
	} else {
		delete(log, "timestamp")
	}

	if output, ok := settings["output"].(string); ok && output != "" {
		log["output"] = output
	} else {
		delete(log, "output")
	}

	return m.saveDraftToDisk()
}
