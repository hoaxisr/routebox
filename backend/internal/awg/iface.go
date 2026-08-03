package awg

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// iface_Up (re)applies the CURRENT conf to the running interface. `enable` (no
// --now) sets boot persistence idempotently; `restart` then starts-or-reloads the
// unit so a changed conf (port / obfuscation / subnet) actually takes effect.
// `enable --now` is a no-op on an ALREADY-ACTIVE unit, which silently left every
// Apply/re-enable change unapplied to the live interface (conf on disk updated, but
// awg-quick never re-read it).
func (m *Manager) iface_Up(ctx context.Context) error {
	if systemdRunning() {
		if _, _, err := m.run.Run(ctx, "systemctl", "enable", "awg-quick@"+m.iface); err != nil {
			return err
		}
		_, _, err := m.run.Run(ctx, "systemctl", "restart", "awg-quick@"+m.iface)
		return err
	}

	// No systemd: drive awg-quick directly. This is the container case, and any
	// host without systemd — iface_Down has always had the same fallback, so
	// until now such a system could tear an interface down but never bring one
	// up. There is no unit to hold boot persistence here, so RestoreKernelIface
	// re-applies the conf at startup instead.
	//
	// `awg-quick up` refuses an interface that already exists, while this
	// function must be re-appliable (a changed port or obfuscation has to reach
	// the live link), so an existing one is taken down first.
	if m.kernelIfacePresent(ctx) {
		if _, _, err := m.run.Run(ctx, "awg-quick", "down", m.iface); err != nil {
			return err
		}
	}
	_, _, err := m.run.Run(ctx, "awg-quick", "up", m.iface)
	return err
}

// RestoreKernelIface brings the kernel interface back up at startup when there
// is no systemd unit to have done it. Without this a container that comes back
// shows an AmneziaWG server the settings call enabled, with no interface behind
// it — the same gap RouteBox closes for amnezia-box itself.
//
// Best-effort by construction: it reports what happened for the log and never
// blocks startup. A no-op when systemd is present (the unit owns persistence),
// when the operator disabled the server, or when the link is already there.
func (m *Manager) RestoreKernelIface(ctx context.Context, desiredEnabled bool) error {
	if !desiredEnabled {
		return nil
	}
	if systemdRunning() {
		return nil
	}
	if m.kernelIfacePresent(ctx) {
		return nil
	}
	if _, err := os.Stat(m.confPath); err != nil {
		return nil // never configured; nothing to restore
	}
	if _, _, err := m.run.Run(ctx, "awg-quick", "up", m.iface); err != nil {
		return fmt.Errorf("restoring the AmneziaWG interface: %w", err)
	}
	m.mu.Lock()
	m.enabled = true
	m.phase = PhaseReady
	m.mu.Unlock()
	return nil
}

// iface_Down stops the interface AND clears its boot persistence, covering every
// launch variant — not just the systemd unit:
//   - `systemctl disable --now` handles the unit-managed case and removes the
//     enable symlink (an enabled-but-inactive unit would resurrect the kernel
//     iface at the next boot);
//   - a manually-launched iface (`awg-quick up` outside the unit) is untouched by
//     the unit stop, so if the link is still present we run `awg-quick down`
//     directly (its PostDown reverts NAT);
//   - with awg-tools broken/missing (or no systemd at all), the last resort is
//     `ip link delete` plus an explicit RBOX-AWG-* chain teardown, because
//     PostDown never ran on that path.
//
// Success is judged by the OUTCOME (link gone), not by any one command's exit
// code — `systemctl` legitimately fails on non-systemd boxes.
func (m *Manager) iface_Down(ctx context.Context) error {
	_, _, sysErr := m.run.Run(ctx, "systemctl", "disable", "--now", "awg-quick@"+m.iface)
	if !m.kernelIfacePresent(ctx) {
		return nil
	}
	if _, _, err := m.run.Run(ctx, "awg-quick", "down", m.iface); err == nil && !m.kernelIfacePresent(ctx) {
		return nil
	}
	_, _, _ = m.run.Run(ctx, "ip", "link", "delete", "dev", m.iface)
	m.cleanupNATChains(ctx)
	if m.kernelIfacePresent(ctx) {
		if sysErr != nil {
			return fmt.Errorf("interface %s is still up after teardown (systemctl: %v)", m.iface, sysErr)
		}
		return fmt.Errorf("interface %s is still up after teardown", m.iface)
	}
	return nil
}

