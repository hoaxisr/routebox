// Package consumers answers "who moves how many bytes through this box": panel
// users, AWG peers, Telegram clients and LAN devices as one row shape, so the
// monitor page does not need to know where each number comes from.
package consumers

import (
	"context"
	"log"

	"routebox/backend/internal/quota"
	"routebox/backend/internal/traffic"
)

// Row is one consumer over a range. Upload/Download are the CLIENT's view:
// upload is what the consumer sent, download is what it received.
type Row struct {
	Kind          string                   `json:"kind"`
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	Tags          []string                 `json:"tags,omitempty"`
	State         string                   `json:"state"`
	StateReason   string                   `json:"state_reason,omitempty"`
	Online        bool                     `json:"online,omitempty"`
	Upload        int64                    `json:"upload"`
	Download      int64                    `json:"download"`
	History       []traffic.UserHistoryRow `json:"history"`
	Address       string                   `json:"address,omitempty"`
	LastHandshake int64                    `json:"last_handshake,omitempty"`
	QuotaBytes    int64                    `json:"quota_bytes,omitempty"`
	Used          int64                    `json:"used,omitempty"`
	ExpiresAt     int64                    `json:"expires_at,omitempty"`
}

// Row states. Suspended rows carry the cause in StateReason.
const (
	StateActive    = "active"
	StateSuspended = "suspended"
	StateDisabled  = "disabled"
)

// Counter is a cumulative byte count in the client's direction.
type Counter struct{ Up, Down int64 }

// Source is one kind of consumer. A source that does not apply on this box
// (no users in router mode, AWG off) returns no rows and nil counters, not an
// error; an error means it applies but could not be read.
type Source interface {
	Kind() string
	List(start, end int64) ([]Row, error)
	Counters(ctx context.Context) (map[string]Counter, error)
}

// List asks every source for its rows. A failing source is logged and left out:
// one broken kind must not take the whole page with it.
func List(sources []Source, start, end int64) []Row {
	out := []Row{}
	for _, s := range sources {
		rows, err := s.List(start, end)
		if err != nil {
			log.Printf("consumers: %s: %v", s.Kind(), err)
			continue
		}
		for i := range rows {
			if rows[i].History == nil {
				rows[i].History = []traffic.UserHistoryRow{}
			}
		}
		out = append(out, rows...)
	}
	return out
}

// stateOf maps the shared suspension verdict onto the row state.
func stateOf(r quota.Reason) (string, string) {
	switch r {
	case quota.ReasonNone:
		return StateActive, ""
	case quota.ReasonManual:
		return StateDisabled, ""
	}
	return StateSuspended, string(r)
}

// keyed reads totals + stepped history for user_traffic keys. A nil store
// (no traffic.db on this install) is zeros, not an error.
func keyed(store *traffic.Store, start, end int64, keys []string) (int64, int64, []traffic.UserHistoryRow, error) {
	if store == nil {
		return 0, 0, nil, nil
	}
	up, down, err := store.QueryKeysTotals(start, end, keys)
	if err != nil {
		return 0, 0, nil, err
	}
	hist, err := store.QueryKeysHistory(start, end, keys)
	return up, down, hist, err
}
