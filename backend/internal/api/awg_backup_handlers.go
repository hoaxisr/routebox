package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"routebox/backend/internal/awg"
)

// GetAWGBackup serves the AWG server state as a downloadable JSON (#97). The
// body is secrets (server key, peer private keys), hence no-store.
func (h *Handler) GetAWGBackup(w http.ResponseWriter, r *http.Request) {
	if h.awg == nil {
		writeError(w, http.StatusServiceUnavailable, "awg not available")
		return
	}
	b := h.awg.Snapshot(h.settings.Get().Awg)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="awg-backup.json"`)
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(b)
}

// awgBackupSettingsKeys are the awg.* settings a backup carries onto another
// host. enabled/configured are state, wan_iface is host-bound — never restored.
var awgBackupSettingsKeys = []string{
	"subnet", "listen_port", "mtu", "dns", "client_keepalive", "obf", "obf_preset",
	"backend", "server_host", "header_protection", "ipv6_broker",
}

// RestoreAWGBackup replaces peers.toml and the awg settings section from an
// uploaded backup. Refused while the server is up (409); the operator presses
// Enable afterwards, which re-renders everything from the restored store.
func (h *Handler) RestoreAWGBackup(w http.ResponseWriter, r *http.Request) {
	if h.awg == nil {
		writeError(w, http.StatusServiceUnavailable, "awg not available")
		return
	}
	var b awg.Backup
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid backup: "+err.Error())
		return
	}
	if err := h.awg.Restore(b); err != nil {
		if errors.Is(err, awg.ErrServerUp) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeOpError(w, http.StatusBadRequest, "invalid backup", err)
		return
	}
	// Settings go through the same per-key Update the form uses, so every value
	// gets the same validation. JSON round-trip yields the map shapes it expects.
	raw, _ := json.Marshal(b.Settings)
	var m map[string]interface{}
	json.Unmarshal(raw, &m)
	updates := map[string]interface{}{"awg.configured": true, "awg.enabled": false}
	for _, k := range awgBackupSettingsKeys {
		if v, ok := m[k]; ok && !(k == "backend" && v == "") {
			updates["awg."+k] = v
		}
	}
	if err := h.settings.Update(updates); err != nil {
		writeError(w, http.StatusBadRequest, "backup settings: "+err.Error())
		return
	}
	if err := h.settings.Save(); err != nil {
		writeOpError(w, http.StatusInternalServerError, "failed to save settings", err)
		return
	}
	writeSuccess(w, map[string]int{"peers": len(b.Peers)})
}
