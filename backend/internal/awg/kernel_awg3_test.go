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

func TestDetectedKernelModuleVersion(t *testing.T) {
	origSysfs, origMod := sysfsModuleVersion, kernelModuleVersion
	t.Cleanup(func() { sysfsModuleVersion, kernelModuleVersion = origSysfs, origMod })

	notLoaded := func() (string, error) { return "", fs.ErrNotExist }

	t.Run("loaded => sysfs version, trimmed, modinfo not consulted", func(t *testing.T) {
		sysfsModuleVersion = func() (string, error) { return "3.0.20260731-04\n", nil }
		kernelModuleVersion = func() (string, error) {
			t.Error("modinfo must not be consulted when the module is loaded")
			return "", nil
		}
		v, ok := DetectedKernelModuleVersion()
		if !ok || v != "3.0.20260731-04" {
			t.Fatalf("got (%q, %v), want (\"3.0.20260731-04\", true)", v, ok)
		}
	})

	t.Run("not loaded, on disk => modinfo version, trimmed", func(t *testing.T) {
		sysfsModuleVersion = notLoaded
		kernelModuleVersion = func() (string, error) { return "3.1.20260812\n", nil }
		v, ok := DetectedKernelModuleVersion()
		if !ok || v != "3.1.20260812" {
			t.Fatalf("got (%q, %v), want (\"3.1.20260812\", true)", v, ok)
		}
	})

	t.Run("not loaded, not on disk => not ok", func(t *testing.T) {
		sysfsModuleVersion = notLoaded
		kernelModuleVersion = func() (string, error) { return "", errors.New("modinfo: ERROR: Module amneziawg not found") }
		if _, ok := DetectedKernelModuleVersion(); ok {
			t.Fatal("expected not ok when neither sysfs nor modinfo has a version")
		}
	})

	t.Run("sysfs fails for another reason => fail closed, no modinfo fallback", func(t *testing.T) {
		sysfsModuleVersion = func() (string, error) { return "", fs.ErrPermission }
		kernelModuleVersion = func() (string, error) {
			t.Error("modinfo must not be a fallback for an unreadable sysfs")
			return "3.1.20260812\n", nil
		}
		if _, ok := DetectedKernelModuleVersion(); ok {
			t.Fatal("expected not ok on a non-ENOENT sysfs error")
		}
	})
}

func TestAwg3AtLeast(t *testing.T) {
	cases := []struct {
		name         string
		in           string
		major, minor int
		want         bool
	}{
		{"3.1 clears the 3.1 bar", "3.1.20260812\n", 3, 1, true},
		{"3.0 does not clear the 3.1 bar", "3.0.20260805\n", 3, 1, false},
		{"3.0 clears the 3.0 bar", "3.0.20260731-04\n", 3, 0, true},
		{"tools 3.1 line", "amneziawg-tools v3.1.20260812 - https://amnezia.org\n", 3, 1, true},
		{"tools 3.0 line", "amneziawg-tools v3.0.20260730 - https://amnezia.org\n", 3, 1, false},
		{"newer major clears it", "4.0.20270101\n", 3, 1, true},
		{"newer minor clears it", "3.2.20270101\n", 3, 1, true},
		{"v1 era", "1.0.20260725\n", 3, 1, false},
		{"garbage", "not a version at all\n", 3, 1, false},
		{"empty", "", 3, 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := awg3AtLeast(c.in, c.major, c.minor); got != c.want {
				t.Fatalf("awg3AtLeast(%q, %d, %d) = %v, want %v", c.in, c.major, c.minor, got, c.want)
			}
		})
	}
}

func TestKernelSupportsAWG31(t *testing.T) {
	origSysfs, origMod, origTools := sysfsModuleVersion, kernelModuleVersion, kernelToolsVersion
	t.Cleanup(func() {
		sysfsModuleVersion, kernelModuleVersion, kernelToolsVersion = origSysfs, origMod, origTools
	})

	notLoaded := func() (string, error) { return "", fs.ErrNotExist }
	toolsV31 := func() (string, error) { return "amneziawg-tools v3.1.20260812 - https://amnezia.org\n", nil }
	toolsV30 := func() (string, error) { return "amneziawg-tools v3.0.20260730 - https://amnezia.org\n", nil }

	// The whole point of the separate gate: a 3.0 pairing clears KernelSupportsAWG3
	// and must NOT clear this one. A 3.0 module ignores the two 3.1 device flags
	// without an error, so the tunnel would look configured and run as before.
	t.Run("loaded 3.0 with 3.0 tools => not 3.1 (but is awg3)", func(t *testing.T) {
		sysfsModuleVersion = func() (string, error) { return "3.0.20260805\n", nil }
		kernelToolsVersion = toolsV30
		if !KernelSupportsAWG3() {
			t.Fatal("sanity: a 3.0 pairing must still clear the 3.0 gate")
		}
		if KernelSupportsAWG31() {
			t.Fatal("a 3.0 pairing must not clear the 3.1 gate")
		}
	})

	t.Run("loaded 3.1 with 3.1 tools => supported", func(t *testing.T) {
		sysfsModuleVersion = func() (string, error) { return "3.1.20260812\n", nil }
		kernelModuleVersion = func() (string, error) {
			t.Error("modinfo must not be consulted when the module is loaded")
			return "3.0.20260805\n", nil
		}
		kernelToolsVersion = toolsV31
		if !KernelSupportsAWG31() {
			t.Fatal("expected supported when the loaded module and tools both report 3.1")
		}
	})

	// Both artefacts are versioned independently, so each has to clear the bar
	// on its own — the same rule the 3.0 gate enforces.
	t.Run("module 3.1, tools 3.0 => unsupported", func(t *testing.T) {
		sysfsModuleVersion = func() (string, error) { return "3.1.20260812\n", nil }
		kernelToolsVersion = toolsV30
		if KernelSupportsAWG31() {
			t.Fatal("3.1 module with 3.0 tools must not be trusted")
		}
	})

	t.Run("module 3.0, tools 3.1 => unsupported", func(t *testing.T) {
		sysfsModuleVersion = func() (string, error) { return "3.0.20260805\n", nil }
		kernelToolsVersion = toolsV31
		if KernelSupportsAWG31() {
			t.Fatal("3.0 module with 3.1 tools must not be trusted")
		}
	})

	t.Run("not loaded, disk 3.1, tools 3.1 => supported", func(t *testing.T) {
		sysfsModuleVersion = notLoaded
		kernelModuleVersion = func() (string, error) { return "3.1.20260812\n", nil }
		kernelToolsVersion = toolsV31
		if !KernelSupportsAWG31() {
			t.Fatal("an unloaded 3.1 module on disk is what modprobe will load")
		}
	})

	// Fail-closed: an unreadable sysfs that is not "not exists" may well mean a
	// loaded module we cannot inspect, and guessing from disk is the same trap
	// the 3.0 gate refuses.
	t.Run("sysfs fails for another reason => unsupported, no disk fallback", func(t *testing.T) {
		sysfsModuleVersion = func() (string, error) { return "", errors.New("permission denied") }
		kernelModuleVersion = func() (string, error) {
			t.Error("modinfo must not be consulted when sysfs failed for a non-notexist reason")
			return "3.1.20260812\n", nil
		}
		kernelToolsVersion = toolsV31
		if KernelSupportsAWG31() {
			t.Fatal("expected fail-closed")
		}
	})
}
