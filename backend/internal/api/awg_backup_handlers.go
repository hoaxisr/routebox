package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"routebox/backend/internal/awg"
	"routebox/backend/internal/settings"
)

// GetAWGBackup serves the AWG server state as a downloadable JSON (#97). The
// body is secrets (server key, peer private keys), hence no-store and a log
// line: this is the one GET that hands out the whole server identity.
func (h *Handler) GetAWGBackup(w http.ResponseWriter, r *http.Request) {
	if h.awg == nil {
		writeError(w, http.StatusServiceUnavailable, "awg not available")
		return
	}
	b, err := h.awg.Snapshot(h.settings.Get().Awg)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	log.Printf("awg: backup exported (%d peers) by %s", len(b.Peers), r.RemoteAddr)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="awg-backup.json"`)
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(b); err != nil {
		log.Printf("awg: backup write: %v", err)
	}
}

// awgBackupSettingsKeys are the awg.* settings a backup carries onto another
// host. enabled/configured are state, wan_iface is host-bound — never restored.
var awgBackupSettingsKeys = []string{
	"subnet", "listen_port", "mtu", "dns", "client_keepalive", "obf", "obf_preset",
	"backend", "server_host", "header_protection", "ipv6_broker",
}

// awgSettingsUpdates renders an awg settings section as the per-key map
// settings.Update takes, so every value gets the same validation the form
// gets. The JSON round-trip yields the shapes Update expects ([]interface{}
// for dns, a map for obf); nil values (dns absent) are skipped, not sent as
// null.
func awgSettingsUpdates(s settings.AwgSettings, configured bool) map[string]interface{} {
	raw, _ := json.Marshal(s)
	var m map[string]interface{}
	json.Unmarshal(raw, &m)
	updates := map[string]interface{}{"awg.configured": configured, "awg.enabled": false}
	for _, k := range awgBackupSettingsKeys {
		if v, ok := m[k]; ok && v != nil && !(k == "backend" && v == "") {
			updates["awg."+k] = v
		}
	}
	return updates
}

// RestoreAWGBackup replaces the awg settings section and peers.toml from an
// uploaded backup. Refused while the server is up (409); the operator presses
// Enable afterwards, which re-renders everything from the restored store.
//
// Order: settings are staged FIRST (Update validates and only touches memory),
// then the store is replaced, then settings are saved. A rejected value thus
// fails before the old peers are gone; a failed store replace puts the staged
// settings back. What remains unguarded is a failed Save after the store is
// replaced: memory is coherent, disk has the new peers and the old settings,
// and re-running the restore fixes it.
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
	prev := h.settings.Get().Awg
	if err := h.settings.Update(awgSettingsUpdates(b.Settings, true)); err != nil {
		writeError(w, http.StatusBadRequest, "backup settings: "+err.Error())
		return
	}
	if err := h.awg.Restore(b); err != nil {
		if rerr := h.settings.Update(awgSettingsUpdates(prev, prev.Configured)); rerr != nil {
			log.Printf("awg: restore rejected and settings could not be put back: %v", rerr)
		}
		if errors.Is(err, awg.ErrServerUp) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeOpError(w, http.StatusBadRequest, "invalid backup", err)
		return
	}
	if bs := b.Settings.Backend; bs == "kernel" || bs == "singbox" {
		h.awg.SetBackend(bs) // the live Manager, not only the file (mirrors settings_handlers)
	}
	if err := h.settings.Save(); err != nil {
		writeOpError(w, http.StatusInternalServerError, "failed to save settings", err)
		return
	}
	log.Printf("awg: backup restored (%d peers, subnet %s) by %s", len(b.Peers), b.Settings.Subnet, r.RemoteAddr)
	writeSuccess(w, map[string]int{"peers": len(b.Peers)})
}
