package api

import (
	"net/http"
	"time"

	"routebox/backend/internal/mtproto"
	"routebox/backend/internal/traffic"
)

// mtprotoClientTrafficRow is one client's traffic over a range, in the same
// totals-plus-series shape /api/awg/peers/traffic uses for AWG peers, so the
// panel renders the two the same way.
type mtprotoClientTrafficRow struct {
	Name     string                   `json:"name"`
	Upload   int64                    `json:"upload"`
	Download int64                    `json:"download"`
	History  []traffic.UserHistoryRow `json:"history"`
}

// GetMtprotoClientsTraffic returns every client with its traffic over a range
// (?range=1h|3h|24h|week|month; default 24h).
//
// The bytes come from user_traffic under the mtproto: prefix — the rows the
// flusher writes. The prefix is what keeps a client and a panel user of the
// same name apart, so it is applied here rather than matching on the bare name.
//
// Unlike the AWG equivalent, these counts are always complete: they are taken
// from the proxy's own relay rather than inferred from what sing-box happened
// to see. PROTECTED.
func (h *Handler) GetMtprotoClientsTraffic(w http.ResponseWriter, r *http.Request) {
	if !h.mtprotoReady(w) {
		return
	}

	clients := h.mtproto.Store().List()
	out := make([]mtprotoClientTrafficRow, 0, len(clients))

	// No history store on this install: report the roster at zero rather than
	// failing the page, matching GetAWGPeersTraffic and GetUserTraffic.
	if h.traffic == nil {
		for _, c := range clients {
			out = append(out, mtprotoClientTrafficRow{Name: c.Name, History: []traffic.UserHistoryRow{}})
		}

		writeSuccess(w, out)

		return
	}

	dur := rangeToSeconds(r.URL.Query().Get("range"))
	if dur == 0 {
		dur = 86400 // default 24h
	}

	now := time.Now().Unix()
	start := now - dur

	for _, c := range clients {
		key := mtproto.TrafficKey(c.Name)

		up, down, err := h.traffic.QueryUserTotals(start, now, key)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())

			return
		}

		hist, err := h.traffic.QueryUserHistory(start, now, key)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())

			return
		}

		// Normalised so the JSON is always an array; a null would make the
		// sparkline component error instead of drawing nothing.
		if hist == nil {
			hist = []traffic.UserHistoryRow{}
		}

		out = append(out, mtprotoClientTrafficRow{
			Name:     c.Name,
			Upload:   up,
			Download: down,
			History:  hist,
		})
	}

	writeSuccess(w, out)
}
