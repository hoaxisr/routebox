package consumers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"routebox/backend/internal/quota"
	"routebox/backend/internal/traffic"
	"routebox/backend/internal/users"
	"routebox/backend/internal/v2stats"
)

// UserStats is the slice of *v2stats.Client the live view needs.
type UserStats interface {
	QueryUsersTimeout(d time.Duration) (map[string]v2stats.Counters, error)
}

// errNoUserStats is returned while users exist but v2ray_api was never dialled.
// It is a fixed value on purpose: the live sampler logs an unavailable source
// only when its error text changes, so a persistent down state must read the
// same every tick.
var errNoUserStats = errors.New("v2ray_api is not connected")

// UserSource is the panel users: one row per registry user, its traffic summed
// over every name it is accounted under (PanelUser.TrafficNames).
type UserSource struct {
	// Enabled is false outside vps mode: panel users (and their Manage link)
	// only exist there. nil means enabled — unlike LanSource, whose nil is
	// off, this source was wired before the gate existed and its callers
	// (tests included) read "no gate" as "always on".
	Enabled func() bool
	Users   *users.Manager
	Store   *traffic.Store
	Stats   UserStats // nil when v2ray_api could not be dialled
	Now     func() int64
}

func (s *UserSource) enabled() bool { return s.Enabled == nil || s.Enabled() }

func (s *UserSource) Kind() string { return "user" }

func (s *UserSource) now() int64 {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().Unix()
}

// List returns one row per panel user. Manager.List is already sorted by name
// then ID, so the page order is stable without sorting here.
func (s *UserSource) List(start, end int64) ([]Row, error) {
	if !s.enabled() || s.Users == nil {
		return nil, nil
	}
	now := s.now()
	var out []Row
	for _, u := range s.Users.List() {
		up, down, hist, err := keyed(s.Store, start, end, u.TrafficNames())
		if err != nil {
			return nil, err
		}
		state, why := stateOf(quota.State(u.QuotaBytes, u.UsedRx, u.UsedTx, u.Enabled, u.ExpiresAt, now))
		out = append(out, Row{
			Kind: "user", ID: u.ID, Name: u.Name, Tags: protocols(u.Bindings),
			State: state, StateReason: why, Upload: up, Download: down, History: hist,
			QuotaBytes: u.QuotaBytes, Used: u.UsedRx + u.UsedTx, ExpiresAt: u.ExpiresAt,
		})
	}
	return out, nil
}

// Counters reads the cumulative v2ray_api stats and folds them per user. A
// disabled source or no users means nothing to count (nil, nil); users
// without a reachable v2ray_api is an unavailable source, which is an error.
func (s *UserSource) Counters(ctx context.Context) (map[string]Counter, error) {
	if !s.enabled() || s.Users == nil {
		return nil, nil
	}
	list := s.Users.List()
	if len(list) == 0 {
		return nil, nil
	}
	if s.Stats == nil {
		return nil, errNoUserStats
	}
	snap, err := s.Stats.QueryUsersTimeout(sourceBudget)
	if err != nil {
		// gRPC status text is fixed per cause (no timestamps or counters), so
		// the wrapped message stays stable for as long as the cause does.
		return nil, fmt.Errorf("v2ray_api: %w", err)
	}
	out := make(map[string]Counter, len(list))
	for _, u := range list {
		var c Counter
		for _, n := range u.TrafficNames() {
			c.Up += snap[n].Uplink
			c.Down += snap[n].Downlink
		}
		out[u.ID] = c
	}
	return out, nil
}

// protocols lists a user's binding protocols once each, in binding order.
func protocols(bs []users.Binding) []string {
	seen := map[string]bool{}
	var out []string
	for _, b := range bs {
		if b.Protocol != "" && !seen[b.Protocol] {
			seen[b.Protocol] = true
			out = append(out, b.Protocol)
		}
	}
	return out
}
