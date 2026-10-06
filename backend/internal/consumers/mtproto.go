package consumers

import (
	"context"
	"sort"
	"time"

	"routebox/backend/internal/mtproto"
	"routebox/backend/internal/quota"
	"routebox/backend/internal/traffic"
)

// MtprotoSource is the Telegram proxy's clients. History is the flusher's
// mtproto:<name> rows; live counters are the relay's own (Cumulative).
type MtprotoSource struct {
	Clients func() []mtproto.Client
	Events  func() *mtproto.EventStream // nil until the proxy first starts
	Store   *traffic.Store
	Now     func() int64
}

func (s *MtprotoSource) Kind() string { return "mtproto" }

func (s *MtprotoSource) now() int64 {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().Unix()
}

// List returns one row per client, sorted by name. The roster is copied
// before sorting so the caller's slice is left as it was.
func (s *MtprotoSource) List(start, end int64) ([]Row, error) {
	if s.Clients == nil {
		return nil, nil
	}
	now := s.now()
	clients := append([]mtproto.Client(nil), s.Clients()...)
	sort.Slice(clients, func(i, j int) bool { return clients[i].Name < clients[j].Name })
	var out []Row
	for _, c := range clients {
		up, down, hist, err := keyed(s.Store, start, end, []string{mtproto.TrafficKey(c.Name)})
		if err != nil {
			return nil, err
		}
		state, why := stateOf(quota.State(0, 0, 0, c.Enabled, c.ExpiresAt, now))
		out = append(out, Row{Kind: "mtproto", ID: c.Name, Name: c.Name, State: state, StateReason: why,
			Upload: up, Download: down, History: hist, ExpiresAt: c.ExpiresAt})
	}
	return out, nil
}

// Counters is the relay's cumulative per-client bytes. A proxy that never
// started has nothing to count: nil, nil rather than an error.
func (s *MtprotoSource) Counters(ctx context.Context) (map[string]Counter, error) {
	if s.Events == nil {
		return nil, nil
	}
	es := s.Events()
	if es == nil {
		return nil, nil
	}
	cum := es.Cumulative()
	out := make(map[string]Counter, len(cum))
	for name, t := range cum {
		out[name] = Counter{Up: t.Upload, Down: t.Download}
	}
	return out, nil
}
