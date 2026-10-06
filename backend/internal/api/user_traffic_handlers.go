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
