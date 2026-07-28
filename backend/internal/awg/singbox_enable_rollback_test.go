package awg

import (
	"context"
	"strings"
	"testing"

	"routebox/backend/internal/config"
)

// enableSingbox commits the render parameters (port, mtu, subnet, obfuscation,
// keys) BEFORE it tries to write the config, because renderServerSpec reads them
// under the same lock. When the write fails, enableFailCause only rolls back
// enabled/lastErr/phase — the parameters stayed. RenderClientConf and
// ClientEndpoint read them without looking at enabled, so every .conf and QR
// handed out afterwards described a server that was never applied: the operator
// changes the port, Enable fails on a read-only config, the server keeps serving
// the old port, and the clients get the new one.
func TestSingbox_FailedEnableKeepsTheAppliedRenderParameters(t *testing.T) {
	ctx := context.Background()
	m, fs, _ := newSingboxMgr(t)
	if err := m.Enable(ctx, singboxEnableInput()); err != nil {
		t.Fatal(err)
	}
	sum, err := m.AddPeer(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}

	// A second Enable with new parameters that cannot be written.
	fs.err = errRO()
	next := singboxEnableInput()
	next.ListenPort = 51821
	next.MTU = 1380
	if err := m.Enable(ctx, next); err == nil {
		t.Fatal("Enable must fail when the config cannot be written")
	}

	// The server is still the one on disk: port 51820, MTU 1408.
	conf, err := m.RenderClientConf(sum.PublicKey, "vpn.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(conf, "vpn.example.com:51820") {
		t.Fatalf("client .conf points at a port that was never applied:\n%s", conf)
	}
	if !strings.Contains(conf, "MTU = 1408") {
		t.Fatalf("client .conf carries an MTU that was never applied:\n%s", conf)
	}

	ep, err := m.ClientEndpoint(sum.PublicKey, "alice", "vpn.example.com")
	if err != nil {
		t.Fatal(err)
	}
	peers, _ := ep["peers"].([]interface{})
	if len(peers) != 1 {
		t.Fatalf("client endpoint has %d peers, want 1", len(peers))
	}
	peer, _ := peers[0].(map[string]interface{})
	if got := peer["port"]; got != 51820 {
		t.Fatalf("client endpoint peer port = %v, want 51820", got)
	}
	if got := ep["mtu"]; got != 1408 {
		t.Fatalf("client endpoint mtu = %v, want 1408", got)
	}

	m.mu.Lock()
	port, mtu := m.listenPort, m.mtu
	m.mu.Unlock()
	if port != 51820 || mtu != 1408 {
		t.Fatalf("in-memory render state = port %d mtu %d, want 51820/1408", port, mtu)
	}
}

// A re-Enable that fails is a failed RE-CONFIGURE, not a shutdown: the endpoint
// the operator changed is still in the config and still being served. Reporting
// the server as disabled hands the 30s sweep an instruction — renderServerSpec
// gates on m.enabled, so the next tick syncs a nil spec and removes a live
// endpoint nobody asked to stop. (The persisted awg.enabled is not touched by a
// failed Enable either, so a restart would bring the server back and disagree
// with what the panel showed.)
func TestSingbox_FailedReEnableKeepsAWorkingServerEnabled(t *testing.T) {
	ctx := context.Background()
	m, fs, _ := newSingboxMgr(t)
	if err := m.Enable(ctx, singboxEnableInput()); err != nil {
		t.Fatal(err)
	}

	fs.err = errRO()
	next := singboxEnableInput()
	next.ListenPort = 51821
	if err := m.Enable(ctx, next); err == nil {
		t.Fatal("Enable must fail when the config cannot be written")
	}

	if st := m.Status(ctx); !st.Enabled {
		t.Fatal("the endpoint never changed — a server that was up must stay up")
	}

	// And the sweep must re-render the server that IS running, not tear it down.
	fs.err = nil
	fs.lastSpec = nil
	m.SweepExpired(ctx)
	if fs.lastSpec == nil {
		t.Fatal("the sweep removed the endpoint of a server that was up and serving")
	}
	if fs.lastSpec.ListenPort != 51820 {
		t.Fatalf("the sweep wrote port %d; the running server is on 51820", fs.lastSpec.ListenPort)
	}
}

// Same for a submission rejected before anything is written at all: a typo in
// the subnet field must not take a running server down 30 seconds later.
func TestSingbox_RejectedEnableInputKeepsAWorkingServerEnabled(t *testing.T) {
	ctx := context.Background()
	m, fs, _ := newSingboxMgr(t)
	if err := m.Enable(ctx, singboxEnableInput()); err != nil {
		t.Fatal(err)
	}

	bad := singboxEnableInput()
	bad.Subnet = "not-a-subnet"
	if err := m.Enable(ctx, bad); err == nil {
		t.Fatal("Enable must reject an invalid subnet")
	}
	if st := m.Status(ctx); !st.Enabled {
		t.Fatal("a rejected submission never reached the config — the server must stay up")
	}

	fs.lastSpec = nil
	m.SweepExpired(ctx)
	if fs.lastSpec == nil {
		t.Fatal("the sweep removed the endpoint of a server that was up and serving")
	}
}

// The other direction, and the reason the rollback restores a snapshot rather
// than hardcoding a value: a first Enable that fails leaves nothing serving, so
// the server must read as disabled and the sweep must clear whatever half-write
// the failed attempt left behind.
func TestSingbox_FailedFirstEnableLeavesTheServerDisabled(t *testing.T) {
	ctx := context.Background()
	m, fs, _ := newSingboxMgrDisabled(t)
	fs.err = errRO()

	if err := m.Enable(ctx, singboxEnableInput()); err == nil {
		t.Fatal("Enable must fail when the config cannot be written")
	}
	if st := m.Status(ctx); st.Enabled {
		t.Fatal("a server that never came up must not read as enabled")
	}

	fs.err = nil
	fs.lastSpec = &config.AwgServerSpec{PrivateKey: "sentinel"}
	m.SweepExpired(ctx)
	if fs.lastSpec != nil {
		t.Fatalf("the sweep served a server whose Enable never succeeded: %#v", fs.lastSpec)
	}
}

// The rollback must not undo a SUCCESSFUL enable: the new parameters stand.
func TestSingbox_SuccessfulEnableAppliesTheNewParameters(t *testing.T) {
	ctx := context.Background()
	m, _, _ := newSingboxMgr(t)
	if err := m.Enable(ctx, singboxEnableInput()); err != nil {
		t.Fatal(err)
	}
	next := singboxEnableInput()
	next.ListenPort = 51821
	next.MTU = 1380
	if err := m.Enable(ctx, next); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	port, mtu := m.listenPort, m.mtu
	m.mu.Unlock()
	if port != 51821 || mtu != 1380 {
		t.Fatalf("render state = port %d mtu %d, want 51821/1380", port, mtu)
	}
}
