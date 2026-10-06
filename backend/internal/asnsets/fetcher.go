package asnsets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	neturl "net/url"
	"time"
)

const (
	fetchAttempts = 3
	maxBodyBytes  = 16 << 20
)

// Fetcher reads RIPEstat's public data API. BaseURL is injectable for tests.
type Fetcher struct {
	BaseURL    string
	Client     *http.Client
	RetryDelay time.Duration
}

// NewFetcher targets the public RIPEstat endpoint with a 20 s per-request
// timeout and a 2 s base retry delay.
func NewFetcher() *Fetcher {
	return &Fetcher{BaseURL: "https://stat.ripe.net", Client: &http.Client{Timeout: 20 * time.Second}, RetryDelay: 2 * time.Second}
}

// Prefixes returns what the AS announces (RIPEstat's default two-week window).
// Unparsable entries are skipped; an empty list is a valid answer — the caller
// decides whether that is an error.
func (f *Fetcher) Prefixes(ctx context.Context, asn uint32) ([]netip.Prefix, error) {
	var doc struct {
		Data struct {
			Prefixes []struct {
				Prefix string `json:"prefix"`
			} `json:"prefixes"`
		} `json:"data"`
	}
	if err := f.get(ctx, "announced-prefixes", asn, &doc); err != nil {
		return nil, err
	}
	out := make([]netip.Prefix, 0, len(doc.Data.Prefixes))
	for _, p := range doc.Data.Prefixes {
		if pp, err := netip.ParsePrefix(p.Prefix); err == nil {
			out = append(out, pp)
		}
	}
	return out, nil
}

// Holder returns the registered holder name ("CLOUDFLARENET - Cloudflare, Inc.").
func (f *Fetcher) Holder(ctx context.Context, asn uint32) (string, error) {
	var doc struct {
		Data struct {
			Holder string `json:"holder"`
		} `json:"data"`
	}
	if err := f.get(ctx, "as-overview", asn, &doc); err != nil {
		return "", err
	}
	return doc.Data.Holder, nil
}

// get fetches one RIPEstat data call into out. Only transport errors that are
// not timeouts are retried — a non-2xx or an error envelope will not change on
// a second ask (same rule as subscriptions.Fetch). The back-off between
// attempts grows a little and stops early when ctx is cancelled.
func (f *Fetcher) get(ctx context.Context, call string, asn uint32, out any) error {
	u := fmt.Sprintf("%s/data/%s/data.json?resource=%s&sourceapp=routebox", f.BaseURL, call, FormatASN(asn))
	var err error
	for attempt := 0; ; attempt++ {
		if err = f.getOnce(ctx, u, out); err == nil {
			return nil
		}
		var ue *neturl.Error
		if attempt == fetchAttempts-1 || !errors.As(err, &ue) || ue.Timeout() {
			return fmt.Errorf("RIPEstat %s %s: %w", call, FormatASN(asn), err)
		}
		timer := time.NewTimer(time.Duration(attempt+1) * f.RetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("RIPEstat %s %s: %w", call, FormatASN(asn), ctx.Err())
		case <-timer.C:
		}
	}
}

func (f *Fetcher) getOnce(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "RouteBox")
	resp, err := f.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return err
	}
	var env struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("bad response: %w", err)
	}
	if env.Status != "ok" {
		return fmt.Errorf("status %q", env.Status)
	}
	return json.Unmarshal(body, out)
}
