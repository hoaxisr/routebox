package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/traffic"
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

// userTrafficResponse is the GET /api/users/{id}/traffic wire shape.
type userTrafficResponse struct {
	Upload   int64                    `json:"upload"`
	Download int64                    `json:"download"`
	History  []traffic.UserHistoryRow `json:"history"`
}

// GetUserTraffic returns one panel user's total + per-bucket traffic over a
// range (?range=1h|3h|24h|week|month; default 24h). Traffic is summed across the
// user's binding display-names (userTrafficNames). Collision note: if two panel
// users share a display-name their counters merge under that name and both report
// the combined total — attribution is by name, not panel id (uniqueness
// validator is a documented follow-up, out of scope). PROTECTED.
func (h *Handler) GetUserTraffic(w http.ResponseWriter, r *http.Request) {
	if h.panelUsers == nil {
		writeError(w, http.StatusServiceUnavailable, "users not initialized")
		return
	}
	id := chi.URLParam(r, "id")
	u, ok := h.panelUsers.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if h.traffic == nil {
		// Per-user accounting unavailable (e.g. fork without with_v2ray_api):
		// return zeros rather than failing — additive behaviour.
		writeSuccess(w, userTrafficResponse{History: []traffic.UserHistoryRow{}})
		return
	}
	dur := rangeToSeconds(r.URL.Query().Get("range"))
	if dur == 0 {
		dur = 86400 // default 24h
	}
	now := time.Now().Unix()
	start := now - dur

	var totUp, totDown int64
	bucketSum := map[int64]traffic.UserHistoryRow{}
	for _, name := range userTrafficNames(u) {
		up, down, err := h.traffic.QueryUserTotals(start, now, name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		totUp += up
		totDown += down
		hist, err := h.traffic.QueryUserHistory(start, now, name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, row := range hist {
			agg := bucketSum[row.BucketTs]
			agg.BucketTs = row.BucketTs
			agg.Upload += row.Upload
			agg.Download += row.Download
			bucketSum[row.BucketTs] = agg
		}
	}
	writeSuccess(w, userTrafficResponse{Upload: totUp, Download: totDown, History: flattenUserBuckets(bucketSum)})
}

// flattenUserBuckets returns the per-bucket map as a time-ascending slice.
func flattenUserBuckets(m map[int64]traffic.UserHistoryRow) []traffic.UserHistoryRow {
	out := make([]traffic.UserHistoryRow, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	for i := 1; i < len(out); i++ { // insertion sort (small N)
		for j := i; j > 0 && out[j-1].BucketTs > out[j].BucketTs; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
