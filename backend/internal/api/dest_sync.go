package api

import (
	"time"

	"routebox/backend/internal/bootstrap"
	"routebox/backend/internal/users"
)

// syncDest makes a user change take effect for naive.
//
// naive is the one protocol sing-box does not serve in an out-of-the-box
// install: dest checks its passwords itself (ADR 0002). So the panel writing
// inbound.users[] is only half of an "add a user" — the other half is this,
// which renders the very same users into the credential list dest imports and
// makes dest re-read it. Without it the panel starts lying: the user exists for
// four inbounds and does not exist for the fifth.
//
// A no-op unless this install came up from the bootstrap plan (that is the only
// shape whose dest RouteBox owns a Caddyfile for), and unless the rendered list
// actually differs — dest's reload is graceful, but a reload nobody needed is
// still work done on every apply.
//
// Errors are returned, not logged and dropped: a change that did not reach dest
// is exactly what the operator has to be told about.
func (h *Handler) syncDest() error {
	if h.settings == nil || h.config == nil {
		return nil
	}
	s := h.settings.Get()
	if !s.Server.Bootstrapped || s.Server.Caddyfile == "" {
		return nil
	}

	// Disabled and expired users are rejected inside sing-box by the managed
	// route rule, which dest never sees. Dropping them from the credential list
	// is how the same lifecycle decision reaches naive.
	var blocked map[string]bool
	if h.panelUsers != nil {
		blocked = map[string]bool{}
		for _, name := range users.EffectiveRejectNames(h.panelUsers.List(), time.Now().Unix()) {
			blocked[name] = true
		}
	}

	list := bootstrap.NaiveUsersOfConfig(h.config.GetActive(), blocked)
	changed, err := bootstrap.SyncNaiveUsers(bootstrap.NaiveUsersPath(s.Server.Caddyfile), list)
	if err != nil || !changed {
		return err
	}
	return bootstrap.ReloadCaddy(s.Server.Caddyfile)
}
