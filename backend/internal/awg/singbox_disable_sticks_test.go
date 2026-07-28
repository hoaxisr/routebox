package awg

import (
	"context"
	"errors"
	"testing"
)

var errReloadFailed = errors.New("reload failed")

// failingApply rewires the manager so the endpoint write succeeds and the
// reload that follows does not — the "config written, apply refused" case that
// decommissionSingbox used to flatten into the same error as "nothing was
// written at all".
func failingApply(m *Manager, fs *fakeSync) {
	m.SetConfigSync(fs, func() error { return errReloadFailed }, func() bool { return true })
}

// The endpoint is gone from the config the moment the sync returns; only the
// reload failed. If m.enabled stays true, the next 30s sweep renders a full
// spec and writes the server straight back — a server the operator just turned
// off comes back on by itself.
func TestSingbox_DisableSticksWhenTheReloadFails(t *testing.T) {
	ctx := context.Background()
	m, fs, _ := newSingboxMgr(t)
	if err := m.Enable(ctx, singboxEnableInput()); err != nil {
		t.Fatal(err)
	}
	failingApply(m, fs)

	if err := m.Disable(ctx); !errors.Is(err, errReloadFailed) {
		t.Fatalf("Disable error = %v, want the reload failure reported", err)
	}
	if st := m.Status(ctx); st.Enabled {
		t.Fatal("the server must read as disabled: its endpoint is out of the config")
	}

	fs.lastSpec = nil
	m.SweepExpired(ctx)
	if fs.lastSpec != nil {
		t.Fatalf("the sweep wrote the disabled server back: %#v", fs.lastSpec)
	}
}

// Same hole on the singbox -> kernel switch: it decommissions the endpoint, and
// if the reload fails the caller aborts the switch — leaving the singbox
// backend selected with enabled still true and the sweep ready to resurrect it.
func TestSingbox_BackendSwitchToKernelSticksWhenTheReloadFails(t *testing.T) {
	ctx := context.Background()
	m, fs, _ := newSingboxMgr(t)
	if err := m.Enable(ctx, singboxEnableInput()); err != nil {
		t.Fatal(err)
	}
	failingApply(m, fs)

	if err := m.PrepareBackendSwitch(ctx, "kernel"); !errors.Is(err, errReloadFailed) {
		t.Fatalf("PrepareBackendSwitch error = %v, want the reload failure reported", err)
	}

	fs.lastSpec = nil
	m.SweepExpired(ctx)
	if fs.lastSpec != nil {
		t.Fatalf("the sweep wrote the decommissioned server back: %#v", fs.lastSpec)
	}
}

// A decommission that never reached the config must NOT read as disabled: the
// server is still being served, and pretending otherwise strands it outside the
// panel's view.
func TestSingbox_DisableKeepsEnabledWhenTheConfigWasNotWritten(t *testing.T) {
	ctx := context.Background()
	m, fs, _ := newSingboxMgr(t)
	if err := m.Enable(ctx, singboxEnableInput()); err != nil {
		t.Fatal(err)
	}
	fs.err = errRO()

	if err := m.Disable(ctx); err == nil {
		t.Fatal("Disable must fail when the endpoint cannot be removed")
	}
	if st := m.Status(ctx); !st.Enabled {
		t.Fatal("the endpoint is still in the config — the server must not read as disabled")
	}
}
