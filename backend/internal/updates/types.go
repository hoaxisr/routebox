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
	// Preflight runs after verify but BEFORE the binary swap, receiving the
	// path of the downloaded (verified) new binary. Returning an error aborts
	// the update with the running service completely untouched — the wiring
	// layer uses it to `check` the existing config against the NEW binary
	// (and self-heal RouteBox-owned blocks the new build dropped, e.g.
	// experimental.v2ray_api), so a config the new binary rejects surfaces a
	// clear message instead of a restart-fail + rollback. nil = no preflight.
	Preflight func(newBinaryPath string) error
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
