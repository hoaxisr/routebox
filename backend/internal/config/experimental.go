package config

import "fmt"

// --- Experimental Settings ---

// getExperimental returns the experimental section, creating if needed
func (m *Manager) getExperimental() map[string]interface{} {
	if exp, ok := m.config["experimental"].(map[string]interface{}); ok {
		return exp
	}
	exp := make(map[string]interface{})
	m.config["experimental"] = exp
	return exp
}

// GetExperimental returns experimental configuration
func (m *Manager) GetExperimental() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	exp := m.getExperimental()
	result := make(map[string]interface{})

	// Cache file settings
	if cacheFile, ok := exp["cache_file"].(map[string]interface{}); ok {
		cf := make(map[string]interface{})
		if enabled, ok := cacheFile["enabled"].(bool); ok {
			cf["enabled"] = enabled
		}
		if path, ok := cacheFile["path"].(string); ok {
			cf["path"] = path
		}
		if cacheId, ok := cacheFile["cache_id"].(string); ok {
			cf["cache_id"] = cacheId
		}
		if storeFakeip, ok := cacheFile["store_fakeip"].(bool); ok {
			cf["store_fakeip"] = storeFakeip
		}
		if storeRdrc, ok := cacheFile["store_rdrc"].(bool); ok {
			cf["store_rdrc"] = storeRdrc
		}
		result["cache_file"] = cf
	}

	// Clash API settings
	if clashApi, ok := exp["clash_api"].(map[string]interface{}); ok {
		ca := make(map[string]interface{})
		if controller, ok := clashApi["external_controller"].(string); ok {
			ca["external_controller"] = controller
		}
		if ui, ok := clashApi["external_ui"].(string); ok {
			ca["external_ui"] = ui
		}
		if downloadUrl, ok := clashApi["external_ui_download_url"].(string); ok {
			ca["external_ui_download_url"] = downloadUrl
		}
		if downloadDetour, ok := clashApi["external_ui_download_detour"].(string); ok {
			ca["external_ui_download_detour"] = downloadDetour
		}
		if secret, ok := clashApi["secret"].(string); ok {
			ca["secret"] = secret
		}
		if defaultMode, ok := clashApi["default_mode"].(string); ok {
			ca["default_mode"] = defaultMode
		}
		result["clash_api"] = ca
	}

	return result
}

// UpdateExperimental updates experimental configuration
func (m *Manager) UpdateExperimental(settings map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	exp := m.getExperimental()

	// Update cache_file settings
	if cacheFileInput, ok := settings["cache_file"].(map[string]interface{}); ok {
		cacheFile := make(map[string]interface{})

		if enabled, ok := cacheFileInput["enabled"].(bool); ok {
			cacheFile["enabled"] = enabled
		}
		if path, ok := cacheFileInput["path"].(string); ok && path != "" {
			cacheFile["path"] = path
		}
		if cacheId, ok := cacheFileInput["cache_id"].(string); ok && cacheId != "" {
			cacheFile["cache_id"] = cacheId
		}
		if storeFakeip, ok := cacheFileInput["store_fakeip"].(bool); ok {
			cacheFile["store_fakeip"] = storeFakeip
		}
		if storeRdrc, ok := cacheFileInput["store_rdrc"].(bool); ok {
			cacheFile["store_rdrc"] = storeRdrc
		}

		if len(cacheFile) > 0 {
			exp["cache_file"] = cacheFile
		} else {
			delete(exp, "cache_file")
		}
	}

	// Update clash_api settings
	if clashApiInput, ok := settings["clash_api"].(map[string]interface{}); ok {
		clashApi := make(map[string]interface{})

		if controller, ok := clashApiInput["external_controller"].(string); ok && controller != "" {
			clashApi["external_controller"] = controller
		}
		if ui, ok := clashApiInput["external_ui"].(string); ok && ui != "" {
			clashApi["external_ui"] = ui
		}
		if downloadUrl, ok := clashApiInput["external_ui_download_url"].(string); ok && downloadUrl != "" {
			clashApi["external_ui_download_url"] = downloadUrl
		}
		if downloadDetour, ok := clashApiInput["external_ui_download_detour"].(string); ok && downloadDetour != "" {
			clashApi["external_ui_download_detour"] = downloadDetour
		}
		if secret, ok := clashApiInput["secret"].(string); ok {
			clashApi["secret"] = secret
		}
		if defaultMode, ok := clashApiInput["default_mode"].(string); ok {
			validModes := map[string]bool{"rule": true, "global": true, "direct": true}
			if !validModes[defaultMode] {
				return fmt.Errorf("invalid default_mode '%s'", defaultMode)
			}
			clashApi["default_mode"] = defaultMode
		}

		if len(clashApi) > 0 {
			exp["clash_api"] = clashApi
		} else {
			delete(exp, "clash_api")
		}
	}

	// Remove experimental section if empty
	if len(exp) == 0 {
		delete(m.config, "experimental")
	}

	return nil
}
