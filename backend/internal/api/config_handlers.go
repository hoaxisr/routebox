package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"routebox/backend/internal/config"
	"routebox/backend/internal/mtproto"
	"routebox/backend/internal/process"
	"routebox/backend/internal/users"
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
		writeConfigError(w, http.StatusInternalServerError, err)
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
	gen := h.config.GetDraftGen()

	// Optionally validate draft before applying
	if h.config.HasDraft() {
		// Pre-check local rule-set paths so the user sees a friendly message
		// instead of sing-box's raw FATAL for missing .srs files. Relative
		// paths resolve against the config file's directory, matching how
		// sing-box's filemanager resolves them.
		configDir := filepath.Dir(h.config.GetPath())
		if pathErrs := config.ValidateLocalRuleSetPaths(h.config.Get(), configDir); len(pathErrs) > 0 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Config validation failed: %v", pathErrs))
			return
		}
		valid, checkErrs, checkedGen := h.config.CheckDraft()
		if !valid {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Config validation failed: %v", checkErrs))
			return
		}
		gen = checkedGen
	}

	if err := h.config.SaveToDiskIfGen(gen); err != nil {
		if errors.Is(err, config.ErrDraftChanged) {
			writeError(w, http.StatusConflict, "Draft changed since validation — re-check and retry")
			return
		}
		writeConfigError(w, http.StatusInternalServerError, err)
		return
	}

	// Mirror the now-applied active config into the panel-user registry.
	// A1 auto-deletes entries whose credential vanished; idempotent if unchanged.
	if h.panelUsers != nil {
		if _, err := h.panelUsers.Reconcile(h.config.GetActive()); err != nil {
			log.Printf("users: post-apply reconcile failed: %v", err)
		}
		// Keep experimental.v2ray_api stats.users synced with the registry so
		// sing-box counts per-user traffic (it counts a user ONLY if its name is
		// in stats.users). Change-gated; the apply's own reload (below) re-reads
		// the synced block from disk — do NOT add a second reload here.
		// Best-effort: a failure does not fail the apply. Panel context only —
		// empty names (router / no users) remove the block.
		listen := h.settings.Get().Singbox.V2RayAPI
		if listen == "" {
			listen = "127.0.0.1:8081"
		}
		// Additivity guard: only write the v2ray_api block when the running
		// binary advertises with_v2ray_api. An old/forked binary without it
		// REJECTS the block and the apply's reload/restart would fail the VPN.
		// Unsupported (incl. fail-closed detection error) → nil names so
		// SyncV2RayAPI REMOVES any stale block (a downgrade self-heals).
		var names []string
		if h.supportsV2RayAPI() {
			names = panelUserNames(h.panelUsers)
		}
		if _, err := h.config.SyncV2RayAPI(listen, names); err != nil {
			log.Printf("users: v2ray_api sync failed: %v", err)
		}

		// Lifecycle enforcement: recompute the managed reject rule from the
		// now-current registry and write it to active. hasDraft is already false
		// here (SaveToDiskIfGen applied/cleared it), so the writer does NOT defer.
		// The apply's own reload (below) picks up the rewritten active — do NOT
		// add a second reload (don't call h.syncRejectRule). Best-effort: a failure
		// logs but never fails the apply (mirrors Reconcile / SyncV2RayAPI above).
		rejectNames := users.EffectiveRejectNames(h.panelUsers.List(), time.Now().Unix())
		if _, err := h.config.SyncRejectRuleActive(rejectNames); err != nil {
			log.Printf("users: reject-rule sync (apply) failed: %v", err)
		}
	}

	// Re-assert the Telegram proxy's SOCKS inbound and route rule. An apply can
	// carry a draft that predates them, or one an operator edited them out of,
	// and the settings would still claim Telegram leaves through an outbound
	// while the plumbing is gone. Same contract as the two syncs above: hasDraft
	// is already false, and the apply's own reload (below) picks this up — do NOT
	// call h.syncMtprotoSocks here, which would reload a second time.
	if h.mtproto != nil {
		mt := h.settings.Get().Mtproto
		if _, err := h.config.SyncMtprotoSocksActive(mtproto.SocksPortOrDefault(mt.SocksPort), mt.Outbound); err != nil {
			log.Printf("mtproto: socks routing sync (apply) failed: %v", err)
		}
	}

	// naive is dest's, not sing-box's, so the users just applied reach it only
	// through here (see syncDest). Not fatal — the other four inbounds did get
	// the change — but not swallowed either: the answer carries a warning, so
	// the operator learns that naive is still on the previous list instead of
	// finding out from a client that cannot connect.
	respond := writeSuccess
	if err := h.syncDest(); err != nil {
		log.Printf("dest: naive user sync failed: %v", err)
		warning := fmt.Sprintf("the user list did not reach dest, so naive still authenticates against the previous one: %v", err)
		respond = func(w http.ResponseWriter, data interface{}) {
			if m, ok := data.(map[string]interface{}); ok {
				m["warning"] = warning
			}
			writeSuccess(w, data)
		}
	}

	// Check if we should use reload or restart
	// Query param ?mode=reload|restart (default: reload)
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "reload"
	}

	status := h.getProcessStatus()
	if !status.Running {
		respond(w, map[string]interface{}{
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
			respond(w, map[string]interface{}{
				"message":   "Config applied (reload failed, used restart)",
				"reloaded":  false,
				"restarted": true,
			})
			return
		}
		respond(w, map[string]interface{}{
			"message":  "Config applied (hot reload)",
			"reloaded": true,
		})
	} else {
		// Force restart
		if err := h.process.Restart(h.config.GetPath()); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("Saved but failed to restart: %v", err))
			return
		}
		respond(w, map[string]interface{}{
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
	// DiscardDraft drops the draft from memory and only logs a file it could not
	// remove, so there is nothing here that a read-only config can turn into a
	// 409: Discard stays available exactly when everything else is blocked.
	if err := h.config.DiscardDraft(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeConfigError(w, http.StatusInternalServerError, err)
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

	valid, checkErrs, _ := h.config.CheckDraft()
	if checkErrs == nil {
		checkErrs = []string{}
	}

	writeSuccess(w, map[string]interface{}{
		"valid":  valid,
		"errors": checkErrs,
	})
}

// GetActiveConfig returns the active config on disk (ignoring draft)
func (h *Handler) GetActiveConfig(w http.ResponseWriter, r *http.Request) {
	cfg := h.config.GetActive()
	writeSuccess(w, cfg)
}

// FixUnitConfigPath перенацеливает systemd-юнит на путь конфига, которым
// управляет RouteBox. Вызывается только по кнопке пользователя: это
// единственное место, где RouteBox пишет за пределами своих файлов, поэтому
// без зафиксированного расхождения ничего не пишем вовсе.
func (h *Handler) FixUnitConfigPath(w http.ResponseWriter, r *http.Request) {
	ours, _, ok := h.process.ConfigMismatch()
	if !ok {
		writeError(w, http.StatusConflict, "no config path mismatch to fix")
		return
	}
	if err := h.process.WriteConfigDropIn(ours); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeSuccess(w, map[string]string{"message": "systemd unit now uses " + ours})
}

// RemoveUnitConfigDropIn снимает drop-in, которым RouteBox перенацелил юнит, —
// обратная операция к FixUnitConfigPath. Без неё единственная запись RouteBox за
// пределами своих файлов была односторонней: снять её можно было только руками
// (rm + daemon-reload), зная и имя файла, и то, что его вообще ставили.
//
// Ни режим только-чтения конфига, ни состояние конфига здесь ни при чём:
// правится systemd-юнит, а не файлы RouteBox. Нечитаемый на запись конфиг — как
// раз повод перенацелить юнит или откатить это, а не запрет.
func (h *Handler) RemoveUnitConfigDropIn(w http.ResponseWriter, r *http.Request) {
	res, err := h.process.RemoveConfigDropIn()
	switch {
	case errors.Is(err, process.ErrNoConfigDropIn):
		// Не поломка, а состояние: снимать нечего.
		writeError(w, http.StatusConflict, "RouteBox has no systemd drop-in installed here, so there is nothing to remove")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// unit_path отдаётся отдельно от текста: панель говорит о вернувшемся
	// расхождении на языке пользователя, а факт «юнит теперь на этом пути» —
	// наш. Пустая строка означает «прочитать юнит не вышло», и об этом же
	// говорит warning.
	resp := map[string]interface{}{
		"message":   "The RouteBox drop-in is removed",
		"unit_path": res.UnitConfigPath,
	}
	if res.Warning != "" {
		resp["warning"] = res.Warning
	}
	writeSuccess(w, resp)
}
