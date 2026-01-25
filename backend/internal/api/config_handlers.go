package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// GetConfig returns the current configuration
func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := h.config.Get()
	writeSuccess(w, cfg)
}

// SaveConfig saves and applies the configuration
func (h *Handler) SaveConfig(w http.ResponseWriter, r *http.Request) {
	var newConfig map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	// Validate
	errors := h.config.Validate(newConfig)
	if len(errors) > 0 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Validation errors: %v", errors))
		return
	}

	// Save
	if err := h.config.Save(newConfig); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save: %v", err))
		return
	}

	// Reload if running (try hot reload first)
	status := h.process.GetStatus()
	if status.Running {
		if err := h.process.Reload(); err != nil {
			// Reload failed, try restart
			if err := h.process.Restart(h.config.GetPath()); err != nil {
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("Saved but failed to reload/restart: %v", err))
				return
			}
			writeSuccess(w, map[string]interface{}{
				"message":   "Config saved and restarted",
				"reloaded":  false,
				"restarted": true,
			})
			return
		}
		writeSuccess(w, map[string]interface{}{
			"message":  "Config saved and reloaded",
			"reloaded": true,
		})
		return
	}

	writeSuccess(w, map[string]interface{}{
		"message": "Config saved",
	})
}

// ValidateConfig validates config without saving
func (h *Handler) ValidateConfig(w http.ResponseWriter, r *http.Request) {
	var newConfig map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	errors := h.config.Validate(newConfig)
	writeSuccess(w, map[string]interface{}{
		"valid":  len(errors) == 0,
		"errors": errors,
	})
}

// GetConfigDiff returns diff between current and new config
func (h *Handler) GetConfigDiff(w http.ResponseWriter, r *http.Request) {
	var newConfig map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	diff, err := h.config.Diff(newConfig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to diff: %v", err))
		return
	}

	writeSuccess(w, map[string]interface{}{
		"diff": diff,
	})
}

// ExportConfig returns config as downloadable JSON file
func (h *Handler) ExportConfig(w http.ResponseWriter, r *http.Request) {
	cfg := h.config.Get()

	// Use encoder with SetEscapeHTML(false) to preserve special characters
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to encode config: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=sing-box-config.json")
	w.Write(buf.Bytes())
}

// ImportConfig validates uploaded config and returns validation results
func (h *Handler) ImportConfig(w http.ResponseWriter, r *http.Request) {
	var newConfig map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	errors := h.config.Validate(newConfig)
	writeSuccess(w, map[string]interface{}{
		"valid":  len(errors) == 0,
		"errors": errors,
		"config": newConfig,
	})
}

// ApplyConfig saves in-memory config to disk and reloads/restarts
func (h *Handler) ApplyConfig(w http.ResponseWriter, r *http.Request) {
	if err := h.config.SaveToDisk(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save: %v", err))
		return
	}

	// Check if we should use reload or restart
	// Query param ?mode=reload|restart (default: reload)
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "reload"
	}

	status := h.process.GetStatus()
	if !status.Running {
		writeSuccess(w, map[string]interface{}{
			"message":   "Config saved (process not running)",
			"reloaded":  false,
			"restarted": false,
		})
		return
	}

	if mode == "reload" {
		// Try reload first (SIGHUP)
		if err := h.process.Reload(); err != nil {
			// Reload failed, try restart as fallback
			if err := h.process.Restart(h.config.GetPath()); err != nil {
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("Saved but failed to reload/restart: %v", err))
				return
			}
			writeSuccess(w, map[string]interface{}{
				"message":   "Config applied (reload failed, used restart)",
				"reloaded":  false,
				"restarted": true,
			})
			return
		}
		writeSuccess(w, map[string]interface{}{
			"message":  "Config applied (hot reload)",
			"reloaded": true,
		})
	} else {
		// Force restart
		if err := h.process.Restart(h.config.GetPath()); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("Saved but failed to restart: %v", err))
			return
		}
		writeSuccess(w, map[string]interface{}{
			"message":   "Config applied (restarted)",
			"restarted": true,
		})
	}
}
