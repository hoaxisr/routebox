package users

import (
	"sort"

	"routebox/backend/internal/quota"
)

// SuspendReason returns the single reason this user is out of service right now,
// or quota.ReasonNone. The decision itself lives in the shared quota package so a
// panel user and an AWG peer can never disagree about the same four numbers;
// priority is manual → quota → expired (spec Q18). PURE.
func SuspendReason(u PanelUser, now int64) quota.Reason {
	return quota.State(u.QuotaBytes, u.UsedRx, u.UsedTx, u.Enabled, u.ExpiresAt, now)
}

// IsEffectivelyActive reports whether a user may currently connect: enabled, with
// quota left and not past its expiry. QuotaBytes==0 means "no limit",
// ExpiresAt==0 means "never expires". Time is unix seconds; at the exact boundary
// now==ExpiresAt the user is EXPIRED. PURE.
func IsEffectivelyActive(u PanelUser, now int64) bool {
	return SuspendReason(u, now) == quota.ReasonNone
}

// userNames returns the deduped, non-blank inbound-user names a single panel
// user is matchable under: its own Name plus each binding's cached Name. These
// ARE the metadata.User identities sing-box's auth_user rule matches on (twin of
// PanelUser.TrafficNames). Own-name first, then binding order. PURE.
func userNames(u PanelUser) []string {
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		if n == "" || seen[n] {
			return
		}
		seen[n] = true
		out = append(out, n)
	}
	add(u.Name)
	for _, b := range u.Bindings {
		add(b.Name)
	}
	return out
}

// EffectiveRejectNames returns the sorted, de-duplicated set of inbound user
// names that must be rejected right now: the union of userNames(u) over every
// user that is NOT effectively active (manually disabled, out of quota, or
// expired — see SuspendReason). Empty (nil) slice => no reject rule
// needed (every user active / no users). PURE.
func EffectiveRejectNames(list []PanelUser, now int64) []string {
	seen := map[string]bool{}
	var out []string
	for _, u := range list {
		if IsEffectivelyActive(u, now) {
			continue
		}
		for _, n := range userNames(u) {
			if seen[n] {
				continue
			}
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// DuplicateNames returns the sorted set of non-blank names shared by more than
// one user. auth_user matches by NAME, so duplicates over-block (all same-named
// users are rejected when any one is inactive). Surfaced as a startup warning so
// the operator can rename; pre-existing duplicates are tolerated, not blocked.
// PURE.
func DuplicateNames(list []PanelUser) []string {
	count := map[string]int{}
	for _, u := range list {
		if u.Name != "" {
			count[u.Name]++
		}
	}
	var out []string
	for n, c := range count {
		if c > 1 {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}
