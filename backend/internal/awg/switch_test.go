package awg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// linkPresentUntil scripts `ip link show dev <iface>` statefully: the link reads
// as present until a call whose joined argv contains downMarker has happened,
// then reads as absent. Mirrors a real teardown (down command removes the link).
func linkPresentUntil(f *fakeRunner, iface, downMarker string) {
	f.match = func(name string, args []string) (string, bool) {
		joined := name + " " + strings.Join(args, " ")
		if joined != "ip link show dev "+iface {
			return "", false
		}
		// Present unless the down command already ran (skip the current call,
		// which IS the ip-link probe just appended to f.calls).
		for _, c := range f.calls[:len(f.calls)-1] {
			if strings.Contains(strings.Join(c, " "), downMarker) {
				return "", false // absent -> fall through to errs/outputs (empty out)
			}
		}
		return "1: " + iface + ": <POINTOPOINT,UP>", true
	}
}

// Unit-managed launch: `systemctl disable --now` takes the link down, so no
// direct awg-quick/ip-link teardown must run (and boot persistence is cleared).
func TestIfaceDownUnitManaged(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	linkPresentUntil(f, m.iface, "systemctl disable --now awg-quick@"+m.iface)
	if err := m.iface_Down(context.Background()); err != nil {
		t.Fatalf("iface_Down: %v", err)
	}
	if f.sawContains("awg-quick down") || f.sawContains("ip link delete") {
		t.Fatalf("unit stop sufficed; no direct teardown expected: calls=%v", f.calls)
	}
}

// Manually-launched iface (awg-quick up outside the unit): the unit stop is a
// no-op for the link, so iface_Down must fall through to `awg-quick down`.
func TestIfaceDownManualLaunch(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	linkPresentUntil(f, m.iface, "awg-quick down "+m.iface)
	if err := m.iface_Down(context.Background()); err != nil {
		t.Fatalf("iface_Down: %v", err)
	}
	if !f.sawContains("awg-quick down " + m.iface) {
		t.Fatalf("manual launch must be taken down via awg-quick; calls=%v", f.calls)
	}
	if f.sawContains("ip link delete") {
		t.Fatalf("awg-quick down sufficed; no force-delete expected: calls=%v", f.calls)
	}
}

// Non-systemd box: systemctl fails outright, but awg-quick down still works —
// iface_Down must report success (outcome-judged, not exit-code-judged).
func TestIfaceDownNoSystemd(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	f.errs["systemctl disable --now awg-quick@"+m.iface] = errFake
	linkPresentUntil(f, m.iface, "awg-quick down "+m.iface)
	if err := m.iface_Down(context.Background()); err != nil {
		t.Fatalf("iface_Down without systemd: %v", err)
	}
	if !f.sawContains("awg-quick down " + m.iface) {
		t.Fatalf("expected awg-quick down fallback; calls=%v", f.calls)
	}
}

// awg-tools broken/missing while the module keeps the iface alive: the last
// resort is `ip link delete` plus the RBOX-AWG-* chain teardown (PostDown never
// ran on this path).
func TestIfaceDownForceDelete(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	f.errs["awg-quick down "+m.iface] = errFake
	linkPresentUntil(f, m.iface, "ip link delete dev "+m.iface)
	if err := m.iface_Down(context.Background()); err != nil {
		t.Fatalf("iface_Down force path: %v", err)
	}
	if !f.sawContains("ip link delete dev " + m.iface) {
		t.Fatalf("expected ip link delete fallback; calls=%v", f.calls)
	}
	for _, chain := range []string{"RBOX-AWG-NAT", "RBOX-AWG-FWD", "RBOX-AWG-IN"} {
		if !f.sawContains("-F " + chain) {
			t.Fatalf("expected NAT cleanup of %s; calls=%v", chain, f.calls)
		}
	}
}

// Nothing takes the link down -> iface_Down must fail loudly (a silent success
// here is exactly the reported bug: kernel keeps running, panel looks clean).
func TestIfaceDownStillUpFails(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	f.errs["awg-quick down "+m.iface] = errFake
	linkPresentUntil(f, m.iface, "\x00never") // link never goes away
	if err := m.iface_Down(context.Background()); err == nil {
		t.Fatal("want error when the interface survives every teardown step")
	}
}

// kernel -> singbox: the switch must decommission the kernel runtime (unit
// disable at minimum), even though the panel state says "disabled".
func TestPrepareBackendSwitchKernelToSingbox(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	m.SetBackend("kernel")
	if err := m.PrepareBackendSwitch(context.Background(), "singbox"); err != nil {
		t.Fatalf("PrepareBackendSwitch: %v", err)
	}
	if !f.sawContains("systemctl disable --now awg-quick@" + m.iface) {
		t.Fatalf("switch must clear the kernel unit's boot persistence; calls=%v", f.calls)
	}
}

