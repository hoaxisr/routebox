package awg

import "os/exec"

// lookPath is the exec.LookPath seam (tests substitute it).
var lookPath = exec.LookPath

// KernelBackendUnsupported reports why the kernel backend cannot work in this
// installation, or "" when it can.
//
// The check is about what RouteBox's kernel path actually calls, not about
// where it runs. That path writes /etc/amnezia/amneziawg/<iface>.conf and
// brings the interface up with `systemctl enable/restart awg-quick@<iface>`
// (iface_Up), so it needs the amneziawg-tools AND systemd to run the unit. A
// loaded kernel module on its own is not enough.
//
// Containers are deliberately not a criterion. A host with the amneziawg
// module loaded can absolutely serve a containerised kernel interface — given
// NET_ADMIN and an image that ships the tools, it passes this check like any
// other machine. RouteBox's own image is not such an image, and says so here
// for the same reason it would on a VPS with no tools installed: because the
// tools are missing, not because it is a container.
var KernelBackendUnsupported = func() string {
	if _, err := lookPath("awg-quick"); err != nil {
		return "awg-quick is not installed (amneziawg-tools)"
	}
	if _, err := lookPath("systemctl"); err != nil {
		return "systemd is not available, and the kernel interface is brought up through the awg-quick@ systemd unit"
	}
	return ""
}
