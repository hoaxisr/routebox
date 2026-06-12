package api

import (
	"fmt"
	"net/http"
)

// GetVersion returns the sing-box version, feature flags and RouteBox version
func (h *Handler) GetVersion(w http.ResponseWriter, r *http.Request) {
	version, err := h.process.GetVersion()
	if err != nil {
		version = ""
	}
	features := h.process.GetFeatureFlags()
	writeSuccess(w, map[string]interface{}{
		"version":          version,
		"features":         features,
		"routebox_version": h.routeboxVersion,
	})
}

// GetStatus returns amnezia-box process status
func (h *Handler) GetStatus(w http.ResponseWriter, r *http.Request) {
	status := h.process.GetStatus()
	writeSuccess(w, status)
}

// NeedsSetup checks if the setup wizard should be shown
// Returns needs_setup=true if:
// 1. sing-box binary is not installed, OR
// 2. sing-box is not running AND config has no VPN (vless/hy2 outbound or awg endpoint), OR
// 3. sing-box is running AND config has no VPN configuration
func (h *Handler) NeedsSetup(w http.ResponseWriter, r *http.Request) {
	binaryInstalled := h.process.IsBinaryInstalled()
	status := h.process.GetStatus()
	hasVpnConfig := h.config.HasVpnConfig()

	needsSetup := false
	reason := ""

	if !binaryInstalled {
		needsSetup = true
		reason = "binary_not_installed"
	} else if !status.Running && !hasVpnConfig {
		needsSetup = true
		reason = "not_running_no_vpn"
	} else if status.Running && !hasVpnConfig {
		needsSetup = true
		reason = "running_no_vpn"
	}

	writeSuccess(w, map[string]interface{}{
		"needs_setup":          needsSetup,
		"reason":               reason,
		"binary_installed":     binaryInstalled,
		"running":              status.Running,
		"has_vpn_config":       hasVpnConfig,
		"clash_api_configured": h.getClashAddr() != "",
		"version":              status.Version,
		"binary_path":          status.BinaryPath,
	})
}

// Start starts the amnezia-box process
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	if err := h.process.Start(h.config.GetPath()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, map[string]interface{}{
		"message": "Started",
	})
}

// Stop stops the amnezia-box process
func (h *Handler) Stop(w http.ResponseWriter, r *http.Request) {
	if err := h.process.Stop(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, map[string]string{"message": "Stopped"})
}

// Restart restarts the amnezia-box process
func (h *Handler) Restart(w http.ResponseWriter, r *http.Request) {
	if err := h.process.Restart(h.config.GetPath()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, map[string]interface{}{
		"message": "Restarted",
	})
}

// Reload sends SIGHUP to reload config without restarting
func (h *Handler) Reload(w http.ResponseWriter, r *http.Request) {
	if err := h.process.Reload(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, map[string]string{"message": "Configuration reloaded"})
}

// GetDetectedConfig returns the auto-detected config path from running process
func (h *Handler) GetDetectedConfig(w http.ResponseWriter, r *http.Request) {
	detectedPath := h.process.GetDetectedConfigPath()
	currentPath := h.config.GetPath()

	writeSuccess(w, map[string]interface{}{
		"detected_path": detectedPath,
		"current_path":  currentPath,
		"match":         detectedPath == currentPath || detectedPath == "",
	})
}

// UseDetectedConfig switches to using the detected config path
func (h *Handler) UseDetectedConfig(w http.ResponseWriter, r *http.Request) {
	detectedPath := h.process.GetDetectedConfigPath()
	if detectedPath == "" {
		writeError(w, http.StatusNotFound, "No config path detected from running process")
		return
	}

	// Switch to detected config path
	if err := h.config.SetPath(detectedPath); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to load config from %s: %v", detectedPath, err))
		return
	}

	writeSuccess(w, map[string]interface{}{
		"message": "Switched to detected config",
		"path":    detectedPath,
	})
}

// GetJournalLogs returns recent logs from systemd journal
func (h *Handler) GetJournalLogs(w http.ResponseWriter, r *http.Request) {
	linesParam := r.URL.Query().Get("lines")
	lines := 50
	if linesParam != "" {
		if n, err := fmt.Sscanf(linesParam, "%d", &lines); err != nil || n != 1 {
			lines = 50
		}
	}
	if lines > 500 {
		lines = 500
	}

	logs, err := h.process.GetJournalLogs(lines)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, map[string]interface{}{
		"logs":  logs,
		"lines": lines,
	})
}
