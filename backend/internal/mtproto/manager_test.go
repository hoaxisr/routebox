package mtproto

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	secretA = "00112233445566778899aabbccddeeff"
	secretB = "11112233445566778899aabbccddeeff"
	secretC = "22112233445566778899aabbccddeeff"
)

func testManager(t *testing.T) *Manager {
	t.Helper()

	m := NewManager(NewStore(""))
	t.Cleanup(func() { _ = m.Stop() })

	return m
}

// freeAddr reserves a loopback port and hands it back, so a test can bind it.
func freeAddr(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	defer l.Close()

	return "127.0.0.1:" + strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
}

func testConfig(t *testing.T) Config {
	t.Helper()

	return Config{Listen: freeAddr(t), MaskingDomain: "example.com"}
}

func TestStartListensAndStopReleasesThePort(t *testing.T) {
	m := testManager(t)
	if err := m.Store().Put(Client{Name: "alice", Secret: secretA, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig(t)
	if err := m.Start(cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	status := m.Status()
	if !status.Running || status.Clients != 1 {
		t.Errorf("Status = %+v, want running with 1 client", status)
	}

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// The port must come free, or restarting from the panel would fail.
	l, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		t.Fatalf("port still held after Stop: %v", err)
	}

	_ = l.Close()
}

func TestStartWithNoActiveClientsSaysSo(t *testing.T) {
	m := testManager(t)

	if err := m.Start(testConfig(t)); !errors.Is(err, ErrNoActiveClients) {
		t.Errorf("err = %v, want ErrNoActiveClients", err)
	}
}

func TestStartWithoutAMaskingDomainSaysSo(t *testing.T) {
	m := testManager(t)
	if err := m.Store().Put(Client{Name: "alice", Secret: secretA, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig(t)
	cfg.MaskingDomain = ""

	// mtglib.Secret.Valid() requires a host, so without this the operator would
	// see a bare "secret is invalid" and no clue which field to fix.
	if err := m.Start(cfg); !errors.Is(err, ErrNoMaskingDomain) {
		t.Errorf("err = %v, want ErrNoMaskingDomain", err)
	}
}

func TestStartRejectsAMalformedSecret(t *testing.T) {
	m := testManager(t)
	if err := m.Store().Put(Client{Name: "broken", Secret: "not-hex", Enabled: true}); err != nil {
		t.Fatal(err)
	}

	err := m.Start(testConfig(t))
	if err == nil {
		t.Fatal("a malformed secret must not start a proxy")
	}

	// The name matters: with a roster of thirty, "invalid secret" is useless.
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("err = %v, want it to name the offending client", err)
	}
}

func TestStartIsIdempotentlyRefusedWhileRunning(t *testing.T) {
	m := testManager(t)
	if err := m.Store().Put(Client{Name: "alice", Secret: secretA, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	if err := m.Start(testConfig(t)); err != nil {
		t.Fatal(err)
	}

	if err := m.Start(testConfig(t)); err == nil {
		t.Error("a second Start must fail rather than leak the first listener")
	}
}

func TestStopImmediatelyAfterStart(t *testing.T) {
	// Stopping before the serve goroutine has been scheduled used to have
	// mtglib's Shutdown wait on a counter Serve was about to increment. Run it
	// repeatedly so the scheduler actually lands in that window.
	for range 20 {
		m := NewManager(NewStore(""))
		if err := m.Store().Put(Client{Name: "alice", Secret: secretA, Enabled: true}); err != nil {
			t.Fatal(err)
		}

		if err := m.Start(testConfig(t)); err != nil {
			t.Fatal(err)
		}

		if err := m.Stop(); err != nil {
			t.Fatalf("Stop: %v", err)
		}
	}
}

func TestStopOnAStoppedManagerIsFine(t *testing.T) {
	m := testManager(t)

	if err := m.Stop(); err != nil {
		t.Errorf("Stop on a stopped manager returned %v, want nil", err)
	}
}

func TestStatusOnAStoppedManager(t *testing.T) {
	m := testManager(t)

	if status := m.Status(); status.Running {
		t.Errorf("Status = %+v, want stopped", status)
	}
}

func TestDisabledAndExpiredClientsAreNotServed(t *testing.T) {
	m := testManager(t)
	past := time.Now().Add(-time.Hour).Unix()

	for _, c := range []Client{
		{Name: "on", Secret: secretA, Enabled: true},
		{Name: "off", Secret: secretB, Enabled: false},
		{Name: "gone", Secret: secretC, Enabled: true, ExpiresAt: past},
	} {
		if err := m.Store().Put(c); err != nil {
			t.Fatal(err)
		}
	}

	if err := m.Start(testConfig(t)); err != nil {
		t.Fatal(err)
	}

	if n := m.Status().Clients; n != 1 {
		t.Errorf("Clients = %d, want only the enabled unexpired one", n)
	}
}

func TestRebuildPicksUpANewClient(t *testing.T) {
	m := testManager(t)
	if err := m.Store().Put(Client{Name: "alice", Secret: secretA, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	if err := m.Start(testConfig(t)); err != nil {
		t.Fatal(err)
	}

	if err := m.Store().Put(Client{Name: "bob", Secret: secretB, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	if err := m.Rebuild(); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	if n := m.Status().Clients; n != 2 {
		t.Errorf("Clients = %d, want 2 after Rebuild", n)
	}
}

func TestRebuildKeepsTheSameListenAddress(t *testing.T) {
	m := testManager(t)
	if err := m.Store().Put(Client{Name: "alice", Secret: secretA, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig(t)
	if err := m.Start(cfg); err != nil {
		t.Fatal(err)
	}

	if err := m.Rebuild(); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	// A rebuild that moved the port would silently break every client.
	if got := m.Status().Listen; got != cfg.Listen {
		t.Errorf("Listen = %q, want %q", got, cfg.Listen)
	}
}

func TestRebuildOnAStoppedManagerDoesNothing(t *testing.T) {
	m := testManager(t)

	if err := m.Rebuild(); err != nil {
		t.Errorf("Rebuild while stopped returned %v, want nil", err)
	}

	if m.Status().Running {
		t.Error("Rebuild must not start a proxy that was never started")
	}
}

func TestRebuildToAnEmptyRosterStopsTheProxy(t *testing.T) {
	m := testManager(t)
	if err := m.Store().Put(Client{Name: "alice", Secret: secretA, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	if err := m.Start(testConfig(t)); err != nil {
		t.Fatal(err)
	}

	if err := m.Store().Delete("alice"); err != nil {
		t.Fatal(err)
	}

	err := m.Rebuild()

	// Deleting the last client cannot leave the old secret still being served.
	if !errors.Is(err, ErrNoActiveClients) {
		t.Errorf("err = %v, want ErrNoActiveClients", err)
	}

	if m.Status().Running {
		t.Error("the proxy must be stopped, not left serving a revoked secret")
	}
}

func TestEventsIsNilBeforeStart(t *testing.T) {
	m := testManager(t)

	if m.Events() != nil {
		t.Error("Events must be nil until a proxy exists, so the flush loop can skip it")
	}
}

func TestEventsIsUsableAfterStart(t *testing.T) {
	m := testManager(t)
	if err := m.Store().Put(Client{Name: "alice", Secret: secretA, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	if err := m.Start(testConfig(t)); err != nil {
		t.Fatal(err)
	}

	if m.Events() == nil {
		t.Fatal("Events must exist once the proxy is running")
	}

	if conns := m.Events().Connections(); len(conns) != 0 {
		t.Errorf("Connections = %+v, want empty on a fresh proxy", conns)
	}
}

func TestStartOnABusyPortFails(t *testing.T) {
	m := testManager(t)
	if err := m.Store().Put(Client{Name: "alice", Secret: secretA, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	defer busy.Close()

	cfg := Config{Listen: busy.Addr().String(), MaskingDomain: "example.com"}

	if err := m.Start(cfg); err == nil {
		t.Error("binding an occupied port must fail")
	}

	if m.Status().Running {
		t.Error("a failed Start must leave the manager stopped, not half-up")
	}
}
