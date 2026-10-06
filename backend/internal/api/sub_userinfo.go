package api

import (
	"fmt"

	"routebox/backend/internal/users"
)

// formatUserinfo renders the Subscription-Userinfo header value per the
// SIP008 / clash-meta convention. total is the user's QuotaBytes; 0 = no quota.
// PURE.
func formatUserinfo(up, down, total, expire int64) string {
	return fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d", up, down, total, expire)
}

// userinfoUsage picks the up/down pair that belongs NEXT TO total= in the
// header. A subscription client reads the three as one triple — it renders
// "upload+download of total" and calls the rest the remaining allowance — so
// with a quota set the only pair that can honestly sit there is the quota's own
// counters (UsedTx/UsedRx, cumulative since the last reset). The lifetime totals
// from SQLite have a different lifetime by design: after "Reset counter" they
// still name every byte the user ever moved, which would leave the client
// showing an exhausted subscription for a user RouteBox has just put back in
// service (#95).
//
// Without a quota (total=0) there is nothing to be consistent with, so the
// lifetime totals stay — they are the more informative number, and that is what
// this header reported before quotas existed.
//
// PURE.
func userinfoUsage(u users.PanelUser, allTimeUp, allTimeDown int64) (up, down int64) {
	if u.QuotaBytes > 0 {
		return u.UsedTx, u.UsedRx
	}
	return allTimeUp, allTimeDown
}

// userAllTimeTraffic sums a user's lifetime up/down across all its display
// names. Returns 0,0 when no traffic store is wired.
func (h *Handler) userAllTimeTraffic(u users.PanelUser) (int64, int64) {
	if h.traffic == nil {
		return 0, 0
	}
	var up, down int64
	for _, name := range u.TrafficNames() {
		nu, nd, err := h.traffic.QueryUserTotals(0, 1<<62, name)
		if err == nil {
			up += nu
			down += nd
		}
	}
	return up, down
}
