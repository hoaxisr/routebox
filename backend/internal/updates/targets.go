package updates

import "os"

// AmneziaTarget builds the amnezia-box (sing-box AWG fork) update target.
// Release assets: sing-box-<ver>-awg2.0-{linux-amd64|entware-mipsel|entware-aarch64}.
// preflight (may be nil) validates the existing config against the NEW binary
// before the swap — see Target.Preflight.
func AmneziaTarget(binaryPath func() string, currentVersion func() (string, error), restart func() error, preflight func(newBinaryPath string) error) Target {
	return Target{
		Name: "amnezia-box",
		Repo: "hoaxisr/amnezia-box",
		Preflight: preflight,
		AssetSuffix: func(arch string) (string, bool) {
			switch arch {
			case "amd64":
				return "linux-amd64", true
			case "mipsle":
				return "entware-mipsel", true
			case "arm64":
				return "entware-aarch64", true
			}
			return "", false
		},
		BinaryPath:     binaryPath,
		CurrentVersion: currentVersion,
		Restart:        restart,
	}
}

// RouteBoxTarget builds the self-update target. Only linux-amd64 releases
// exist; other arches report unsupported (UI hides the card).
func RouteBoxTarget(version string) Target {
	return Target{
		Name: "routebox",
		Repo: "hoaxisr/routebox",
		AssetSuffix: func(arch string) (string, bool) {
			if arch == "amd64" {
				return "routebox-linux-amd64", true
			}
			return "", false
		},
		BinaryPath: func() string {
			p, err := os.Executable()
			if err != nil {
				return ""
			}
			return p
		},
		CurrentVersion: func() (string, error) { return version, nil },
		SelfUpdate:     true,
	}
}
