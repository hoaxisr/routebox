package awg

import (
	"errors"
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
	origMod, origTools := kernelModuleVersion, kernelToolsVersion
	t.Cleanup(func() { kernelModuleVersion, kernelToolsVersion = origMod, origTools })

	t.Run("both v3+ => supported", func(t *testing.T) {
		kernelModuleVersion = func() (string, error) { return "3.0.20260731-04\n", nil }
		kernelToolsVersion = func() (string, error) { return "amneziawg-tools v3.0.20260730 - https://amnezia.org\n", nil }
		if !KernelSupportsAWG3() {
			t.Fatal("expected supported when module and tools both report v3+")
		}
	})

	t.Run("module v1, tools v3 => unsupported (mismatched pairing)", func(t *testing.T) {
		kernelModuleVersion = func() (string, error) { return "1.0.20260725\n", nil }
		kernelToolsVersion = func() (string, error) { return "amneziawg-tools v3.0.20260730 - https://amnezia.org\n", nil }
		if KernelSupportsAWG3() {
			t.Fatal("a stale module must not be masked by fresh tools")
		}
	})

	t.Run("module v3, tools v1 => unsupported (mismatched pairing)", func(t *testing.T) {
		kernelModuleVersion = func() (string, error) { return "3.0.20260731-04\n", nil }
		kernelToolsVersion = func() (string, error) { return "amneziawg-tools v1.0.20260618 - https://amnezia.org\n", nil }
		if KernelSupportsAWG3() {
			t.Fatal("stale tools must not be masked by a fresh module")
		}
	})

	t.Run("modinfo errors (module not loaded) => unsupported", func(t *testing.T) {
		kernelModuleVersion = func() (string, error) { return "", errors.New("modinfo: ERROR: Module amneziawg not found") }
		kernelToolsVersion = func() (string, error) { return "amneziawg-tools v3.0.20260730 - https://amnezia.org\n", nil }
		if KernelSupportsAWG3() {
			t.Fatal("expected unsupported when modinfo fails")
		}
	})

	t.Run("awg --version errors (tools missing) => unsupported", func(t *testing.T) {
		kernelModuleVersion = func() (string, error) { return "3.0.20260731-04\n", nil }
		kernelToolsVersion = func() (string, error) { return "", errors.New("exec: \"awg\": executable file not found in $PATH") }
		if KernelSupportsAWG3() {
			t.Fatal("expected unsupported when awg --version fails")
		}
	})
}
