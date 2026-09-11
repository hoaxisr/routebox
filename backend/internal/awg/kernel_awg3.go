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
	return probeKernel().clears(awg3MajorAtLeast3)
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
	return probeKernel().clears(awg31Bar)
}

// awg31Bar is the 3.1 comparison as a named function so the gate and the
// combined probe below cannot pass subtly different closures.
func awg31Bar(s string) bool { return awg3AtLeast(s, 3, 1) }

// kernelProbe is ONE resolution of the two version sources every module
// question shares. Resolving them together is what lets a caller that needs
// more than one answer — Status needs three (the version and both bars) — pay a
// single sysfs read and at most one modinfo fork instead of one per answer.
// That fork is the whole cost on a host without the module, and the AWG page
// polls Status every 5s.
type kernelProbe struct {
	module  string // per the loaded-vs-on-disk rules; "" when unresolvable
	tools   string
	modOK   bool
	loaded  bool // module version came from sysfs, i.e. the kernel is running it
	toolsOK bool
}

// probeKernel resolves both versions FAIL-CLOSED: an unresolvable source leaves
// its ok false and clears() then answers "no" for every bar. The tools binary
// is consulted only once the module resolved, since no bar can be cleared
// without the module anyway — one less fork on a host that has neither.
//
// It does give up one short-circuit the two gates had separately: they returned
// before running `awg --version` when the module missed the bar, and this runs
// it either way. That is one extra fork per standalone gate call on a host with
// a pre-3.0 module — the Enable path, called once — against two fewer module
// resolutions per Status, which the open AWG page polls every 5s.
func probeKernel() kernelProbe {
	mod, loaded, ok := resolvedModuleVersion()
	if !ok {
		return kernelProbe{}
	}
	tools, err := kernelToolsVersion()
	if err != nil {
		return kernelProbe{module: mod, modOK: true, loaded: loaded}
	}
	return kernelProbe{module: mod, tools: tools, modOK: true, loaded: loaded, toolsOK: true}
}

// clears holds the arbitration both gates share: the module version that counts
// (see the comment on KernelSupportsAWG3), that the tools binary has to clear
// the same bar independently, and that every read/exec/parse error is a "no".
// Kept in one place so the gates cannot drift apart on the fail-closed rules —
// those are the subtle part, not the comparison.
func (p kernelProbe) clears(bar func(string) bool) bool {
	return p.modOK && p.toolsOK && bar(p.module) && bar(p.tools)
}

// resolvedModuleVersion is the loaded-vs-on-disk arbitration from
// KernelSupportsAWG3's doc comment. Separate from the bar comparison so the
// UI's plain "what version is installed" read shares the exact same
// fail-closed rules instead of re-deriving them and risking drift.
//
// loaded says WHICH source answered: true for sysfs (the kernel is running this
// module), false for modinfo (a .ko on disk that modprobe would try). The gates
// do not care — either way it is the version `awg-quick up` will face — but a
// caller reporting readiness does: a package that is installed and cannot load
// (unsigned under Secure Boot, built for another kernel) answers modinfo just
// as happily as one that works.
func resolvedModuleVersion() (version string, loaded, ok bool) {
	modOut, err := sysfsModuleVersion()
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return "", false, false
		}
		if modOut, err = kernelModuleVersion(); err != nil {
			return "", false, false
		}
		if v := strings.TrimSpace(modOut); v != "" {
			return v, false, true
		}
		return "", false, false
	}
	v := strings.TrimSpace(modOut)
	if v == "" {
		return "", false, false
	}
	return v, true, true
}

// KernelModuleInfo answers every module question Status asks, from one probe.
type KernelModuleInfo struct {
	// Version is the installed module's version for display, "" when it could
	// not be resolved.
	Version string
	// Detected reports that a version was actually read. False covers TWO
	// different worlds: the module is genuinely absent, and the module is
	// loaded but its version is unreadable (the fail-closed non-ENOENT sysfs
	// case — a container with /sys restricted, say). So !Detected is NOT proof
	// of "not installed", and the UI must not render it as one; Status settles
	// presence separately, from the module state and a live interface.
	Detected bool
	// Loaded narrows Detected to the one source that proves the module WORKS:
	// /sys/module/amneziawg/version, which exists only while the kernel runs it.
	// Detected alone would also be true for a .ko sitting on disk that cannot
	// load at all (unsigned under Secure Boot, built for another kernel), and
	// calling that "ready" hides both the warning and its remedy until Enable
	// fails. Only Loaded may upgrade a module state.
	Loaded bool
	// SupportsAWG3/SupportsAWG31 are the two capability bars, identical to
	// KernelSupportsAWG3/KernelSupportsAWG31 by construction (same probe, same
	// clears) — this type just answers all three at once.
	SupportsAWG3  bool
	SupportsAWG31 bool
}

// DetectKernelModule reports the installed amneziawg module's version and both
// capability bars from a single probe. Wired into the Manager by
// SetKernelModuleInfo; the per-gate KernelSupportsAWG3/AWG31 remain for the
// Enable path, which asks one bar at a time.
var DetectKernelModule = func() KernelModuleInfo {
	p := probeKernel()
	return KernelModuleInfo{
		Version:       p.module,
		Detected:      p.modOK,
		Loaded:        p.loaded,
		SupportsAWG3:  p.clears(awg3MajorAtLeast3),
		SupportsAWG31: p.clears(awg31Bar),
	}
}
