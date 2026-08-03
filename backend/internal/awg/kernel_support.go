package awg

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// lookPath is the exec.LookPath seam (tests substitute it).
var lookPath = exec.LookPath

// systemdRunning reports whether systemd is the running init (tests substitute
// it — the withSystemd/withoutSystemd pins). The canonical probe is the
// existence of the /run/systemd/system directory, which systemd creates at
// boot (it is what sd_booted(3) stats); lookPath("systemctl") was wrong here
// because containers/chroots ship the binary without systemd being PID 1, and
// then `systemctl enable/restart` can only fail where the direct awg-quick
// path would have worked.
var systemdRunning = func() bool {
	fi, err := os.Stat("/run/systemd/system")
	return err == nil && fi.IsDir()
}

// capEffective reads the process's effective capability set. Overridable in
// tests; "" means unknown, which is treated as "do not block".
var capEffective = func() string {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "CapEff:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "CapEff:"))
		}
	}
	return ""
}

// capNetAdmin is CAP_NET_ADMIN (bit 12) as a mask.
const capNetAdmin = 1 << 12

// hasNetAdmin reports whether this process can configure network interfaces.
// Unknown (no /proc, unparsable) counts as yes: the check exists to give a
// precise reason up front, not to invent a new way to refuse.
func hasNetAdmin() bool {
	eff := capEffective()
	if eff == "" {
		return true
	}
	v, err := strconv.ParseUint(eff, 16, 64)
	if err != nil {
		return true
	}
	return v&capNetAdmin != 0
}

// KernelBackendUnsupported reports why the kernel backend cannot work in this
// installation, or "" when it can.
//
// It asks what RouteBox's kernel path actually needs, not where it is running.
// That path renders /etc/amnezia/amneziawg/<iface>.conf and hands it to
// awg-quick, which creates the interface — so the tools have to be present and
// this process has to be allowed to configure interfaces. systemd is no longer
// required: iface_Up drives awg-quick directly when there is no unit.
//
// Containers are deliberately not a criterion. An image that ships the tools
// and is granted CAP_NET_ADMIN passes this like any host; one that is not says
// exactly which of the two is missing, instead of failing later at Enable with
// a command error.
var KernelBackendUnsupported = func() string {
	if _, err := lookPath("awg-quick"); err != nil {
		return "awg-quick is not installed (amneziawg-tools)"
	}
	if !hasNetAdmin() {
		return "this process has no CAP_NET_ADMIN, so it cannot create the interface (in Docker: add cap_add: [NET_ADMIN] and recreate the container)"
	}
	return ""
}

// SetKernelSupportsAWG3 wires the kernel backend's awg3 capability gate. nil
// (unset) = unsupported.
func (m *Manager) SetKernelSupportsAWG3(fn func() bool) {
	m.mu.Lock()
	m.kernelSupports3Fn = fn
	m.mu.Unlock()
}
