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

	origCap := capEffective
	capEffective = func() string { return "0000003fffffffff" } // everything, incl. CAP_NET_ADMIN
	t.Cleanup(func() { capEffective = origCap })

	t.Run("tools present and CAP_NET_ADMIN held => supported", func(t *testing.T) {
		lookPath = present("awg-quick")
		if reason := KernelBackendUnsupported(); reason != "" {
			t.Fatalf("expected supported, got %q", reason)
		}
	})

	// systemd used to be required. iface_Up now drives awg-quick directly when
	// there is no unit, so its absence must no longer refuse the backend.
	t.Run("no systemd is fine now", func(t *testing.T) {
		lookPath = present("awg-quick")
		if reason := KernelBackendUnsupported(); reason != "" {
			t.Fatalf("systemd must not be required any more, got %q", reason)
		}
	})

	t.Run("no CAP_NET_ADMIN => unsupported, and says how to grant it", func(t *testing.T) {
		lookPath = present("awg-quick")
		capEffective = func() string { return "0000000000000000" }
		defer func() { capEffective = func() string { return "0000003fffffffff" } }()
		reason := KernelBackendUnsupported()
		if reason == "" {
			t.Fatal("a panel that cannot configure interfaces must be refused")
		}
		if !strings.Contains(reason, "NET_ADMIN") {
			t.Errorf("reason %q should name the capability", reason)
		}
	})

	// A host with the module loaded, the tools installed and systemd running is
	// supported whether or not it is a container — the check must never key on
	// the runtime.
	t.Run("no amneziawg-tools => unsupported, and says which tool", func(t *testing.T) {
		lookPath = present()
		reason := KernelBackendUnsupported()
		if reason == "" {
			t.Fatal("missing awg-quick must be reported")
		}
		if want := "awg-quick"; !strings.Contains(reason, want) {
			t.Errorf("reason %q does not name %q", reason, want)
		}
	})

	t.Run("unknown capabilities do not block", func(t *testing.T) {
		lookPath = present("awg-quick")
		capEffective = func() string { return "" }
		defer func() { capEffective = func() string { return "0000003fffffffff" } }()
		if reason := KernelBackendUnsupported(); reason != "" {
			t.Fatalf("an unreadable /proc must not invent a refusal, got %q", reason)
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
