package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"routebox/backend/internal/config"
)

// GetConfig returns the current configuration
func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := h.config.Get()
	writeSuccess(w, cfg)
}

// SaveConfig saves full config to draft (does not restart - use ApplyConfig for that)
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

	// Replace draft with new config
	if err := h.config.SetDraft(newConfig); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save draft: %v", err))
		return
	}

	writeSuccess(w, map[string]interface{}{
		"message": "Config saved to draft (use Apply to restart)",
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

// ApplyConfig saves draft to disk and reloads/restarts (with optional validation)
func (h *Handler) ApplyConfig(w http.ResponseWriter, r *http.Request) {
	// Optionally validate draft before applying
	if h.config.HasDraft() {
		// Pre-check local rule-set paths so the user sees a friendly message
		// instead of sing-box's raw FATAL for missing .srs files.
		if pathErrs := config.ValidateLocalRuleSetPaths(h.config.Get()); len(pathErrs) > 0 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Config validation failed: %v", pathErrs))
			return
		}
		valid, errors := h.config.CheckDraft()
		if !valid {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Config validation failed: %v", errors))
			return
		}
	}

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

// --- Draft Config API ---

// GetConfigStatus returns draft status (hasDraft, changeCount)
func (h *Handler) GetConfigStatus(w http.ResponseWriter, r *http.Request) {
	hasDraft := h.config.HasDraft()

	// Calculate change count from diff
	changeCount := 0
	if hasDraft {
		_, additions, deletions, _ := h.config.GetDiff()
		changeCount = additions + deletions
	}

	writeSuccess(w, map[string]interface{}{
		"hasDraft":    hasDraft,
		"changeCount": changeCount,
	})
}

// DiscardConfig discards all draft changes and reverts to active config
func (h *Handler) DiscardConfig(w http.ResponseWriter, r *http.Request) {
	if err := h.config.DiscardDraft(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to discard: %v", err))
		return
	}

	writeSuccess(w, map[string]interface{}{
		"message": "Changes discarded",
	})
}

// GetDraftDiff returns diff between active and draft config
func (h *Handler) GetDraftDiff(w http.ResponseWriter, r *http.Request) {
	diff, additions, deletions, err := h.config.GetDiff()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to generate diff: %v", err))
		return
	}

	writeSuccess(w, map[string]interface{}{
		"diff":      diff,
		"additions": additions,
		"deletions": deletions,
	})
}

// SaveConfigDraft saves draft as active without restart
func (h *Handler) SaveConfigDraft(w http.ResponseWriter, r *http.Request) {
	if !h.config.HasDraft() {
		writeSuccess(w, map[string]interface{}{
			"message": "No changes to save",
		})
		return
	}

	if err := h.config.ApplyDraft(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save: %v", err))
		return
	}

	writeSuccess(w, map[string]interface{}{
		"message": "Config saved (restart required to apply)",
	})
}

// CheckConfig validates draft config using sing-box check
func (h *Handler) CheckConfig(w http.ResponseWriter, r *http.Request) {
	if !h.config.HasDraft() {
		writeSuccess(w, map[string]interface{}{
			"valid":  true,
			"errors": []string{},
		})
		return
	}

	valid, errors := h.config.CheckDraft()
	if errors == nil {
		errors = []string{}
	}

	writeSuccess(w, map[string]interface{}{
		"valid":  valid,
		"errors": errors,
	})
}

// GetActiveConfig returns the active config on disk (ignoring draft)
func (h *Handler) GetActiveConfig(w http.ResponseWriter, r *http.Request) {
	cfg := h.config.GetActive()
	writeSuccess(w, cfg)
}
