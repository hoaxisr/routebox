package consumers

import (
	"context"

	"routebox/backend/internal/awg"
	"routebox/backend/internal/traffic"
)

// AwgSource is the AWG server's peers, on either backend. History is the
// sweep's awg:<pubkey> rows (accurate tunnel bytes, not the by-IP guess the
// old monitor used); live counters come straight from the interface/endpoint.
type AwgSource struct {
	Peers func(ctx context.Context) []awg.PeerSummary
	Live  func(ctx context.Context) (map[string]awg.PeerUsage, error)
	Store *traffic.Store
}

func (s *AwgSource) Kind() string { return "awg" }

// List returns one row per peer. Used is the sweep's rx+tx against the quota,
// the same pair the peer pages show; Upload/Download are the range totals.
func (s *AwgSource) List(start, end int64) ([]Row, error) {
	if s.Peers == nil {
		return nil, nil
	}
	var out []Row
	for _, p := range s.Peers(context.Background()) {
		up, down, hist, err := keyed(s.Store, start, end, []string{awg.TrafficKey(p.PublicKey)})
		if err != nil {
			return nil, err
		}
		state, why := stateOf(p.SuspendReason)
		out = append(out, Row{
			Kind: "awg", ID: p.PublicKey, Name: p.Name, State: state, StateReason: why, Online: p.Online,
			Upload: up, Download: down, History: hist, Address: p.Address, LastHandshake: p.LastHandshake,
			QuotaBytes: p.QuotaBytes, Used: p.Rx + p.Tx, ExpiresAt: p.ExpiresAt,
		})
	}
	return out, nil
}

// Counters is the cumulative per-peer bytes since the interface/endpoint came
// up, already in the client's direction (PeerUsage). A server that is not
// enabled yields nil, nil; a read failure is passed through — LiveCounters'
// texts are static, so the live sampler logs a down state once.
func (s *AwgSource) Counters(ctx context.Context) (map[string]Counter, error) {
	if s.Live == nil {
		return nil, nil
	}
	live, err := s.Live(ctx)
	if err != nil || live == nil {
		return nil, err
	}
	out := make(map[string]Counter, len(live))
	for pk, u := range live {
		out[pk] = Counter{Up: u.Up, Down: u.Down}
	}
	return out, nil
}
