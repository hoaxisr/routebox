package api

import (
	"net/http"
	"time"
)

type trafficResponse struct {
	Range   string          `json:"range"`
	StartTs int64           `json:"start_ts"`
	EndTs   int64           `json:"end_ts"`
	Buckets []trafficBucket `json:"buckets"`
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
	out := trafficResponse{
		Range:   rng,
		StartTs: start,
		EndTs:   now,
		Buckets: make([]trafficBucket, len(rows)),
	}
	for i, row := range rows {
		out.Buckets[i] = trafficBucket(row)
	}
	writeSuccess(w, out)
}

// ResetTrafficHistory wipes the traffic_minute table. Destructive — caller
// is expected to confirm before invoking. Live counters (Clash) and the
// sampler's in-memory state are not affected; future deltas continue to
// accumulate from now.
func (h *Handler) ResetTrafficHistory(w http.ResponseWriter, r *http.Request) {
	if h.traffic == nil {
		writeError(w, http.StatusServiceUnavailable, "traffic store not initialized")
		return
	}
	if err := h.traffic.Reset(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeSuccess(w, map[string]string{"message": "traffic history cleared"})
}
