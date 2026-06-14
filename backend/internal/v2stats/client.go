// Package v2stats is a minimal gRPC client for the amnezia-box v2ray_api
// StatsService. It vendors the fork's real generated proto (stats*.pb.go) and
// reads cumulative per-user traffic counters.
package v2stats

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Counters holds one user's cumulative uplink/downlink byte totals.
type Counters struct {
	Uplink   int64
	Downlink int64
}

// StatsQuerier is the minimal surface QueryUsers depends on, so tests inject
// canned results without a gRPC server. The real impl wraps StatsServiceClient.
type StatsQuerier interface {
	QueryStats(ctx context.Context, pattern string, reset bool) ([]*Stat, error)
}

// grpcQuerier is the production StatsQuerier backed by a live connection.
type grpcQuerier struct {
	conn   *grpc.ClientConn
	client StatsServiceClient
}

func (g grpcQuerier) QueryStats(ctx context.Context, pattern string, reset bool) ([]*Stat, error) {
	// The fork server reads Patterns ([]string) + Regexp, NOT the deprecated
	// singular Pattern field. Send substring (Regexp=false) match on the plural
	// field so "user>>>" actually filters server-side.
	resp, err := g.client.QueryStats(ctx, &QueryStatsRequest{
		Patterns: []string{pattern},
		Reset_:   reset,
		Regexp:   false,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetStat(), nil
}

// Client polls the StatsService for per-user cumulative counters.
type Client struct {
	q StatsQuerier
}

// Dial opens a (lazy) gRPC connection to the v2ray_api listen address (e.g.
// "127.0.0.1:8081"). The target is loopback-only plaintext h2c — insecure creds.
// Dial does NOT block: connection failure surfaces at the first QueryStats RPC.
// Caller must Close.
func Dial(addr string) (*Client, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("v2stats dial %s: %w", addr, err)
	}
	return &Client{q: grpcQuerier{conn: conn, client: NewStatsServiceClient(conn)}}, nil
}

// Close releases the underlying connection (if any).
func (c *Client) Close() error {
	if g, ok := c.q.(grpcQuerier); ok && g.conn != nil {
		return g.conn.Close()
	}
	return nil
}

// QueryUsers fetches cumulative per-user counters. reset=false: we diff the
// cumulative values ourselves (see traffic.UserSampler.computeUserDeltas).
func (c *Client) QueryUsers(ctx context.Context) (map[string]Counters, error) {
	stats, err := c.q.QueryStats(ctx, "user>>>", false)
	if err != nil {
		return nil, err
	}
	return aggregateStats(stats), nil
}

// QueryUsersTimeout is QueryUsers with a bounded context (used by the sampler so
// a hung socket can't stall the loop).
func (c *Client) QueryUsersTimeout(d time.Duration) (map[string]Counters, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return c.QueryUsers(ctx)
}

// aggregateStats folds raw stats into per-user counters. Non-user stats are
// ignored. PURE: no I/O.
func aggregateStats(stats []*Stat) map[string]Counters {
	out := make(map[string]Counters)
	for _, s := range stats {
		if s == nil {
			continue
		}
		name, uplink, ok := parseUserStat(s.GetName())
		if !ok {
			continue
		}
		c := out[name]
		if uplink {
			c.Uplink = s.GetValue()
		} else {
			c.Downlink = s.GetValue()
		}
		out[name] = c
	}
	return out
}
