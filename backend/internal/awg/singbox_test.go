package awg

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"

	"routebox/backend/internal/config"
)

type fakeSync struct {
	lastSpec *config.AwgServerSpec
	changed  bool
	calls    int
	err      error
}

func (f *fakeSync) SyncAwgEndpointActive(tag string, spec *config.AwgServerSpec) (bool, error) {
	f.calls++
	f.lastSpec = spec
	if f.err != nil {
		return false, f.err
	}
	return f.changed, nil
}

func newSingboxMgr(t *testing.T) (*Manager, *fakeSync, *int) {
	t.Helper()
	priv, _, err := Generate(rand.Reader) // real 32-byte key so PublicFromPrivate paths work
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{
		store:      NewStore(""), // in-memory
		iface:      "awg-rb0",
		subnet:     "10.10.0.0/24",
		serverIP:   "10.10.0.1",
		listenPort: 51820,
		mtu:        1408,
		serverPriv: priv,
		enabled:    true, // renderServerSpec gates on enabled (Bug C1); harness = enabled server
		phase:      PhaseReady,
	}
	m.probeFn = func() bool { return false } // hermetic default: no test hits the real network
	fs := &fakeSync{changed: true}
	applyCount := 0
	m.SetBackend("singbox")
	m.SetConfigSync(fs, func() error { applyCount++; return nil }, func() bool { return true })
	return m, fs, &applyCount
}

func TestSingbox_AddPeer_SyncsAndApplies(t *testing.T) {
	m, fs, applyCount := newSingboxMgr(t)
	sum, err := m.AddPeer(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	if sum.Address == "" {
		t.Fatal("expected an allocated /32 address")
	}
	if fs.calls == 0 || fs.lastSpec == nil || len(fs.lastSpec.Peers) != 1 {
		t.Fatalf("sync not called with 1 peer: calls=%d spec=%#v", fs.calls, fs.lastSpec)
	}
	if *applyCount != 1 {
		t.Fatalf("apply calls = %d, want 1", *applyCount)
	}
	// The synced peer has NO address/port (server-side) — enforced by BuildAwgServerEndpoint.
	if fs.lastSpec.Peers[0].AllowedIP == "" {
		t.Fatal("peer AllowedIP missing")
	}
}

// singboxEnableInput is a canonical, valid Enable submission for the test harness.
func singboxEnableInput() EnableInput {
	return EnableInput{Subnet: "10.10.0.0/24", ListenPort: 51820, MTU: 1408}
}

// Bug C1 regression: Disable must STICK. After enable→disable, the 30s sweep
// (SweepExpired → singboxSync) must NOT resurrect the endpoint: renderServerSpec
// has to gate on m.enabled, not just on the (deliberately kept) server key.
func TestSingbox_DisableSticksThroughSweep(t *testing.T) {
	m, fs, _ := newSingboxMgr(t)

	// Enable through the real orchestrator: must write a non-nil spec (no regression).
	if err := m.Enable(context.Background(), singboxEnableInput()); err != nil {
		t.Fatal(err)
	}
	if fs.lastSpec == nil {
		t.Fatal("enable must sync a non-nil server spec")
	}
	if !m.Status(context.Background()).Enabled {
		t.Fatal("status must report enabled after Enable")
	}

	// Disable removes the endpoint (syncs nil).
	if err := m.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fs.lastSpec != nil {
		t.Fatal("disable must sync a nil spec")
	}

	// The sweep runs unconditionally every 30s. It must NOT re-create the endpoint.
	fs.lastSpec = &config.AwgServerSpec{PrivateKey: "sentinel"} // poison: overwritten by the sweep's sync call
	callsBefore := fs.calls
	m.SweepExpired(context.Background())
	if fs.calls == callsBefore {
		t.Fatal("sweep did not sync at all (expected a nil-spec no-op sync)")
	}
	if fs.lastSpec != nil {
		t.Fatalf("sweep resurrected a DISABLED server: synced spec %+v", fs.lastSpec)
	}
	if m.Status(context.Background()).Enabled {
		t.Fatal("status must stay disabled after the sweep")
	}
}

// Bug C1 regression: AddPeer while disabled must not resurrect the endpoint either.
func TestSingbox_AddPeerWhileDisabledDoesNotResurrect(t *testing.T) {
	m, fs, _ := newSingboxMgr(t)
	if err := m.Enable(context.Background(), singboxEnableInput()); err != nil {
		t.Fatal(err)
	}
	if err := m.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}
	fs.lastSpec = &config.AwgServerSpec{PrivateKey: "sentinel"}
	if _, err := m.AddPeer(context.Background(), "late"); err != nil {
		t.Fatal(err)
	}
	if fs.lastSpec != nil {
		t.Fatalf("AddPeer on a disabled server resurrected the endpoint: %+v", fs.lastSpec)
	}
}

