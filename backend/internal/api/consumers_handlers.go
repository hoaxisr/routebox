package api

import (
	"net/http"
	"time"

	"routebox/backend/internal/consumers"
	"routebox/backend/internal/traffic"
)

// SetConsumers wires the per-kind traffic sources behind /api/consumers and the
// on-demand live sampler that reads their counters.
func (h *Handler) SetConsumers(sources []consumers.Source) {
	h.consumers = sources
	h.live = consumers.NewLive(sources)
}

type consumersResponse struct {
	Range   string          `json:"range"`
	StartTs int64           `json:"start_ts"`
	EndTs   int64           `json:"end_ts"`
	Step    int64           `json:"step"`
	Rows    []consumers.Row `json:"rows"`
}

// ListConsumers returns every consumer (panel users, AWG peers, Telegram
// clients, LAN devices) with totals and stepped history over a range
// (?range=1h|3h|24h|week|month, default 24h). PROTECTED.
func (h *Handler) ListConsumers(w http.ResponseWriter, r *http.Request) {
	rng := r.URL.Query().Get("range")
	if rng == "" {
		rng = "24h"
	}
	dur := rangeToSeconds(rng)
	if dur == 0 {
		writeError(w, http.StatusBadRequest, "invalid range — expected 1h|3h|24h|week|month")
		return
	}
	now := time.Now().Unix()
	start := now - dur
	writeSuccess(w, consumersResponse{Range: rng, StartTs: start, EndTs: now, Step: traffic.HistoryStep(dur),
		Rows: consumers.List(h.consumers, start, now)})
}

// LiveConsumers returns the current per-consumer rates and the last minute of
// them. Each call keeps the sampler awake; it sleeps 15 s after the last one.
// PROTECTED.
func (h *Handler) LiveConsumers(w http.ResponseWriter, r *http.Request) {
	if h.live == nil {
		h.SetConsumers(nil)
	}
	writeSuccess(w, h.live.Snapshot())
}
