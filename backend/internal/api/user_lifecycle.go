package api

import (
	"fmt"
	"log"
	"sort"
	"time"

	"routebox/backend/internal/users"
)

// syncRejectRule recomputes the reject set from the registry and writes it to the
// active config, reloading the process when the managed rule changed. The same
// recomputation then reaches dest, which enforces the lifecycle for naive on its
// own. Reload failure falls back to Restart (matching ApplyConfig). Status is
// read via getProcessStatus() (test-overridable) so a nil h.process never panics.
//
// It returns the two ways the decision can fail to take effect, because a
// lifecycle change the panel reports as done while the user still connects is
// the one outcome nobody can debug from the UI:
//
//   - deferred — the rule was NOT written or NOT reloaded, and the caller's
//     answer should say so: a pending config draft (SyncRejectRuleActive defers
//     to the coming Apply), a read-only config, a write that failed, or a reload
//     AND restart that both failed. Empty when the rule is live. Nothing is
//     reported when the wanted rule is already the one in the active config —
//     a deferral that changes nothing is not a deferral.
//   - err — dest's: a decision that did not reach dest leaves the user
//     connecting over naive after the panel said they were disabled, and the
//     PATCH handler is standing right there with an answer to put it in.
//
// Used by the PATCH/reset handlers, the startup one-shot, and the expiry ticker.
// NOT used inside ApplyConfig (which issues its own reload — see
// config_handlers.go).
func (h *Handler) syncRejectRule() (deferred string, err error) {
	if h.panelUsers == nil {
		return "", nil
	}
	names := users.EffectiveRejectNames(h.panelUsers.List(), time.Now().Unix())
	// What the active config would have to become. Compared BEFORE the write so
	// a deferral is only reported when it actually withholds something.
	pending := !sameNames(names, activeRejectNames(h.config.GetActive()))
	changed, syncErr := h.config.SyncRejectRuleActive(names)
	if syncErr != nil {
		log.Printf("users: reject-rule sync failed: %v", syncErr)
		return fmt.Sprintf("the reject rule could not be written: %v", syncErr), nil
	}
	if !changed && pending {
		switch {
		case h.config.IsReadOnly():
			deferred = "the sing-box config is read-only, so the rule was not written"
		case h.config.HasDraft():
			deferred = "a config draft is pending — apply it to enforce this"
		}
	}
	if changed && h.getProcessStatus().Running {
		if err := h.process.Reload(); err != nil {
			if err := h.process.Restart(h.config.GetPath()); err != nil {
				log.Printf("users: reload after reject-rule sync failed: %v", err)
				deferred = fmt.Sprintf("the rule is written but amnezia-box did not take it: %v", err)
			}
		}
	}
	// The reject rule is sing-box's half of the lifecycle; naive is dest's, and
	// dest never sees that rule. Change-gated inside, so an unchanged list costs
	// nothing.
	if err := h.syncDest(); err != nil {
		if !destErrorIsRepeat(err) {
			log.Printf("dest: sync failed: %v", err)
		}
		return deferred, err
	}
	return deferred, nil
}

// activeRejectNames returns the auth_user names of RouteBox's managed reject rule
// in an active config, or nil when it holds none. Mirrors config.managedRejectRule
// (unexported there): action=="reject", a non-empty auth_user, and exactly those
// two keys — anything else is an operator's own rule. Sorted like
// users.EffectiveRejectNames so the two are directly comparable. PURE.
func activeRejectNames(active map[string]interface{}) []string {
	route, _ := active["route"].(map[string]interface{})
	rules, _ := route["rules"].([]interface{})
	for _, r := range rules {
		rm, ok := r.(map[string]interface{})
		if !ok || len(rm) != 2 {
			continue
		}
		if action, _ := rm["action"].(string); action != "reject" {
			continue
		}
		au, ok := rm["auth_user"].([]interface{})
		if !ok || len(au) == 0 {
			continue
		}
		out := make([]string, 0, len(au))
		for _, n := range au {
			if s, ok := n.(string); ok {
				out = append(out, s)
			}
		}
		sort.Strings(out)
		return out
	}
	return nil
}

// sameNames reports whether two name sets are equal element-wise (both sorted by
// their producers). PURE.
func sameNames(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// SyncRejectRuleAndReload is the exported entry point for main.go: the expiry
// ticker (as a func()) and the startup one-shot both call it. It defers on a
// pending draft and reloads only when the managed rule actually changed.
//
// Nobody is waiting on these two, so the dest failure that the PATCH handler
// answers with is only logged here — the next apply or PATCH surfaces it.
func (h *Handler) SyncRejectRuleAndReload() {
	// The deferral is dropped here on purpose: a pending draft or a read-only
	// config would otherwise log the same line every 30 seconds forever, and
	// both states are already on the panel. The handlers answer with it.
	// A re-plan failure already reported, with nothing changed since, is not
	// logged again: this runs every 30 seconds.
	if _, err := h.syncRejectRule(); err != nil && !destErrorIsRepeat(err) {
		log.Printf("users: lifecycle sync to dest failed: %v", err)
	}
}

// nameTaken reports whether name collides with an existing panel user — either a
// registry user or a pending (draft-only, server-inbound) user (the same surface
// ListUsers derives Pending from). Used by CreateUser to prevent NEW
// inbound-user-name collisions: auth_user matches by name, so two users sharing a
// name would be lifecycle-blocked together. Pre-existing duplicates are tolerated
// (a startup warning flags them) — this only blocks creating fresh ones. Empty
// name is never taken. AddBinding is exempt (it reuses an existing user's own
// name — same identity, no new name — and never calls this).
func (h *Handler) nameTaken(name string) bool {
	if name == "" {
		return false
	}
	for _, u := range h.panelUsers.List() {
		if u.Name == name {
			return true
		}
	}
	registered := map[string]bool{}
	for _, u := range h.panelUsers.List() {
		for _, b := range u.Bindings {
			registered[b.InboundTag+"\x00"+b.Credential] = true
		}
	}
	for _, ib := range h.config.ListInbounds() {
		for _, cu := range users.ServerInboundUsers(ib) {
			if registered[cu.InboundTag+"\x00"+cu.Credential] {
				continue
			}
			if cu.Name == name {
				return true // pending draft user with this name
			}
		}
	}
	return false
}
