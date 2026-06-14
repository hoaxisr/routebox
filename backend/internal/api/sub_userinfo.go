package api

import (
	"fmt"

	"routebox/backend/internal/users"
)

// formatUserinfo renders the Subscription-Userinfo header value per the
// SIP008 / clash-meta convention. total=0 means "no quota" (Phase 5 read-only).
// PURE.
func formatUserinfo(up, down, total, expire int64) string {
	return fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d", up, down, total, expire)
}

// userAllTimeTraffic sums a user's lifetime up/down across all its display
// names. Returns 0,0 when no traffic store is wired.
func (h *Handler) userAllTimeTraffic(u users.PanelUser) (int64, int64) {
	if h.traffic == nil {
		return 0, 0
	}
	var up, down int64
	for _, name := range userTrafficNames(u) {
		nu, nd, err := h.traffic.QueryUserTotals(0, 1<<62, name)
		if err == nil {
			up += nu
			down += nd
		}
	}
	return up, down
}
