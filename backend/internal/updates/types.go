// Package updates implements GitHub-release checking and atomic binary
// self-update for amnezia-box and RouteBox itself.
package updates

import "time"

// Target describes one updatable binary.
type Target struct {
	Name string // "amnezia-box" | "routebox"
	Repo string // "owner/repo" on GitHub
	// AssetSuffix maps a GOARCH value to the release asset name suffix.
	// ok=false means the target is unsupported on this arch (UI hides it).
	AssetSuffix func(arch string) (suffix string, ok bool)
	// BinaryPath returns the current on-disk path of the binary.
	BinaryPath func() string
	// CurrentVersion returns the installed version string.
	CurrentVersion func() (string, error)
	// Restart restarts the managed process after the swap. nil for
	// self-update targets — the API handler owns process exit (see Updater).
	Restart func() error
	// SelfUpdate marks the RouteBox binary itself: Apply skips Restart and
	// reports whether a supervisor (systemd) will respawn the process.
	SelfUpdate bool
}

// ReleaseInfo is the parsed result of GET /repos/<repo>/releases/latest.
type ReleaseInfo struct {
	Version     string    `json:"version"` // normalized: no "v" prefix, no "-awg2.0"
	TagName     string    `json:"tag_name"`
	Notes       string    `json:"notes"`
	AssetName   string    `json:"asset_name"`
	AssetURL    string    `json:"asset_url"`
	Sha256URL   string    `json:"sha256_url,omitempty"` // <asset>.sha256 or checksums.txt
	PublishedAt time.Time `json:"published_at"`
}
