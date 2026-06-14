package api

import (
	"routebox/backend/internal/users"
)

// panelUserNames returns the deduped, non-blank display names of all registry
// users — the value RouteBox writes to experimental.v2ray_api.stats.users.
func panelUserNames(mgr *users.Manager) []string {
	if mgr == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, u := range mgr.List() {
		if u.Name == "" || seen[u.Name] {
			continue
		}
		seen[u.Name] = true
		out = append(out, u.Name)
	}
	return out
}

// userTrafficNames returns the deduped, non-blank display names a single panel
// user is accounted under: its own Name plus each binding's cached Name. Traffic
// is SUMMED across these (a user with multiple bindings under different names is
// one logical user). PURE.
func userTrafficNames(u users.PanelUser) []string {
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
