package api

import (
	"net/http"
	"strings"
	"time"

	"routebox/backend/internal/traffic"
	"routebox/backend/internal/util"
)

type trafficResponse struct {
	Range   string          `json:"range"`
	StartTs int64           `json:"start_ts"`
	EndTs   int64           `json:"end_ts"`
	Buckets []trafficBucket `json:"buckets"`
	// With ?series=1: the same window as a time series (bytes per bucket of
	// `step` seconds, buckets with no traffic omitted), summed over the
	// filters above — the dashboard graph for 1h/24h (#99).
	Series []traffic.UserHistoryRow `json:"series,omitempty"`
	Step   int64                    `json:"step,omitempty"`
}

type trafficBucket struct {
	Source   string `json:"source"`
	Domain   string `json:"domain"`
	Chain    string `json:"chain"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
}

// rangeToSeconds maps the public range token to a duration in seconds.
// Returns 0 for unknown tokens.
func rangeToSeconds(r string) int64 {
	switch r {
	case "1h":
		return 3600
	case "3h":
		return 10800
	case "24h":
		return 86400
	case "week":
		return 7 * 86400
	case "month":
		return 30 * 86400
	}
	return 0
}

// GetTrafficHistory returns aggregated traffic over the requested range.
func (h *Handler) GetTrafficHistory(w http.ResponseWriter, r *http.Request) {
	if h.traffic == nil {
		writeError(w, http.StatusServiceUnavailable, "traffic store not initialized")
		return
	}
	q := r.URL.Query()
	rng := q.Get("range")
	dur := rangeToSeconds(rng)
	if dur == 0 {
		writeError(w, http.StatusBadRequest, "invalid range — expected 1h|3h|24h|week|month")
		return
	}
	now := time.Now().Unix()
	start := now - dur
	rows, err := h.traffic.QueryAggregate(start, now, q.Get("source"), q.Get("domain"), q.Get("chain"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// One parse per request, not per row: the tunnel subnet is a setting.
	//
	// VPS mode keeps every row: there the source of a bucket IS a remote client.
	// Breakdown is a router-only page today, so this only decides what an API
	// caller sees — but "the buckets are empty" is the wrong answer to give one.
	local, filterSources := h.tunnelPrefixes(), h.panelMode != "vps"
	out := trafficResponse{
		Range:   rng,
		StartTs: start,
		EndTs:   now,
		Buckets: make([]trafficBucket, 0, len(rows)),
	}
	for _, row := range rows {
		// Breakdown is a view of this box's devices. A public source is not one:
		// sing-box reports the occasional connection whose source is a remote
		// address, and unfiltered it became a client row named after a Google
		// front-end (#102). Dropping the row here keeps by-client, by-domain and
		// by-chain telling the same story. Rows recorded before the sampler
		// started filtering are on disk and stay there — this hides them.
		//
		// The series below is deliberately NOT filtered: it is the dashboard
		// graph, which both modes show, and in VPS mode every byte in it comes
		// from a source this test would reject.
		// An empty source is one of sing-box's own dials, shown as "unknown"
		// since long before this filter; it stays.
		if filterSources && row.Source != "" && !util.IsLocalClientIP(row.Source, local...) {
			continue
		}
		out.Buckets = append(out.Buckets, trafficBucket(row))
	}
	if q.Get("series") == "1" {
		// Only the source filter applies: the series is per (bucket, source)
		// and domain/chain would need their own query shape. An idle window
		// has no series key at all (omitempty) — callers default to [].
		series, err := h.traffic.QuerySourceHistory(start, now, q.Get("source"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out.Series = series
		out.Step = traffic.HistoryStep(dur)
	}
	writeSuccess(w, out)
}

// ResetTrafficHistory wipes both traffic tables — traffic_minute (the
// connection-level Breakdown series) and user_traffic (the per-user totals on
// the Users page and in the subscription userinfo header). Together they are
// the "all accumulated traffic statistics" the confirm dialog promises to
// erase, so both are part of one user action. Destructive — caller is expected
// to confirm before invoking. Live counters (Clash) and the sampler's
// in-memory state are not affected; future deltas continue to accumulate from
// now.
func (h *Handler) ResetTrafficHistory(w http.ResponseWriter, r *http.Request) {
	if h.traffic == nil {
		writeError(w, http.StatusServiceUnavailable, "traffic store not initialized")
		return
	}
	// Attempt both halves, then report every part that failed. Stopping after
	// the first error — or reporting success while one table still holds data —
	// would reproduce the exact defect this fixes: a reset the UI calls total
	// but that silently leaves stale numbers behind.
	var failed []string
	if err := h.traffic.Reset(); err != nil {
		failed = append(failed, "connection history: "+err.Error())
	}
	if err := h.traffic.ResetUsers(); err != nil {
		failed = append(failed, "per-user totals: "+err.Error())
	}
	if len(failed) > 0 {
		writeError(w, http.StatusInternalServerError, "reset traffic history — "+strings.Join(failed, "; "))
		return
	}
	writeSuccess(w, map[string]string{"message": "traffic history cleared"})
}