// kernelIfacePresent reports whether the kernel netdev exists, independent of the
// awg tool (which may be broken/uninstalled while the module keeps the iface
// alive). `ip link show dev <iface>` exits non-zero when the device is absent and
// names it in stdout when present; both are required so a permissive fake runner
// ("" output, nil error) reads as absent.
func (m *Manager) kernelIfacePresent(ctx context.Context) bool {
	out, _, err := m.run.Run(ctx, "ip", "link", "show", "dev", m.iface)
	return err == nil && strings.Contains(out, m.iface)
}

// kernelUnitEnabled reports whether awg-quick@<iface> is systemd-enabled (boot
// persistence). Degrades to false where systemctl is absent or the unit unknown.
func (m *Manager) kernelUnitEnabled(ctx context.Context) bool {
	out, _, err := m.run.Run(ctx, "systemctl", "is-enabled", "awg-quick@"+m.iface)
	return err == nil && strings.TrimSpace(out) == "enabled"
}

// cleanupNATChains reverts the RBOX-AWG-* chains exactly like the rendered
// PostDown (unlink/flush/delete per chain), for teardown paths where awg-quick
// never ran PostDown. Every step tolerates absence; errors are ignored.
func (m *Manager) cleanupNATChains(ctx context.Context) {
	for _, ch := range []struct {
		table   []string
		builtin string
		chain   string
	}{
		{[]string{"-t", "nat"}, "POSTROUTING", "RBOX-AWG-NAT"},
		{nil, "FORWARD", "RBOX-AWG-FWD"},
		{nil, "INPUT", "RBOX-AWG-IN"},
	} {
		unlink := append(append([]string{}, ch.table...), "-D", ch.builtin, "-j", ch.chain)
		flush := append(append([]string{}, ch.table...), "-F", ch.chain)
		del := append(append([]string{}, ch.table...), "-X", ch.chain)
		_, _, _ = m.run.Run(ctx, "iptables", unlink...)
		_, _, _ = m.run.Run(ctx, "iptables", flush...)
		_, _, _ = m.run.Run(ctx, "iptables", del...)
	}
}

// iface_SetPeer adds/updates a peer live; PSK is passed by FILE (awg convention).
// pub is a validated 32-byte std-base64 key and allowedIP is a netip-canonical
// "<ip>/32" — neither can begin with '-', so there is no flag-injection via argv
// (and the Runner never uses a shell).
func (m *Manager) iface_SetPeer(ctx context.Context, pub, pskFile, allowedIP string) error {
	_, _, err := m.run.Run(ctx, "awg", "set", m.iface, "peer", pub,
		"preshared-key", pskFile, "allowed-ips", allowedIP)
	return err
}

// iface_RemovePeer removes a peer live.
func (m *Manager) iface_RemovePeer(ctx context.Context, pub string) error {
	_, _, err := m.run.Run(ctx, "awg", "set", m.iface, "peer", pub, "remove")
	return err
}

// iface_Handshakes returns pubkey -> last-handshake unix-seconds from
// `awg show <iface> latest-handshakes` (tab-separated "<pubkey>\t<unix>"; 0 = never).
// Degrades to an empty map on any error so callers just report everyone offline.
func (m *Manager) iface_Handshakes(ctx context.Context) map[string]int64 {
	out := map[string]int64{}
	res, _, err := m.run.Run(ctx, "awg", "show", m.iface, "latest-handshakes")
	if err != nil {
		return out
	}
	for _, line := range strings.Split(res, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		ts, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil {
			continue
		}
		out[f[0]] = ts
	}
	return out
}

// peerXfer is one peer's cumulative byte counters (since the iface came up).
type peerXfer struct{ rx, tx int64 }

// iface_Transfer returns pubkey -> {rx,tx} bytes from `awg show <iface> transfer`
// (tab-separated "<pubkey>\t<rx>\t<tx>"). Cumulative since iface up; resets on
// awg-quick restart. Degrades to an empty map on any error.
func (m *Manager) iface_Transfer(ctx context.Context) map[string]peerXfer {
	out := map[string]peerXfer{}
	res, _, err := m.run.Run(ctx, "awg", "show", m.iface, "transfer")
	if err != nil {
		return out
	}
	for _, line := range strings.Split(res, "\n") {
		f := strings.Fields(line)
		if len(f) != 3 {
			continue
		}
		rx, e1 := strconv.ParseInt(f[1], 10, 64)
		tx, e2 := strconv.ParseInt(f[2], 10, 64)
		if e1 != nil || e2 != nil {
			continue
		}
		out[f[0]] = peerXfer{rx: rx, tx: tx}
	}
	return out
}

// iface_ShowPeers returns the live peer pubkeys (for reconcile).
func (m *Manager) iface_ShowPeers(ctx context.Context) ([]string, error) {
	out, _, err := m.run.Run(ctx, "awg", "show", m.iface)
	if err != nil {
		return nil, err
	}
	return parseShowPeers(out), nil
}