// Bug C1 regression: a persisted-disabled server must stay disabled across a
// RouteBox restart — RehydrateSingbox takes the enabled flag from settings, not
// from "a server key exists" (the key is deliberately kept for reuse).
func TestSingbox_RehydratePersistedDisabled(t *testing.T) {
	m, fs, _ := newSingboxMgr(t)
	if err := m.store.SetServerKey(m.serverPriv); err != nil { // key persisted (reuse)
		t.Fatal(err)
	}

	m.RehydrateSingbox(singboxEnableInput(), false) // settings say Disabled
	if m.Status(context.Background()).Enabled {
		t.Fatal("rehydrate must honor persisted enabled=false despite a stored server key")
	}
	fs.lastSpec = &config.AwgServerSpec{PrivateKey: "sentinel"}
	m.SweepExpired(context.Background())
	if fs.lastSpec != nil {
		t.Fatalf("sweep after disabled rehydrate resurrected the endpoint: %+v", fs.lastSpec)
	}

	m.RehydrateSingbox(singboxEnableInput(), true) // settings say Enabled
	if !m.Status(context.Background()).Enabled {
		t.Fatal("rehydrate must honor persisted enabled=true")
	}
	if m.renderServerSpec() == nil {
		t.Fatal("enabled rehydrate must render a non-nil spec")
	}
}

// Enable ordering: renderServerSpec now gates on m.enabled, so enableSingbox must
// flip it BEFORE syncing — and roll it back when the sync fails.
func TestSingbox_EnableSyncFailureRollsBackEnabled(t *testing.T) {
	m, fs, _ := newSingboxMgr(t)
	fs.err = fmt.Errorf("boom")
	if err := m.Enable(context.Background(), singboxEnableInput()); err == nil {
		t.Fatal("enable must fail when the config sync fails")
	}
	if m.Status(context.Background()).Enabled {
		t.Fatal("failed enable must leave enabled=false")
	}
	// And the sweep afterwards must not write a spec.
	fs.err = nil
	fs.lastSpec = &config.AwgServerSpec{PrivateKey: "sentinel"}
	m.SweepExpired(context.Background())
	if fs.lastSpec != nil {
		t.Fatalf("sweep after failed enable resurrected the endpoint: %+v", fs.lastSpec)
	}
}

// Bug I1: Busy must report true while an Enable orchestrator is in flight or the
// manager sits in a transitional phase — the backend-switch guard keys on it.
func TestBusyDuringTransitionalPhases(t *testing.T) {
	m := &Manager{}
	if m.Busy() {
		t.Fatal("idle manager must not be busy")
	}
	m.inFlight = true
	if !m.Busy() {
		t.Fatal("inFlight manager must be busy")
	}
	m.inFlight = false
	for _, p := range []EnablePhase{PhaseValidating, PhaseInstalling, PhaseRendering, PhaseStarting, PhaseHealth} {
		m.phase = p
		if !m.Busy() {
			t.Fatalf("phase %q must report busy", p)
		}
	}
	for _, p := range []EnablePhase{PhaseIdle, PhaseReady, PhaseFailed, ""} {
		m.phase = p
		if m.Busy() {
			t.Fatalf("terminal phase %q must not report busy", p)
		}
	}
}

func TestSingbox_ExpiredPeerOmittedFromSpec(t *testing.T) {
	m, fs, _ := newSingboxMgr(t)
	m.AddPeer(context.Background(), "bob")
	pk := fs.lastSpec.Peers[0].PublicKey
	// Expire it in the past and re-sync.
	m.RenewPeer(context.Background(), pk, 1) // unix ts 1 = long past
	if len(fs.lastSpec.Peers) != 0 {
		t.Fatalf("expired peer must be omitted from spec, got %d", len(fs.lastSpec.Peers))
	}
}

func TestEnableSingboxIPv6BrokerActivates(t *testing.T) {
	m, fs, _ := newSingboxMgr(t)
	m.probeFn = func() bool { return true }
	in := singboxEnableInput()
	in.IPv6Broker = true
	if err := m.enableSingbox(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if !m.v6Active {
		t.Fatal("v6Active should be true when broker desired + probe ok")
	}
	if m.store.ULAPrefix() == "" {
		t.Fatal("ULA prefix should be generated + persisted")
	}
	if fs.lastSpec == nil || fs.lastSpec.Address6 == "" {
		t.Fatalf("server spec missing IPv6 address: %#v", fs.lastSpec)
	}
}

func TestEnableSingboxIPv6BrokerFallsBackNoEgress(t *testing.T) {
	m, fs, _ := newSingboxMgr(t)
	m.probeFn = func() bool { return false }
	in := singboxEnableInput()
	in.IPv6Broker = true
	if err := m.enableSingbox(context.Background(), in); err != nil {
		t.Fatal(err) // server still enables
	}
	if m.v6Active {
		t.Fatal("v6Active must be false when probe fails")
	}
	if fs.lastSpec != nil && fs.lastSpec.Address6 != "" {
		t.Fatal("no IPv6 must be emitted on fallback")
	}
}

func TestEnableSingboxIPv6BrokerRejectsLowMTU(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	m.probeFn = func() bool { return true }
	in := singboxEnableInput()
	in.IPv6Broker = true
	in.MTU = 1200
	if err := m.enableSingbox(context.Background(), in); err == nil {
		t.Fatal("MTU<1280 with broker must fail")
	}
}
