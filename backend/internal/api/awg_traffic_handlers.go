package api

import (
	"net/http"
	"net/netip"
	"time"

	"routebox/backend/internal/awg"
	"routebox/backend/internal/traffic"
)

// awgPeerTrafficRow is one peer's entry in GET /api/awg/peers/traffic. It mirrors
// the per-user shape of /api/users/{id}/traffic (totals + per-bucket series) so
// the monitor page renders peers and panel users the same way (#40).
type awgPeerTrafficRow struct {
	PublicKey     string                   `json:"public_key"`
	Name          string                   `json:"name"`
	Address       string                   `json:"address"` // "<ip>/32" as stored
	Source        string                   `json:"source"`  // the tunnel IP the bytes are keyed by
	LastHandshake int64                    `json:"last_handshake"`
	Online        bool                     `json:"online"`
	Upload        int64                    `json:"upload"`
	Download      int64                    `json:"download"`
	History       []traffic.UserHistoryRow `json:"history"`
}

// peerSource strips the /32 (or /128) mask from a peer address, yielding the
// `source` key traffic_minute records connections under. Same normalisation the
// peer-delete purge does.
func peerSource(addr string) string {
	if addr == "" {
		return ""
	}
	if pfx, err := netip.ParsePrefix(addr); err == nil {
		return pfx.Addr().String()
	}
	return addr
}

// GetAWGPeersTraffic returns every AWG peer with its traffic over a range
// (?range=1h|3h|24h|week|month; default 24h).
//
// The bytes come from traffic_minute keyed by the peer's tunnel IP — the rows
// the Breakdown panel reads — NOT from user_traffic: sing-box's StatsService
// counts inbound USERS, and an AWG peer is not one. That has a consequence worth
// knowing: this only sees traffic that transits sing-box, which is the case for
// the singbox AWG backend. On the kernel backend the tunnel is a kernel
// interface NAT'ed by iptables, sing-box never sees the packets, and the series
// is empty — the live Rx/Tx counters on the AWG page are the source of truth
// there. PROTECTED.
func (h *Handler) GetAWGPeersTraffic(w http.ResponseWriter, r *http.Request) {
	if h.awg == nil {
		writeError(w, http.StatusServiceUnavailable, "awg not available")
		return
	}
	peers := h.awg.ListPeers(r.Context())
	out := make([]awgPeerTrafficRow, 0, len(peers))
	if h.traffic == nil {
		// No history store: report the peers with zeroed traffic rather than
		// failing, matching GetUserTraffic's additive behaviour.
		for _, p := range peers {
			out = append(out, newAWGPeerTrafficRow(p, nil, 0, 0))
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
	for _, p := range peers {
		src := peerSource(p.Address)
		if src == "" {
			out = append(out, newAWGPeerTrafficRow(p, nil, 0, 0))
			continue
		}
		up, down, err := h.traffic.QuerySourceTotals(start, now, src)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		hist, err := h.traffic.QuerySourceHistory(start, now, src)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, newAWGPeerTrafficRow(p, hist, up, down))
	}
	writeSuccess(w, out)
}

// newAWGPeerTrafficRow assembles one row, normalising a nil history to an empty
// slice so the JSON is always an array.
func newAWGPeerTrafficRow(p awg.PeerSummary, hist []traffic.UserHistoryRow, up, down int64) awgPeerTrafficRow {
	if hist == nil {
		hist = []traffic.UserHistoryRow{}
	}
	return awgPeerTrafficRow{
		PublicKey:     p.PublicKey,
		Name:          p.Name,
		Address:       p.Address,
		Source:        peerSource(p.Address),
		LastHandshake: p.LastHandshake,
		Online:        p.Online,
		Upload:        up,
		Download:      down,
		History:       hist,
	}
}
