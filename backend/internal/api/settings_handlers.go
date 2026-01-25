package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// --- Log Settings ---

// GetLogSettings returns log settings
func (h *Handler) GetLogSettings(w http.ResponseWriter, r *http.Request) {
	settings := h.config.GetLogSettings()
	writeSuccess(w, settings)
}

// UpdateLogSettings updates log settings
func (h *Handler) UpdateLogSettings(w http.ResponseWriter, r *http.Request) {
	var settings map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if err := h.config.UpdateLogSettings(settings); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeSuccess(w, settings)
}

// --- Experimental Settings ---

// GetExperimental returns experimental settings
func (h *Handler) GetExperimental(w http.ResponseWriter, r *http.Request) {
	settings := h.config.GetExperimental()
	writeSuccess(w, settings)
}

// UpdateExperimental updates experimental settings
func (h *Handler) UpdateExperimental(w http.ResponseWriter, r *http.Request) {
	var settings map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if err := h.config.UpdateExperimental(settings); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeSuccess(w, settings)
}

// --- RouteBox Settings ---

// GetSettings returns RouteBox application settings
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "Settings not configured")
		return
	}

	cfg := h.settings.Get()

	// Add runtime info
	response := map[string]interface{}{
		"settings":      cfg,
		"settings_path": h.settings.GetPath(),
		"geoip_loaded":  h.geoip != nil && h.geoip.IsLoaded(),
	}

	writeSuccess(w, response)
}

// UpdateSettings updates RouteBox application settings (runtime-safe fields only)
func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "Settings not configured")
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	// Apply updates
	if err := h.settings.Update(updates); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Save to file
	if err := h.settings.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save settings: %v", err))
		return
	}

	writeSuccess(w, map[string]interface{}{
		"message":  "Settings updated",
		"settings": h.settings.Get(),
	})
}

// ReloadSettings reloads settings from file
func (h *Handler) ReloadSettings(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "Settings not configured")
		return
	}

	if err := h.settings.Load(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reload settings: %v", err))
		return
	}

	writeSuccess(w, map[string]interface{}{
		"message":  "Settings reloaded",
		"settings": h.settings.Get(),
	})
}
