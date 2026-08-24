package awg

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
)

// sysfsModuleVersion, kernelModuleVersion and kernelToolsVersion are the seams
// for KernelSupportsAWG3 (tests substitute them).
//
// The module version needs TWO sources because "the module's version" is
// ambiguous whenever a stale module stays loaded: /sys/module/amneziawg/version
// exists iff the module is LOADED and reports what the kernel is actually
// running, while modinfo reads the on-disk .ko — what modprobe WOULD load. A
// dkms upgrade without a reboot makes them diverge (loaded v1, disk v3), and
// trusting modinfo then had the gate say "capable", Enable render AWG3 keys
// (header_protection_key etc), and `awg-quick up` fail against the running v1
// module. KernelSupportsAWG3 arbitrates between the two.
var sysfsModuleVersion = func() (string, error) {
	out, err := os.ReadFile("/sys/module/amneziawg/version")
	return string(out), err
}

var kernelModuleVersion = func() (string, error) {
	out, err := exec.Command("modinfo", "-F", "version", "amneziawg").Output()
	return string(out), err
}

var kernelToolsVersion = func() (string, error) {
	out, err := exec.Command("awg", "--version").Output()
	return string(out), err
}

// awg3MajorAtLeast3 reports whether s contains a "[v]X.Y..." version token
// whose major component is >= 3. Matches both modinfo's bare "3.0.20260731-04"
// and awg --version's "amneziawg-tools v3.0.20260730 - https://amnezia.org".
// PURE.
func awg3MajorAtLeast3(s string) bool {
	for _, tok := range strings.Fields(s) {
		tok = strings.TrimPrefix(tok, "v")
		var maj int
		if _, err := fmt.Sscanf(tok, "%d.", &maj); err == nil {
			return maj >= 3
		}
	}
	return false
}

// KernelSupportsAWG3 reports whether the amneziawg kernel module AND the
// awg-quick/tools binary both report a major version >= 3 — the upstream
// AWG3 bump (amnezia-vpn/amneziawg-linux-kernel-module and amneziawg-tools
// v3.0.20260730) shipped kernel and userspace support as two independently
// versioned artefacts, so a module upgraded without the matching tools (or
// vice versa) must not be trusted to round-trip AWG3 fields — both have to
// independently clear the bar.
//
// Which module version counts depends on whether the module is LOADED:
//   - loaded (/sys/module/amneziawg/version exists): that file is the version
//     the kernel is actually running, and it is authoritative — the on-disk
//     .ko is irrelevant until a reload/reboot, so modinfo is not consulted;
//   - not loaded (sysfs read fails with not-exists): modinfo's on-disk version
//     is exactly what modprobe will load at `awg-quick up`, so it applies;
//   - sysfs fails for any OTHER reason: FAIL-CLOSED, no modinfo fallback — the
//     module may well be loaded with a version we cannot read, and guessing
//     from the on-disk .ko is the loaded-v1/disk-v3 bug all over again.
//
// FAIL-CLOSED throughout: any read/exec/parse error is "no", matching
// KernelBackendUnsupported's stance of refusing rather than guessing.
var KernelSupportsAWG3 = func() bool {
	return kernelAndToolsClear(awg3MajorAtLeast3)
}

// awg3AtLeast reports whether s carries a "[v]X.Y..." version token at or above
// the given major.minor. awg3MajorAtLeast3 above reads only the major component
// and so cannot tell 3.0 from 3.1 — which matters because a 3.0 module ignores
// the two AWG 3.1 device flags without an error, exactly the way a 2.x module
// ignores the 3.0 params. PURE.
func awg3AtLeast(s string, major, minor int) bool {
	for _, tok := range strings.Fields(s) {
		tok = strings.TrimPrefix(tok, "v")
		var maj, min int
		if _, err := fmt.Sscanf(tok, "%d.%d", &maj, &min); err == nil {
			return maj > major || (maj == major && min >= minor)
		}
	}
	return false
}

// KernelSupportsAWG31 is KernelSupportsAWG3 with the bar raised to 3.1, for the
// two device flags that version added (RandomTrailers, DisableCookies). Same
// sources, same loaded-vs-on-disk arbitration, same fail-closed stance.
var KernelSupportsAWG31 = func() bool {
	return kernelAndToolsClear(func(s string) bool { return awg3AtLeast(s, 3, 1) })
}

// kernelAndToolsClear holds the version arbitration both gates share: which
// module version counts (see the comment on KernelSupportsAWG3), that the tools
// binary has to clear the same bar independently, and that every read/exec/parse
// error is a "no". Kept in one place so the two gates cannot drift apart on the
// fail-closed rules — those are the subtle part, not the comparison.
func kernelAndToolsClear(clears func(string) bool) bool {
	modOut, ok := resolvedModuleVersion()
	if !ok || !clears(modOut) {
		return false
	}
	toolsOut, err := kernelToolsVersion()
	if err != nil || !clears(toolsOut) {
		return false
	}
	return true
}

// resolvedModuleVersion is the loaded-vs-on-disk arbitration from
// KernelSupportsAWG3's doc comment, factored out of kernelAndToolsClear so
// DetectedKernelModuleVersion (a plain "what version is installed" read for
// the UI, no capability bar involved) shares the exact same fail-closed rules
// instead of re-deriving them and risking drift.
func resolvedModuleVersion() (version string, ok bool) {
	modOut, err := sysfsModuleVersion()
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return "", false
		}
		if modOut, err = kernelModuleVersion(); err != nil {
			return "", false
		}
	}
	v := strings.TrimSpace(modOut)
	if v == "" {
		return "", false
	}
	return v, true
}

// DetectedKernelModuleVersion reports the amneziawg kernel module's version
// for display, using the same loaded-vs-on-disk arbitration as
// KernelSupportsAWG3/AWG31 (see that doc comment). ok is false when the
// module is neither loaded nor installed, or when its version cannot be
// read — the UI shows a "not installed" notice in that case, never a guess.
func DetectedKernelModuleVersion() (version string, ok bool) {
	return resolvedModuleVersion()
}
