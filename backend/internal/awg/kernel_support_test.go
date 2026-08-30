package awg

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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
	t.Run("no amneziawg-tools and no installer => unsupported, and says which tool", func(t *testing.T) {
		lookPath = present()
		// Pinned to a distro RouteBox cannot install on, so the assertion is about
		// the message and not about whatever /etc/os-release this machine has.
		// Where the installer CAN run, missing tools are no longer a refusal —
		// that case is TestKernelBackendUnsupported_MissingToolsRouteBoxCanInstall.
		fixture := filepath.Join(t.TempDir(), "os-release")
		if err := os.WriteFile(fixture, []byte("ID=alpine\n"), 0644); err != nil {
			t.Fatal(err)
		}
		origOS := osReleaseFile
		osReleaseFile = fixture
		defer func() { osReleaseFile = origOS }()

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

// The production seam must probe /run/systemd/system, not PATH: a systemctl
// binary on disk does not mean systemd is PID 1 (containers/chroots ship the
// binary without the init), and keying bring-up on lookPath("systemctl") sent
// iface_Up down the `systemctl enable/restart` path where those calls can only
// fail. The directory is the canonical sd_booted(3) probe.
func TestSystemdRunningDefaultProbesRunDir(t *testing.T) {
	fi, err := os.Stat("/run/systemd/system")
	want := err == nil && fi.IsDir()
	if got := systemdRunning(); got != want {
		t.Fatalf("systemdRunning() = %v, want %v (per /run/systemd/system on this machine)", got, want)
	}
}

// The production seam must be the real thing.
func TestLookPathDefaultsToExec(t *testing.T) {
	if _, err := lookPath("sh"); err != nil {
		if _, execErr := exec.LookPath("sh"); execErr == nil {
			t.Fatal("lookPath is not wired to exec.LookPath")
		}
	}
}

// The panel offers a backend choice, and on a system that cannot run the kernel
// one every choice of it ends in the same 409. The status carries the reason so
// the picker can refuse it up front instead of letting the operator find out by
// clicking — reported from a live container with no CAP_NET_ADMIN.
func TestStatusReportsWhyTheKernelBackendIsUnavailable(t *testing.T) {
	orig := KernelBackendUnsupported
	t.Cleanup(func() { KernelBackendUnsupported = orig })

	KernelBackendUnsupported = func() string { return "no CAP_NET_ADMIN" }
	m := newTestManager(t, &fakeRunner{})
	m.SetBackend("singbox")
	if got := m.Status(context.Background()).KernelUnavailable; got != "no CAP_NET_ADMIN" {
		t.Fatalf("KernelUnavailable = %q, want the reason", got)
	}

	KernelBackendUnsupported = func() string { return "" }
	if got := m.Status(context.Background()).KernelUnavailable; got != "" {
		t.Fatalf("KernelUnavailable = %q on a system that can run it", got)
	}
}

// #93: RouteBox ships the installer for amneziawg-tools, and it runs only on the
// kernel backend — so refusing that backend for want of the tools made a clean
// host unable to ever reach the installer. Missing tools must refuse nothing
// where Ensure can run, and name both halves where it cannot.
func TestKernelBackendUnsupported_MissingToolsRouteBoxCanInstall(t *testing.T) {
	origLook, origCap, origUID, origOS := lookPath, capEffective, geteuid, osReleaseFile
	t.Cleanup(func() { lookPath, capEffective, geteuid, osReleaseFile = origLook, origCap, origUID, origOS })

	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	capEffective = func() string { return "0000003fffffffff" }
	geteuid = func() int { return 0 }

	osRelease := func(t *testing.T, body string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "os-release")
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("debian family, root, codename known => installable, so no refusal", func(t *testing.T) {
		osReleaseFile = osRelease(t, "ID=ubuntu\nID_LIKE=debian\nVERSION_CODENAME=noble\n")
		if reason := KernelBackendUnsupported(); reason != "" {
			t.Fatalf("the installer can run here; got refusal %q", reason)
		}
	})

	t.Run("not debian => refused, naming the distro and the missing tool", func(t *testing.T) {
		osReleaseFile = osRelease(t, "ID=alpine\n")
		reason := KernelBackendUnsupported()
		if !strings.Contains(reason, "awg-quick") || !strings.Contains(reason, "alpine") {
			t.Fatalf("reason %q must name both the missing tool and why it cannot be installed", reason)
		}
	})

	t.Run("no codename => refused (the PPA suite would be unknown)", func(t *testing.T) {
		osReleaseFile = osRelease(t, "ID=debian\n")
		if reason := KernelBackendUnsupported(); reason == "" {
			t.Fatal("Ensure fails without VERSION_CODENAME; the picker must not offer it")
		}
	})

	t.Run("not root => refused, apt cannot run", func(t *testing.T) {
		osReleaseFile = osRelease(t, "ID=ubuntu\nID_LIKE=debian\nVERSION_CODENAME=noble\n")
		geteuid = func() int { return 1000 }
		defer func() { geteuid = func() int { return 0 } }()
		reason := KernelBackendUnsupported()
		if !strings.Contains(reason, "root") {
			t.Fatalf("reason %q should say it cannot install without root", reason)
		}
	})

	// The tools being absent must never mask a missing capability: an installable
	// host with no CAP_NET_ADMIN still cannot create the interface.
	t.Run("installable but no CAP_NET_ADMIN => still refused", func(t *testing.T) {
		osReleaseFile = osRelease(t, "ID=ubuntu\nID_LIKE=debian\nVERSION_CODENAME=noble\n")
		capEffective = func() string { return "0000000000000000" }
		defer func() { capEffective = func() string { return "0000003fffffffff" } }()
		if reason := KernelBackendUnsupported(); !strings.Contains(reason, "NET_ADMIN") {
			t.Fatalf("reason %q should name the capability", reason)
		}
	})
}

// The DKMS build needs headers for the kernel that is actually running, and on
// Proxmox VE those live in pve-headers-*. Asking for linux-headers-<pve release>
// fails outright — the original reason the kernel backend was switched off.
func TestHeadersPackage(t *testing.T) {
	cases := map[string]string{
		"6.8.0-51-generic":  "linux-headers-6.8.0-51-generic",
		"6.8.12-4-pve":      "pve-headers-6.8.12-4-pve",
		"5.15.0-91-generic": "linux-headers-5.15.0-91-generic",
	}
	for release, want := range cases {
		if got := headersPackage(release); got != want {
			t.Errorf("headersPackage(%q) = %q, want %q", release, got, want)
		}
	}
}
