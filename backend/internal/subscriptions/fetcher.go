package subscriptions

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	fetchTimeout  = 30 * time.Second
	maxBodyBytes  = 2 << 20 // 2 MiB
	userAgent     = "RouteBox"
	urltestURL    = "https://www.gstatic.com/generate_204"
	urltestPeriod = "3m"
	tagSeparator  = " · "
)

// ConfigMerger is the slice of config.Manager the fetcher needs. An interface
// avoids importing config; *config.Manager satisfies it.
type ConfigMerger interface {
	ReplaceSubscriptionOutbounds(groupTag, nodePrefix string, nodes []map[string]interface{}, group map[string]interface{}) error
}

// Fetch downloads a subscription body (timeout, RouteBox UA, non-2xx guard, 2MiB cap).
func Fetch(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := (&http.Client{Timeout: fetchTimeout}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("subscription server returned %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
}

// Sanitize keeps alphanumerics, spaces, '-' and '_' for readable display tags.
// EXPORTED so the API Delete handler derives the same group tag (single source
// of truth). Distinct from the store's slugify.
func Sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == ' ':
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// Refresh fetches, parses and merges a subscription into the config draft. Zero
// usable nodes returns an error WITHOUT touching cfg (protects against a
// transient empty response wiping the group).
func Refresh(sub Subscription, cfg ConfigMerger) (nodeCount, skipped int, err error) {
	body, err := Fetch(sub.URL)
	if err != nil {
		return 0, 0, err
	}
	parsed, skipped := ParseLinks(decodeSubscription(body))
	if len(parsed) == 0 {
		return 0, skipped, fmt.Errorf("no usable nodes")
	}
	groupTag := Sanitize(sub.Name)
	nodePrefix := groupTag + tagSeparator
	nodes := make([]map[string]interface{}, 0, len(parsed))
	tags := make([]interface{}, 0, len(parsed))
	used := make(map[string]bool, len(parsed))
	for _, p := range parsed {
		base := nodePrefix + Sanitize(p.Name)
		tag := base
		for i := 2; used[tag]; i++ {
			tag = fmt.Sprintf("%s-%d", base, i)
		}
		used[tag] = true
		p.Outbound["tag"] = tag
		nodes = append(nodes, p.Outbound)
		tags = append(tags, tag)
	}
	group := map[string]interface{}{"type": "urltest", "tag": groupTag, "outbounds": tags, "url": urltestURL, "interval": urltestPeriod}
	if err := cfg.ReplaceSubscriptionOutbounds(groupTag, nodePrefix, nodes, group); err != nil {
		return 0, skipped, err
	}
	return len(nodes), skipped, nil
}
