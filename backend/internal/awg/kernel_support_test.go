package awg

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestKernelBackendUnsupported(t *testing.T) {
	present := func(names ...string) func(string) (string, error) {
		set := map[string]bool{}
		for _, n := range names {
			set[n] = true
		}
		return func(file string) (string, error) {
			if set[file] {
				return "/usr/bin/" + file, nil
			}
			return "", errors.New("not found")
		}
	}
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })

	t.Run("tools and systemd present => supported", func(t *testing.T) {
		lookPath = present("awg-quick", "systemctl")
		if reason := KernelBackendUnsupported(); reason != "" {
			t.Fatalf("expected supported, got %q", reason)
		}
	})

	// A host with the module loaded, the tools installed and systemd running is
	// supported whether or not it is a container — the check must never key on
	// the runtime.
	t.Run("no amneziawg-tools => unsupported, and says which tool", func(t *testing.T) {
		lookPath = present("systemctl")
		reason := KernelBackendUnsupported()
		if reason == "" {
			t.Fatal("missing awg-quick must be reported")
		}
		if want := "awg-quick"; !strings.Contains(reason, want) {
			t.Errorf("reason %q does not name %q", reason, want)
		}
	})

	t.Run("no systemd => unsupported, and says why systemd matters", func(t *testing.T) {
		lookPath = present("awg-quick")
		reason := KernelBackendUnsupported()
		if reason == "" {
			t.Fatal("missing systemd must be reported")
		}
		if !strings.Contains(reason, "awg-quick@") {
			t.Errorf("reason %q should name the unit that needs systemd", reason)
		}
	})
}

// The production seam must be the real thing.
func TestLookPathDefaultsToExec(t *testing.T) {
	if _, err := lookPath("sh"); err != nil {
		if _, execErr := exec.LookPath("sh"); execErr == nil {
			t.Fatal("lookPath is not wired to exec.LookPath")
		}
	}
}
