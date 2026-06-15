package api

import (
	"log"
	"time"

	"routebox/backend/internal/users"
)

// syncRejectRule recomputes the reject set from the registry and writes it to the
// active config, reloading the process when the managed rule changed. No-op /
// deferred when there is no users manager, when nothing changed, or when a draft
// is pending (SyncRejectRuleActive defers internally). Reload failure falls back
// to Restart (matching ApplyConfig). Best-effort: failures are logged, never
// fatal. Status is read via getProcessStatus() (test-overridable) so a nil
// h.process never panics.
//
// Used by the PATCH handler, the startup one-shot, and the expiry ticker. NOT
// used inside ApplyConfig (which issues its own reload — see config_handlers.go).
func (h *Handler) syncRejectRule() {
	if h.panelUsers == nil {
		return
	}
	names := users.EffectiveRejectNames(h.panelUsers.List(), time.Now().Unix())
	changed, err := h.config.SyncRejectRuleActive(names)
	if err != nil {
		log.Printf("users: reject-rule sync failed: %v", err)
		return
	}
	if !changed {
		return
	}
	if h.getProcessStatus().Running {
		if err := h.process.Reload(); err != nil {
			if err := h.process.Restart(h.config.GetPath()); err != nil {
				log.Printf("users: reload after reject-rule sync failed: %v", err)
			}
		}
	}
}

// SyncRejectRuleAndReload is the exported entry point for main.go: the expiry
// ticker (as a func()) and the startup one-shot both call it. It defers on a
// pending draft and reloads only when the managed rule actually changed.
func (h *Handler) SyncRejectRuleAndReload() { h.syncRejectRule() }
