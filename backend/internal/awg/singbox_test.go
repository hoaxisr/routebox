package awg

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
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

// TestRenderClientConfDualStack: manager-level coverage that RenderClientConf
// actually wires m.v6Active/m.ulaPrefix through to the rendered .conf (a pure
// BuildClient test can't catch a wrong field/lock/prefix-arg wiring bug).
func TestRenderClientConfDualStack(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	m.probeFn = func() bool { return true }
	in := singboxEnableInput()
	in.IPv6Broker = true
	if err := m.enableSingbox(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	sum, err := m.AddPeer(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	conf, err := m.RenderClientConf(sum.PublicKey, "vps.example")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(conf, sum.Address) {
		t.Fatalf("client conf missing v4 address %q:\n%s", sum.Address, conf)
	}
	if !strings.Contains(conf, "/128") {
		t.Fatalf("client conf missing IPv6 /128 address:\n%s", conf)
	}
	if !strings.Contains(conf, "::/0") {
		t.Fatalf("client conf missing ::/0 in AllowedIPs:\n%s", conf)
	}
}

// TestClientEndpointDualStack: same as above but for the sing-box JSON endpoint
// export (ClientEndpoint).
func TestClientEndpointDualStack(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	m.probeFn = func() bool { return true }
	in := singboxEnableInput()
	in.IPv6Broker = true
	if err := m.enableSingbox(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	sum, err := m.AddPeer(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	ep, err := m.ClientEndpoint(sum.PublicKey, "alice", "vps.example")
	if err != nil {
		t.Fatal(err)
	}
	addr, ok := ep["address"].([]interface{})
	if !ok || len(addr) != 2 {
		t.Fatalf("expected 2-element dual-stack address, got %#v", ep["address"])
	}
	if !strings.Contains(fmt.Sprint(addr[1]), "/128") {
		t.Fatalf("second address element must be the /128 IPv6: %#v", addr)
	}
	peers, ok := ep["peers"].([]interface{})
	if !ok || len(peers) != 1 {
		t.Fatalf("expected 1 peer, got %#v", ep["peers"])
	}
	peer := peers[0].(map[string]interface{})
	allowed, ok := peer["allowed_ips"].([]interface{})
	if !ok {
		t.Fatalf("allowed_ips wrong type: %#v", peer["allowed_ips"])
	}
	found := false
	for _, a := range allowed {
		if a == "::/0" {
			found = true
		}
	}
	if !found {
		t.Fatalf("allowed_ips missing ::/0: %#v", allowed)
	}
}

// TestRenderClientConfV4OnlyNoV6: additivity through the manager — with the
// broker never activated, RenderClientConf must emit nothing IPv6-shaped.
func TestRenderClientConfV4OnlyNoV6(t *testing.T) {
	m, _, _ := newSingboxMgr(t) // m.probeFn already defaults to false (hermetic)
	if err := m.enableSingbox(context.Background(), singboxEnableInput()); err != nil {
		t.Fatal(err)
	}
	sum, err := m.AddPeer(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	conf, err := m.RenderClientConf(sum.PublicKey, "vps.example")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(conf, "/128") || strings.Contains(conf, "::/0") {
		t.Fatalf("v4-only conf must have no IPv6 traces:\n%s", conf)
	}
	for _, line := range strings.Split(conf, "\n") {
		if strings.HasPrefix(line, "Address = ") && strings.Contains(line, ",") {
			t.Fatalf("v4-only Address line must not be comma-joined: %q", line)
		}
	}
}

// TestStatusDirtyOnBrokerToggle: a desired IPv6Broker flip must flag ConfigDirty
// like any other re-appliable field (Task 8).
func TestStatusDirtyOnBrokerToggle(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	m.probeFn = func() bool { return true }
	in := singboxEnableInput()
	in.IPv6Broker = true
	_ = m.enableSingbox(context.Background(), in)
	// desired() now returns IPv6Broker=false -> dirty
	m.SetDesired(func() EnableInput { d := in; d.IPv6Broker = false; return d })
	if !m.statusSingbox(context.Background()).ConfigDirty {
		t.Fatal("broker desire change should flag config dirty")
	}
}

// TestServerClientV6Agreement: proves the server-side spec (renderServerSpec,
// observed via fs.lastSpec) and the client export (RenderClientConf) agree on
// the SAME peer v6 address — a wiring bug in either path would desync them.
func TestServerClientV6Agreement(t *testing.T) {
	m, fs, _ := newSingboxMgr(t)
	m.probeFn = func() bool { return true }
	in := singboxEnableInput()
	in.IPv6Broker = true
	if err := m.enableSingbox(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	sum, err := m.AddPeer(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	if fs.lastSpec == nil || len(fs.lastSpec.Peers) != 1 || fs.lastSpec.Peers[0].AllowedIP6 == "" {
		t.Fatalf("expected server spec with 1 peer carrying AllowedIP6: %#v", fs.lastSpec)
	}
	serverV6 := strings.TrimSuffix(fs.lastSpec.Peers[0].AllowedIP6, "/128")

	conf, err := m.RenderClientConf(sum.PublicKey, "vps.example")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(conf, serverV6) {
		t.Fatalf("client conf v6 address disagrees with server spec: server=%q conf=%s", serverV6, conf)
	}
}

// TestULAPrefixNotRegeneratedOnSecondEnable: enableSingbox must reuse the
// stored ULA prefix on subsequent enables (Fix 1: generate-once, not
// generate-if-probe-was-true-last-time).
func TestULAPrefixNotRegeneratedOnSecondEnable(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	m.probeFn = func() bool { return true }
	in := singboxEnableInput()
	in.IPv6Broker = true
	if err := m.enableSingbox(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	first := m.store.ULAPrefix()
	if first == "" {
		t.Fatal("expected a persisted ULA prefix after first enable")
	}
	if err := m.enableSingbox(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	second := m.store.ULAPrefix()
	if second != first {
		t.Fatalf("ULA prefix regenerated on second enable: first=%q second=%q", first, second)
	}
}

// TestSweepActivatesWhenEgressAppears: Fix 1 generates the ULA prefix even
// when the probe fails, so a later sweep (Fix 3) can auto-activate v6 once
// egress appears — without an operator re-Apply.
func TestSweepActivatesWhenEgressAppears(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	m.probeFn = func() bool { return false } // egress down at enable time
	in := singboxEnableInput()
	in.IPv6Broker = true
	if err := m.enableSingbox(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if m.v6Active {
		t.Fatal("v6Active must be false when the probe fails at enable time")
	}
	if m.store.ULAPrefix() == "" {
		t.Fatal("ULA prefix must be generated+persisted even when the probe fails (Fix 1)")
	}

	m.probeFn = func() bool { return true } // egress appears
	m.SweepExpired(context.Background())
	if !m.v6Active {
		t.Fatal("sweep must activate v6Active once egress appears (Fix 3)")
	}
}

// TestRehydrateRestoresBrokerState: a restart must re-derive v6Active via the
// same preflight (MTU floor, ULA prefix, egress probe), not just copy the
// desired bool (Task 8).
func TestRehydrateRestoresBrokerState(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	_ = m.store.SetULAPrefix("fd00:abcd::/64")
	m.probeFn = func() bool { return true }
	in := singboxEnableInput()
	in.IPv6Broker = true
	m.RehydrateSingbox(in, true)
	if !m.v6Active || m.ulaPrefix.Bits() != 64 {
		t.Fatalf("rehydrate did not restore broker state: active=%v pfx=%v", m.v6Active, m.ulaPrefix)
	}
	if !m.statusSingbox(context.Background()).IPv6Active {
		t.Fatal("status IPv6Active should reflect restored v6Active")
	}
}
