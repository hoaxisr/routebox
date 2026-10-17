package api

import (
	"errors"
	"fmt"
	"strings"
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

	// The Caddyfile itself first: it names the routes the credential list is
	// imported into, and a plan that no longer matches the settings (a moved
	// web front, a WEB proxy switched on) would leave the naive reload below
	// reloading the stale one.
	//
	// A failing re-plan is kept, not returned: the naive sync below is the only
	// thing that takes a disabled or expired user's naive access away, and an
	// unrelated Caddyfile problem must not leave that user connected. dest keeps
	// serving its previous Caddyfile, which imports the very same list, so the
	// sync still lands.
	var replanErr error
	if h.caddyReplan != nil {
		if err := h.caddyReplan(); err != nil {
			replanErr = &destReplanError{err: err}
		}
	}

	return errors.Join(replanErr, h.syncNaiveUsers(s.Server.Caddyfile))
}

// syncNaiveUsers renders the panel's users into dest's credential list and has
// dest re-read it when the list changed. Its error comes back as a
// naiveSyncError, so the caller can tell the operator which half failed.
func (h *Handler) syncNaiveUsers(caddyfile string) error {
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
	changed, err := bootstrap.SyncNaiveUsers(bootstrap.NaiveUsersPath(caddyfile), list)
	if err == nil && changed {
		err = bootstrap.ReloadCaddy(caddyfile)
	}
	if err != nil {
		return &naiveSyncError{err: err}
	}
	return nil
}

// destReplanError is a syncDest failure in re-planning dest's Caddyfile.
type destReplanError struct{ err error }

func (e *destReplanError) Error() string { return "caddyfile: " + e.err.Error() }
func (e *destReplanError) Unwrap() error { return e.err }

// naiveSyncError is a syncDest failure in getting the naive credential list
// to dest.
type naiveSyncError struct{ err error }

func (e *naiveSyncError) Error() string { return "naive users: " + e.err.Error() }
func (e *naiveSyncError) Unwrap() error { return e.err }

// destErrorIsRepeat reports whether a syncDest error is nothing but a
// Caddyfile re-plan failure already reported, with nothing it depends on
// changed since (bootstrap.ErrReplanRepeated). A timer-driven caller logs the
// first one and leaves the repeats out; a naive sync failure is always new.
func destErrorIsRepeat(err error) bool {
	var naive *naiveSyncError
	return errors.Is(err, bootstrap.ErrReplanRepeated) && !errors.As(err, &naive)
}

// destSyncWarning words a syncDest error for an API answer, naming which half
// failed: a Caddyfile re-plan leaves dest on its previous routes, a naive sync
// leaves naive on the previous user list, and the operator has to know which.
func destSyncWarning(err error) string {
	var parts []string
	var replan *destReplanError
	if errors.As(err, &replan) {
		parts = append(parts, fmt.Sprintf("dest's Caddyfile could not be re-planned, so dest still serves the previous one: %v", replan.err))
	}
	var naive *naiveSyncError
	if errors.As(err, &naive) {
		parts = append(parts, fmt.Sprintf("the user list did not reach dest, so naive still authenticates against the previous one: %v", naive.err))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("dest did not take the change: %v", err)
	}
	return strings.Join(parts, "; ")
}
