package awg

import (
	"errors"
	"io/fs"
	"testing"
)

func TestAwg3MajorAtLeast3(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"bare modinfo version", "3.0.20260731-04\n", true},
		{"tools version line", "amneziawg-tools v3.0.20260730 - https://amnezia.org\n", true},
		{"old kmod-era v1 tag", "1.0.20260725\n", false},
		{"old tools v1 tag", "amneziawg-tools v1.0.20260618 - https://amnezia.org\n", false},
		{"garbage", "not a version at all\n", false},
		{"empty", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := awg3MajorAtLeast3(c.in); got != c.want {
				t.Errorf("awg3MajorAtLeast3(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestKernelSupportsAWG3(t *testing.T) {
	origSysfs, origMod, origTools := sysfsModuleVersion, kernelModuleVersion, kernelToolsVersion
	t.Cleanup(func() {
		sysfsModuleVersion, kernelModuleVersion, kernelToolsVersion = origSysfs, origMod, origTools
	})

	notLoaded := func() (string, error) { return "", fs.ErrNotExist }
	toolsV3 := func() (string, error) { return "amneziawg-tools v3.0.20260730 - https://amnezia.org\n", nil }
	toolsV1 := func() (string, error) { return "amneziawg-tools v1.0.20260618 - https://amnezia.org\n", nil }

	// The dkms-upgrade-without-reboot trap: the kernel still runs the v1 module
	// it loaded at boot while the on-disk .ko already says v3. Trusting modinfo
	// here had Enable render AWG3 keys that `awg-quick up` fed to a v1 module.
	t.Run("loaded v1, disk v3 => unsupported (kernel runs v1)", func(t *testing.T) {
		sysfsModuleVersion = func() (string, error) { return "1.0.20260725\n", nil }
		kernelModuleVersion = func() (string, error) { return "3.0.20260731-04\n", nil }
		kernelToolsVersion = toolsV3
		if KernelSupportsAWG3() {
			t.Fatal("a loaded v1 module must not be masked by a v3 module on disk")
		}
	})

	// When the module is loaded, sysfs IS the truth; modinfo must not even be
	// consulted (its on-disk answer is irrelevant to the running kernel).
	t.Run("loaded v3 => supported, disk never consulted", func(t *testing.T) {
		sysfsModuleVersion = func() (string, error) { return "3.0.20260731-04\n", nil }
		kernelModuleVersion = func() (string, error) {
			t.Error("modinfo must not be consulted when the module is loaded")
			return "1.0.20260725\n", nil
		}
		kernelToolsVersion = toolsV3
		if !KernelSupportsAWG3() {
			t.Fatal("expected supported when the loaded module and tools both report v3+")
		}
	})

	// Not loaded => the on-disk module is exactly what modprobe will load at
	// `awg-quick up`, so modinfo's answer is the right one.
	t.Run("not loaded, disk v3, tools v3 => supported", func(t *testing.T) {
		sysfsModuleVersion = notLoaded
		kernelModuleVersion = func() (string, error) { return "3.0.20260731-04\n", nil }
		kernelToolsVersion = toolsV3
		if !KernelSupportsAWG3() {
			t.Fatal("expected supported: an unloaded v3 module on disk is what will be loaded")
		}
	})

	t.Run("not loaded, disk v1, tools v3 => unsupported (mismatched pairing)", func(t *testing.T) {
		sysfsModuleVersion = notLoaded
		kernelModuleVersion = func() (string, error) { return "1.0.20260725\n", nil }
		kernelToolsVersion = toolsV3
		if KernelSupportsAWG3() {
			t.Fatal("a stale module must not be masked by fresh tools")
		}
	})

	t.Run("loaded v3, tools v1 => unsupported (mismatched pairing)", func(t *testing.T) {
		sysfsModuleVersion = func() (string, error) { return "3.0.20260731-04\n", nil }
		kernelModuleVersion = func() (string, error) { return "3.0.20260731-04\n", nil }
		kernelToolsVersion = toolsV1
		if KernelSupportsAWG3() {
			t.Fatal("stale tools must not be masked by a fresh module")
		}
	})

	// A sysfs failure that is NOT "not exists" means the module may well be
	// loaded with a version we cannot read — FAIL-CLOSED, no modinfo fallback
	// (falling back to a v3 .ko on disk would be the loaded-v1 bug again).
	t.Run("sysfs unreadable (not ENOENT) => fail closed, no modinfo fallback", func(t *testing.T) {
		sysfsModuleVersion = func() (string, error) { return "", fs.ErrPermission }
		kernelModuleVersion = func() (string, error) {
			t.Error("modinfo must not be a fallback for an unreadable sysfs")
			return "3.0.20260731-04\n", nil
		}
		kernelToolsVersion = toolsV3
		if KernelSupportsAWG3() {
			t.Fatal("an unreadable /sys/module version must fail closed")
		}
	})

	t.Run("not loaded and modinfo errors (no module on disk) => unsupported", func(t *testing.T) {
		sysfsModuleVersion = notLoaded
		kernelModuleVersion = func() (string, error) { return "", errors.New("modinfo: ERROR: Module amneziawg not found") }
		kernelToolsVersion = toolsV3
		if KernelSupportsAWG3() {
			t.Fatal("expected unsupported when modinfo fails")
		}
	})

	t.Run("awg --version errors (tools missing) => unsupported", func(t *testing.T) {
		sysfsModuleVersion = func() (string, error) { return "3.0.20260731-04\n", nil }
		kernelModuleVersion = func() (string, error) { return "3.0.20260731-04\n", nil }
		kernelToolsVersion = func() (string, error) { return "", errors.New("exec: \"awg\": executable file not found in $PATH") }
		if KernelSupportsAWG3() {
			t.Fatal("expected unsupported when awg --version fails")
		}
	})
}

// The production sysfs seam must map "module not loaded" onto fs.ErrNotExist —
// that errno class is what routes KernelSupportsAWG3 to modinfo. On a machine
// with the module loaded it simply succeeds; any other error class here would
// silently turn every gate call on module-less machines into a fail-closed "no".
func TestSysfsModuleVersionDefault(t *testing.T) {
	if _, err := sysfsModuleVersion(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("sysfsModuleVersion must fail with fs.ErrNotExist when the module is not loaded, got %v", err)
	}
}
