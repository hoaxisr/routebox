package updates

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultAPIBase = "https://api.github.com"

// CachedCheck is the stored result of the last Check for a target.
type CachedCheck struct {
	Release     *ReleaseInfo `json:"release,omitempty"`
	LastChecked time.Time    `json:"last_checked"`
	Error       string       `json:"error,omitempty"`
}

// Checker queries GitHub releases/latest and caches the result per target.
type Checker struct {
	apiBase string
	client  *http.Client
	arch    string // runtime.GOARCH, overridable in tests

	mu    sync.Mutex
	cache map[string]CachedCheck
}

// NewChecker builds a Checker. UPDATES_API_BASE overrides the GitHub API
// base URL (tests / e2e against a local fake); read once at construction.
func NewChecker() *Checker {
	base := os.Getenv("UPDATES_API_BASE")
	if base == "" {
		base = defaultAPIBase
	}
	return &Checker{
		apiBase: strings.TrimRight(base, "/"),
		client:  &http.Client{Timeout: 15 * time.Second},
		arch:    runtime.GOARCH,
		cache:   make(map[string]CachedCheck),
	}
}

// Check queries releases/latest for the target and updates the cache.
func (c *Checker) Check(t Target) (ReleaseInfo, error) {
	info, err := c.fetch(t)
	entry := CachedCheck{LastChecked: time.Now()}
	if err != nil {
		entry.Error = err.Error()
	} else {
		entry.Release = &info
	}
	c.mu.Lock()
	c.cache[t.Name] = entry
	c.mu.Unlock()
	return info, err
}

// Cached returns the last Check result for a target name.
func (c *Checker) Cached(name string) (CachedCheck, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.cache[name]
	return entry, ok
}

func (c *Checker) fetch(t Target) (ReleaseInfo, error) {
	suffix, ok := t.AssetSuffix(c.arch)
	if !ok {
		return ReleaseInfo{}, fmt.Errorf("%s: no release asset for arch %s", t.Name, c.arch)
	}

	url := fmt.Sprintf("%s/repos/%s/releases/latest", c.apiBase, t.Repo)
	resp, err := c.client.Get(url)
	if err != nil {
		return ReleaseInfo{}, fmt.Errorf("%s: %w", t.Repo, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return ReleaseInfo{}, fmt.Errorf("%s: no releases found", t.Repo)
	case http.StatusForbidden, http.StatusTooManyRequests:
		return ReleaseInfo{}, fmt.Errorf("%s: GitHub API rate limit exceeded, try again later", t.Repo)
	default:
		return ReleaseInfo{}, fmt.Errorf("%s: GitHub API returned %d", t.Repo, resp.StatusCode)
	}

	var rel struct {
		TagName     string    `json:"tag_name"`
		Body        string    `json:"body"`
		PublishedAt time.Time `json:"published_at"`
		Assets      []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return ReleaseInfo{}, fmt.Errorf("%s: parse release JSON: %w", t.Repo, err)
	}

	info := ReleaseInfo{
		Version:     NormalizeVersion(rel.TagName),
		TagName:     rel.TagName,
		Notes:       rel.Body,
		PublishedAt: rel.PublishedAt,
	}
	for _, a := range rel.Assets {
		if strings.HasSuffix(a.Name, suffix) && !strings.HasSuffix(a.Name, ".sha256") {
			info.AssetName = a.Name
			info.AssetURL = a.BrowserDownloadURL
			break
		}
	}
	if info.AssetURL == "" {
		return ReleaseInfo{}, fmt.Errorf("%s: release %s has no asset matching %q", t.Repo, rel.TagName, suffix)
	}
	// Checksum asset: prefer <asset>.sha256 (routebox), fall back to
	// checksums.txt (amnezia-box). Old releases have neither — that's fine.
	for _, a := range rel.Assets {
		if a.Name == info.AssetName+".sha256" {
			info.Sha256URL = a.BrowserDownloadURL
			break
		}
	}
	if info.Sha256URL == "" {
		for _, a := range rel.Assets {
			if a.Name == "checksums.txt" {
				info.Sha256URL = a.BrowserDownloadURL
				break
			}
		}
	}
	return info, nil
}

// NormalizeVersion strips the "v" prefix and the "-awg2.0" build suffix.
func NormalizeVersion(v string) string {
	v = strings.TrimSpace(strings.TrimPrefix(v, "v"))
	return strings.TrimSuffix(v, "-awg2.0")
}

// CompareVersions compares two semver-ish versions after normalization.
// Returns -1, 0 or 1. Missing segments count as 0 ("0.17" == "0.17.0").
func CompareVersions(a, b string) int {
	as := strings.Split(NormalizeVersion(a), ".")
	bs := strings.Split(NormalizeVersion(b), ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		ai, bi := segmentNum(as, i), segmentNum(bs, i)
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

// segmentNum returns the leading numeric value of segment i ("9-beta" → 9).
func segmentNum(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	s := parts[i]
	j := 0
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	n, _ := strconv.Atoi(s[:j])
	return n
}
