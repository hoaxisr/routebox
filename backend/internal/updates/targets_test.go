package updates

import (
	"strings"
	"testing"
)

// Asset names as published by the fork's release-entware.yml workflow
// (singbox-<ver>-<matrix.arch>). Update this list when the matrix changes —
// a mismatch here means the in-panel update silently breaks on that arch.
var releaseAssets = []string{
	"singbox-1.14.0-beta.1-awgm.2-linux-amd64",
	"singbox-1.14.0-beta.1-awgm.2-aarch64-3.10",
	"singbox-1.14.0-beta.1-awgm.2-mipsel-3.4",
}

func TestAmneziaTargetMatchesReleaseAssets(t *testing.T) {
	target := AmneziaTarget(nil, nil, nil, nil)

	for _, arch := range []string{"amd64", "arm64", "mipsle"} {
		suffix, ok := target.AssetSuffix(arch)
		if !ok {
			t.Fatalf("arch %s: unsupported", arch)
		}
		matched := false
		for _, name := range releaseAssets {
			if strings.HasSuffix(name, suffix) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("arch %s: suffix %q matches no release asset %v", arch, suffix, releaseAssets)
		}
	}
}
