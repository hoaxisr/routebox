package awg

import (
	"context"
	"testing"
)

// I1: statusSingbox must compute ConfigDirty like the kernel Status does —
// enabled AND the saved settings (desired()) differ from the running snapshot on
// any re-apply field (subnet/port/mtu/obf incl. CPA-RAT/preset/header protection).
// Without it the Apply banner never shows on singbox and a settings change is
// silently deferred until the next restart+sweep.
func TestStatusSingbox_ConfigDirty(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	desired := singboxEnableInput()
	m.SetDesired(func() EnableInput { return desired })
	if err := m.Enable(context.Background(), desired); err != nil {
		t.Fatal(err)
	}
	if m.Status(context.Background()).ConfigDirty {
		t.Fatal("must be clean right after enable")
	}
	desired.Subnet = "10.20.0.0/24" // operator changed a setting
	if !m.Status(context.Background()).ConfigDirty {
		t.Fatal("expected dirty after subnet change in saved settings")
	}
}

// The header-protection toggle flips a shared secret in the rendered endpoint, so
// changing it alone MUST flag ConfigDirty (the exact silent-deferral footgun I1
// is about).
func TestStatusSingbox_ConfigDirtyOnHeaderProtectionFlip(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	desired := singboxEnableInput()
	m.SetDesired(func() EnableInput { return desired })
	if err := m.Enable(context.Background(), desired); err != nil {
		t.Fatal(err)
	}
	if m.Status(context.Background()).ConfigDirty {
		t.Fatal("must be clean right after enable")
	}
	desired.HeaderProtection = true
	if !m.Status(context.Background()).ConfigDirty {
		t.Fatal("flipping header_protection alone must flag ConfigDirty")
	}
}

// Changing only CPA/RAT (awg3 obf strings) must flag dirty too — they ride in
// the Obf compare.
func TestStatusSingbox_ConfigDirtyOnCPAChange(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	desired := singboxEnableInput()
	m.SetDesired(func() EnableInput { return desired })
	if err := m.Enable(context.Background(), desired); err != nil {
		t.Fatal(err)
	}
	desired.Obf.CPA = "10-20"
	if !m.Status(context.Background()).ConfigDirty {
		t.Fatal("changing obf CPA alone must flag ConfigDirty")
	}
}

// The 4 AWG3 device-timers (RekeyTimeout/RejectAfterTime/KeepaliveTimeout/
// MaxHandshakeAttempts) ride in the Obf struct compare exactly like CPA/RAT. If
// the desired() mapper carries them, the SAME saved settings stay clean; if a
// mapper drops them (the awgDesired bug), the running m.obf keeps them while
// desired().Obf zeroes them → the Obf compare mismatches and ConfigDirty is
// eternally true. This guards both halves.
func TestStatusSingbox_ConfigDirtyDeviceTimers(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	desired := singboxEnableInput()
	// Canonical UintRange strings so validateObf leaves them unchanged (raw == running).
	desired.Obf.RekeyTimeout = "5"
	desired.Obf.RejectAfterTime = "180"
	desired.Obf.KeepaliveTimeout = "25"
	desired.Obf.MaxHandshakeAttempts = "18"
	m.SetDesired(func() EnableInput { return desired })
	if err := m.Enable(context.Background(), desired); err != nil {
		t.Fatal(err)
	}
	// desired() carries the SAME timers the running config was enabled with:
	// no eternal-dirty (the fixed awgDesired behaviour).
	if m.Status(context.Background()).ConfigDirty {
		t.Fatal("timers present on both running and desired must read clean, not eternal-dirty")
	}
	// Simulate a desired-mapper that DROPS the timers (the awgDesired bug): the
	// Obf compare must now flag dirty — proving the timers participate in it.
	stale := desired
	stale.Obf.RekeyTimeout, stale.Obf.RejectAfterTime = "", ""
	stale.Obf.KeepaliveTimeout, stale.Obf.MaxHandshakeAttempts = "", ""
	m.SetDesired(func() EnableInput { return stale })
	if !m.Status(context.Background()).ConfigDirty {
		t.Fatal("a desired-mapper dropping device-timers must read dirty — timers ride in the Obf compare")
	}
}

// A restart (RehydrateSingbox from the SAME saved settings desired() returns)
// must come up clean — including when header protection is on. A false-dirty
// here would show a phantom Apply banner after every RouteBox restart.
func TestStatusSingbox_RehydrateNotDirty(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	if err := m.store.SetServerKey(m.serverPriv); err != nil {
		t.Fatal(err)
	}
	desired := singboxEnableInput()
	desired.HeaderProtection = true
	m.SetDesired(func() EnableInput { return desired })
	m.RehydrateSingbox(desired, true)
	st := m.Status(context.Background())
	if !st.Enabled {
		t.Fatal("rehydrate with enabled=true must report enabled")
	}
	if st.ConfigDirty {
		t.Fatal("rehydrate from the saved settings must not report dirty")
	}
}
