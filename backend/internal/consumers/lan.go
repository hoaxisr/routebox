package consumers

import (
	"context"
	"net/netip"
	"sync"

	"routebox/backend/internal/clients"
	"routebox/backend/internal/traffic"
	"routebox/backend/internal/util"
)

// LanSource is the router's LAN devices. History is traffic_minute by source
// IP; live counters are Clash /connections folded into a per-source counter
// that never goes down when a connection closes. Router mode only — in VPS mode
// a connection's source is a remote user, already counted as a panel user.
type LanSource struct {
	Enabled     func() bool            // false in vps mode
	Clients     func() []clients.Entry // the named/seen device list
	AwgPrefixes func() []netip.Prefix  // awg.Manager.PeerPrefixes: addresses there are peers, not LAN devices
	Fetch       func() ([]traffic.ConnectionSample, error)
	Store       *traffic.Store

	mu    sync.Mutex
	last  map[string]traffic.ConnectionSample // conn id → sample at the previous read
	cumul map[string]Counter                  // source → bytes folded in so far
}

func (s *LanSource) Kind() string { return "lan" }

// isLan: a local client address outside every AWG peer prefix (the v4 subnet
// and, with the IPv6 broker, the ULA /64). On the singbox AWG backend a peer's
// tunnel IP shows up in traffic_minute too; it is counted as the peer, not a
// second time here. The address is canonicalised before the prefix test
// because Prefix.Contains refuses a 4-in-6 form that IsLocalClientIP accepts.
func (s *LanSource) isLan(ip string) bool {
	if !util.IsLocalClientIP(ip) {
		return false
	}
	a, err := netip.ParseAddr(util.CanonicalClientIP(ip))
	if err != nil {
		return false
	}
	if s.AwgPrefixes != nil {
		for _, p := range s.AwgPrefixes() {
			if p.Contains(a) {
				return false
			}
		}
	}
	return true
}

// List returns one row per LAN device the client list knows, named after the
// entry or, unnamed, after its IP. Devices are never suspended: there is no
// lifecycle for them, so every row is active.
func (s *LanSource) List(start, end int64) ([]Row, error) {
	if s.Enabled == nil || !s.Enabled() || s.Clients == nil {
		return nil, nil
	}
	var out []Row
	for _, e := range s.Clients() {
		if !s.isLan(e.IP) {
			continue
		}
		var up, down int64
		var hist []traffic.UserHistoryRow
		if s.Store != nil {
			var err error
			if up, down, err = s.Store.QuerySourceTotals(start, end, e.IP); err != nil {
				return nil, err
			}
			if hist, err = s.Store.QuerySourceHistory(start, end, e.IP); err != nil {
				return nil, err
			}
		}
		name := e.Name
		if name == "" {
			name = e.IP
		}
		out = append(out, Row{Kind: "lan", ID: e.IP, Name: name, State: StateActive,
			Upload: up, Download: down, History: hist, Address: e.IP})
	}
	return out, nil
}

// Counters folds the current Clash connections into per-source cumulative
// counters. Each connection contributes only what it grew since the previous
// read (all of it when first seen), so a connection closing between reads
// leaves the source's counter where it was instead of dropping it — a drop
// would read as a counter reset upstream and zero the rate. A connection whose
// own counter went down (Clash reused an id) contributes nothing that tick.
// Fetch errors are passed through: for a persistent down state the transport
// text is the same on every read.
func (s *LanSource) Counters(ctx context.Context) (map[string]Counter, error) {
	if s.Enabled == nil || !s.Enabled() || s.Fetch == nil {
		return nil, nil
	}
	conns, err := s.Fetch()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cumul == nil {
		s.cumul = map[string]Counter{}
	}
	next := make(map[string]traffic.ConnectionSample, len(conns))
	for _, c := range conns {
		if !s.isLan(c.Source) {
			continue
		}
		next[c.ID] = c
		d := Counter{Up: c.Upload, Down: c.Download} // new connection: all of it
		if p, ok := s.last[c.ID]; ok {
			d = Counter{}
			if c.Upload >= p.Upload {
				d.Up = c.Upload - p.Upload
			}
			if c.Download >= p.Download {
				d.Down = c.Download - p.Download
			}
		}
		acc := s.cumul[c.Source]
		acc.Up += d.Up
		acc.Down += d.Down
		s.cumul[c.Source] = acc
	}
	s.last = next
	out := make(map[string]Counter, len(s.cumul))
	for ip, c := range s.cumul {
		out[ip] = c
	}
	return out, nil
}
