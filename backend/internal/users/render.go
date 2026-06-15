package users

import "sort"

// IsEffectivelyActive reports whether a user may currently connect: it is
// enabled AND not past its expiry. ExpiresAt==0 means "never expires". Time is
// unix seconds; at the exact boundary now==ExpiresAt the user is EXPIRED
// (the comparison is strict now < ExpiresAt). PURE.
func IsEffectivelyActive(u PanelUser, now int64) bool {
	return u.Enabled && (u.ExpiresAt == 0 || now < u.ExpiresAt)
}

// userNames returns the deduped, non-blank inbound-user names a single panel
// user is matchable under: its own Name plus each binding's cached Name. These
// ARE the metadata.User identities sing-box's auth_user rule matches on (twin of
// api.userTrafficNames). Own-name first, then binding order. PURE.
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
// user that is NOT effectively active. Empty (nil) slice => no reject rule
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
