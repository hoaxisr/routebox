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

	// Backend switch: only while the AWG server is disabled and no config draft is
	// pending (a singbox enable/disable rewrites the ACTIVE config, and a pending
	// draft would silently defer the sync). Mirrored to the live Manager below —
	// persisting the setting alone does NOT change Manager.backend (set at boot).
	if v, ok := updates["awg.backend"]; ok {
		bs, _ := v.(string)
		if bs != "kernel" && bs != "singbox" {
			writeError(w, http.StatusBadRequest, "invalid awg.backend")
			return
		}
		if h.awg != nil && h.awg.Status(r.Context()).Enabled {
			writeError(w, http.StatusConflict, "disable the AWG server before switching backend")
			return
		}
		// Status().Enabled stays false through every Enable phase (a kernel module
		// install can take minutes). Switching mid-enable would finish the
		// orchestrator on the OLD branch (iface + NAT up) while Status routes to the
		// new one — a live, panel-invisible interface. Reject while in flight (Bug I1).
		if h.awg != nil && h.awg.Busy() {
			writeError(w, http.StatusConflict, "AWG server is starting up — wait before switching backend")
			return
		}
		if h.config != nil && h.config.HasDraft() {
			writeError(w, http.StatusConflict, "apply or discard pending config changes before switching backend")
			return
		}
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

	// Apply a validated backend switch to the live Manager.
	if v, ok := updates["awg.backend"]; ok {
		if bs, _ := v.(string); (bs == "kernel" || bs == "singbox") && h.awg != nil {
			h.awg.SetBackend(bs)
		}
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