// Same-backend "switch" is a no-op: nothing must be torn down.
func TestPrepareBackendSwitchSameBackendNoop(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	m.SetBackend("singbox")
	if err := m.PrepareBackendSwitch(context.Background(), "singbox"); err != nil {
		t.Fatalf("PrepareBackendSwitch: %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("same-backend switch must run nothing; calls=%v", f.calls)
	}
}

// singbox -> kernel: the switch must drop the managed endpoint from the active
// config (idempotent) and reload on a real change.
func TestPrepareBackendSwitchSingboxToKernel(t *testing.T) {
	m, fs, applyCount := newSingboxMgr(t)
	if err := m.PrepareBackendSwitch(context.Background(), "kernel"); err != nil {
		t.Fatalf("PrepareBackendSwitch: %v", err)
	}
	if fs.calls != 1 || fs.lastSpec != nil {
		t.Fatalf("expected one removal sync (nil spec); calls=%d spec=%#v", fs.calls, fs.lastSpec)
	}
	if *applyCount != 1 {
		t.Fatalf("removal changed the config; apply calls = %d, want 1", *applyCount)
	}
}

func TestPrepareBackendSwitchInvalidTarget(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	if err := m.PrepareBackendSwitch(context.Background(), "wireguard"); err == nil {
		t.Fatal("want error on an unknown backend target")
	}
}

// Boot reconcile, singbox backend: a still-enabled kernel unit (residue of a
// pre-fix switch) must be decommissioned — this is what heals installs already
// in the reported broken state.
func TestReconcileResidueSingboxKillsKernelLeftover(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	m.SetBackend("singbox")
	// RouteBox-rendered conf exists -> the iface name is ours.
	os.MkdirAll(filepath.Dir(m.confPath), 0700)
	os.WriteFile(m.confPath, []byte("[Interface]\n"), 0600)
	f.outputs["systemctl is-enabled awg-quick@"+m.iface] = "enabled\n"
	linkPresentUntil(f, m.iface, "systemctl disable --now awg-quick@"+m.iface)

	m.ReconcileBackendResidue(context.Background())
	if !f.sawContains("systemctl disable --now awg-quick@" + m.iface) {
		t.Fatalf("boot reconcile must decommission the kernel leftover; calls=%v", f.calls)
	}
}

// Boot reconcile must NOT touch an iface RouteBox never rendered a conf for —
// an operator's unrelated awg-quick setup with our default name stays alive.
func TestReconcileResidueSingboxNoConfNoTouch(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	m.SetBackend("singbox")
	m.confPath = filepath.Join(t.TempDir(), "absent", "awg-rb0.conf")
	f.outputs["systemctl is-enabled awg-quick@"+m.iface] = "enabled\n"

	m.ReconcileBackendResidue(context.Background())
	if f.sawContains("systemctl disable") || f.sawContains("awg-quick down") || f.sawContains("ip link delete") {
		t.Fatalf("no RouteBox conf -> not our iface -> must not be torn down; calls=%v", f.calls)
	}
}

// Boot reconcile, singbox backend with a clean system: probes only, no teardown.
func TestReconcileResidueSingboxCleanNoop(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	m.SetBackend("singbox")
	os.MkdirAll(filepath.Dir(m.confPath), 0700)
	os.WriteFile(m.confPath, []byte("[Interface]\n"), 0600)
	f.errs["systemctl is-enabled awg-quick@"+m.iface] = errFake // not-enabled
	// default fake: `ip link show` returns ("", nil) -> absent

	m.ReconcileBackendResidue(context.Background())
	if f.sawContains("systemctl disable") || f.sawContains("awg-quick down") || f.sawContains("ip link delete") {
		t.Fatalf("clean system -> reconcile must be probe-only; calls=%v", f.calls)
	}
}

// Boot reconcile, kernel backend: an orphaned managed endpoint in the active
// config is removed (and the config reloaded).
func TestReconcileResidueKernelRemovesOrphanEndpoint(t *testing.T) {
	m, fs, applyCount := newSingboxMgr(t)
	m.SetBackend("kernel")
	m.ReconcileBackendResidue(context.Background())
	if fs.calls != 1 || fs.lastSpec != nil {
		t.Fatalf("expected one removal sync (nil spec); calls=%d spec=%#v", fs.calls, fs.lastSpec)
	}
	if *applyCount != 1 {
		t.Fatalf("apply calls = %d, want 1", *applyCount)
	}
}

// TestIfaceUpWithoutSystemd: bringing an interface up used to require systemd,
// so a host (or container) without it could tear one down but never start one.
// awg-quick is driven directly instead, and because `up` refuses an existing
// link, a live one is taken down first so a changed port or obfuscation reaches
// it.
func TestIfaceUpWithoutSystemd(t *testing.T) {
	withoutSystemd(t)

	t.Run("no link yet: straight up, and never touches systemctl", func(t *testing.T) {
		f := newFakeRunner()
		m := newTestManager(t, f)
		linkPresentUntil(f, m.iface, "\x00never-present") // link absent throughout
		f.match = func(name string, args []string) (string, bool) {
			if name == "ip" {
				return "", true // absent: empty output
			}
			return "", false
		}
		if err := m.iface_Up(context.Background()); err != nil {
			t.Fatalf("iface_Up: %v", err)
		}
		if f.sawContains("systemctl") {
			t.Error("systemctl must not be called when it is not installed")
		}
		if !f.sawContains("awg-quick up " + m.iface) {
			t.Fatalf("expected `awg-quick up %s`, got %v", m.iface, f.calls)
		}
	})

	t.Run("existing link is re-applied, not left stale", func(t *testing.T) {
		f := newFakeRunner()
		m := newTestManager(t, f)
		linkPresentUntil(f, m.iface, "awg-quick down "+m.iface)
		if err := m.iface_Up(context.Background()); err != nil {
			t.Fatalf("iface_Up: %v", err)
		}
		down, up := f.indexOf("awg-quick down "+m.iface), f.indexOf("awg-quick up "+m.iface)
		if down < 0 || up < 0 || down > up {
			t.Fatalf("a live link must be taken down before up, got %v", f.calls)
		}
	})
}

// TestRestoreKernelIface: with systemd the unit owns boot persistence; without
// one, a restarted container would show an enabled server with no interface.
func TestRestoreKernelIface(t *testing.T) {
	writeConf := func(t *testing.T, m *Manager) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(m.confPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(m.confPath, []byte("[Interface]\nPrivateKey = x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("brings it up", func(t *testing.T) {
		withoutSystemd(t)
		f := newFakeRunner()
		m := newTestManager(t, f)
		writeConf(t, m)
		if err := m.RestoreKernelIface(context.Background(), true); err != nil {
			t.Fatalf("RestoreKernelIface: %v", err)
		}
		if !f.sawContains("awg-quick up " + m.iface) {
			t.Fatalf("expected the interface to be restored, got %v", f.calls)
		}
	})

	t.Run("disabled server is left alone", func(t *testing.T) {
		withoutSystemd(t)
		f := newFakeRunner()
		m := newTestManager(t, f)
		writeConf(t, m)
		if err := m.RestoreKernelIface(context.Background(), false); err != nil {
			t.Fatal(err)
		}
		if f.sawContains("awg-quick") {
			t.Fatalf("must not start a server the operator disabled: %v", f.calls)
		}
	})

	t.Run("systemd present: the unit owns this", func(t *testing.T) {
		withSystemd(t)
		f := newFakeRunner()
		m := newTestManager(t, f)
		writeConf(t, m)
		if err := m.RestoreKernelIface(context.Background(), true); err != nil {
			t.Fatal(err)
		}
		if f.sawContains("awg-quick") {
			t.Fatalf("systemd installs must not be second-guessed: %v", f.calls)
		}
	})

	t.Run("never configured: nothing to restore", func(t *testing.T) {
		withoutSystemd(t)
		f := newFakeRunner()
		m := newTestManager(t, f) // no conf written
		if err := m.RestoreKernelIface(context.Background(), true); err != nil {
			t.Fatal(err)
		}
		if f.sawContains("awg-quick") {
			t.Fatalf("no conf on disk means nothing to bring up: %v", f.calls)
		}
	})
}

// withSystemd pins the systemd bring-up path for tests that assert on it, so
// they do not depend on whether the machine running them booted systemd. The
// seam is systemdRunning (the /run/systemd/system probe), not lookPath — a
// systemctl binary on disk proves nothing about the running init.
func withSystemd(t *testing.T) {
	t.Helper()
	orig := systemdRunning
	systemdRunning = func() bool { return true }
	t.Cleanup(func() { systemdRunning = orig })
}

// withoutSystemd pins systemd as not running (not PID 1) for one test.
func withoutSystemd(t *testing.T) {
	t.Helper()
	orig := systemdRunning
	systemdRunning = func() bool { return false }
	t.Cleanup(func() { systemdRunning = orig })
}
