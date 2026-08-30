package awg

import (
	"fmt"
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

// osReleaseFile is where the installability probe reads the distro from (tests
// point it at a fixture).
var osReleaseFile = "/etc/os-release"

// geteuid is the root check's seam. apt needs root, and so does modprobe.
var geteuid = os.Geteuid

// KernelBackendUnsupported reports why the kernel backend cannot work in this
// installation, or "" when it can.
//
// It asks what RouteBox's kernel path actually needs, not where it is running.
// That path renders /etc/amnezia/amneziawg/<iface>.conf and hands it to
// awg-quick, which creates the interface — so the tools have to be reachable
// and this process has to be allowed to configure interfaces. systemd is no
// longer required: iface_Up drives awg-quick directly when there is no unit.
//
// "Reachable" is not the same as "already installed" (#93). RouteBox ships the
// installer — ModuleManager.Ensure adds the PPA and installs `amneziawg`, tools
// included — but it only runs on the kernel backend, which this function used
// to refuse for want of the very tools that install would have brought. A clean
// host could therefore never reach the installer at all. So a missing awg-quick
// refuses nothing where Ensure can run; where it cannot, the reason names both
// halves.
//
// Containers are deliberately not a criterion. An image that ships the tools
// and is granted CAP_NET_ADMIN passes this like any host; one that is not says
// exactly which of the two is missing, instead of failing later at Enable with
// a command error.
var KernelBackendUnsupported = func() string {
	if _, err := lookPath("awg-quick"); err != nil {
		if reason := kernelToolsInstallable(); reason != "" {
			return "awg-quick is not installed (amneziawg-tools), and " + reason
		}
	}
	if !hasNetAdmin() {
		return "this process has no CAP_NET_ADMIN, so it cannot create the interface (in Docker: add cap_add: [NET_ADMIN] and recreate the container)"
	}
	return ""
}

// inContainer reports whether this process runs inside a container. Best effort:
// the two runtime marker files, then PID 1's environment, which systemd-nspawn
// and LXC stamp with container=. Unreadable /proc/1/environ (not root, hidepid)
// yields no signal and therefore no refusal — the probe exists to name a certain
// failure, not to invent one.
var inContainer = func() bool {
	for _, marker := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	data, err := os.ReadFile("/proc/1/environ")
	if err != nil {
		return false
	}
	for _, kv := range strings.Split(string(data), "\x00") {
		if strings.HasPrefix(kv, "container=") {
			return true
		}
	}
	return false
}

// kernelToolsInstallable reports why RouteBox could not install the tools here,
// or "" when it could. It is not a full dry run of ModuleManager.Ensure — it
// cannot know whether a headers package exists for this exact kernel, and that
// failure is reported by name when it happens — but it does cover every case
// Ensure is certain to fail: the wrong distro, no release codename, no root, and
// a container.
func kernelToolsInstallable() string {
	// The container check comes first because it is the one that does damage: apt
	// would run, the PPA would be added, and only the DKMS build would fail —
	// against a kernel this container does not own. Note that a container which
	// SHIPS the tools never reaches this function at all: awg-quick is on PATH,
	// the module is the host's, and that arrangement works.
	if inContainer() {
		return "RouteBox cannot install them in a container: the kernel belongs to the host, so there is nothing for DKMS to build against. Install amneziawg on the host and load the module there (the image needs amneziawg-tools too), or use the singbox backend, which needs neither"
	}
	id, idLike, codename := readOSRelease(osReleaseFile)
	if !isDebianFamily(id, idLike) {
		return fmt.Sprintf("RouteBox cannot install them here (distro %q; the installer supports Debian/Ubuntu). Install amneziawg-tools yourself, or use the singbox backend, which needs neither", id)
	}
	if codename == "" {
		return "RouteBox cannot install them here (VERSION_CODENAME is absent from " + osReleaseFile + ", so the PPA suite is unknown)"
	}
	if geteuid() != 0 {
		return "RouteBox is not running as root, so it cannot install them"
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

// SetKernelSupportsAWG31 wires the AWG 3.1 capability gate (the two device
// flags). nil (unset) = unsupported.
func (m *Manager) SetKernelSupportsAWG31(fn func() bool) {
	m.mu.Lock()
	m.kernelSupports31Fn = fn
	m.mu.Unlock()
}
