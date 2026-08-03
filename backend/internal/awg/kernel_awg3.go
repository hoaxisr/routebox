package awg

import (
	"fmt"
	"os/exec"
	"strings"
)

// kernelModuleVersion and kernelToolsVersion are exec seams for
// KernelSupportsAWG3 (tests substitute them). They report the loaded
// amneziawg kernel module's version (via modinfo, which reads the module's
// declared MODULE_VERSION — no need for the module to be reloaded to see a
// change) and the awg-quick/tools binary's version, respectively.
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

// KernelSupportsAWG3 reports whether the loaded amneziawg kernel module AND
// the awg-quick/tools binary both report a major version >= 3 — the upstream
// AWG3 bump (amnezia-vpn/amneziawg-linux-kernel-module and amneziawg-tools
// v3.0.20260730) shipped kernel and userspace support as two independently
// versioned artefacts, so a module upgraded without the matching tools (or
// vice versa) must not be trusted to round-trip AWG3 fields — both have to
// independently clear the bar. FAIL-CLOSED: any exec/parse error is "no",
// matching KernelBackendUnsupported's stance of refusing rather than guessing.
var KernelSupportsAWG3 = func() bool {
	modOut, err := kernelModuleVersion()
	if err != nil || !awg3MajorAtLeast3(modOut) {
		return false
	}
	toolsOut, err := kernelToolsVersion()
	if err != nil || !awg3MajorAtLeast3(toolsOut) {
		return false
	}
	return true
}
