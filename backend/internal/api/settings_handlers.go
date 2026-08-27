package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"routebox/backend/internal/awg"
)

// awgPortEnv is set by the container when the deployment publishes the
// AmneziaWG UDP port. Its presence means the port is not the panel's to change:
// the mapping lives in docker-compose.yml, and a port changed here would land
// on a socket nobody forwards to.
const awgPortEnv = "AWG_LISTEN_PORT"

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
		writeConfigError(w, http.StatusBadRequest, err)
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
		writeConfigError(w, http.StatusBadRequest, err)
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

	// The AmneziaWG port belongs to the deployment when the deployment published
	// it: in Docker the compose file maps one UDP port onto the container, and a
	// port changed here would land on a socket nobody forwards to — the server
	// would come up and answer nobody. Refused where the choice is made, with the
	// place it is actually fixed named, rather than at Enable, where it looks
	// like a broken server.
	if v, ok := updates["awg.listen_port"]; ok {
		if fixed := os.Getenv(awgPortEnv); fixed != "" && fmt.Sprint(v) != fixed {
			writeError(w, http.StatusConflict,
				"the AmneziaWG port is fixed at "+fixed+" by this deployment: it is the port published on the container. "+
					"Change it in docker-compose.yml (the published port and "+awgPortEnv+") and recreate the container.")
			return
		}
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
		// RouteBox's kernel path runs `systemctl … awg-quick@<iface>`, so it needs
		// the amneziawg-tools and systemd present. Refused here, where the choice
		// is made and the reason can be named, instead of at Enable, where it
		// surfaces as a failing command. Deliberately a capability check and not
		// a runtime one: a host with the module loaded can serve a containerised
		// kernel interface just fine, and an image that ships the tools passes.
		if bs == "kernel" {
			if reason := awg.KernelBackendUnsupported(); reason != "" {
				writeError(w, http.StatusConflict, "the kernel AWG backend is unavailable on this system: "+reason+". Use the singbox backend, which needs neither.")
				return
			}
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
		// Decommission the OUTGOING backend BEFORE the setting is persisted. The
		// Enabled==false guard above is necessary but not sufficient: an
		// enabled-but-inactive awg-quick unit, a manually-launched iface, or a
		// broken `awg` tool all pass it while the kernel tunnel is (or will be at
		// next boot) alive — and after the switch Status routes to the new
		// backend, so the leftover keeps running panel-invisibly. Failing here
		// leaves settings AND Manager.backend unchanged (no split state).
		if h.awg != nil {
			if err := h.awg.PrepareBackendSwitch(r.Context(), bs); err != nil {
				writeError(w, http.StatusConflict, "failed to decommission the previous backend: "+err.Error())
				return
			}
		}
		// The switch requires a disabled server, but the PERSISTED awg.enabled can
		// still be stale-true (e.g. kernel was enabled, RouteBox restarted, and the
		// live-iface probe failed). Left as-is, the next restart would rehydrate
		// the NEW backend as enabled and silently start serving. Persist false
		// atomically with the backend value.
		updates["awg.enabled"] = false
	}

	// Apply updates. Update only stages into memory, so nothing here can be a
	// read-only refusal — Save below is the write.
	if err := h.settings.Update(updates); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Save to file
	if err := h.settings.Save(); err != nil {
		writeOpError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save settings: %v", err), err)
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
