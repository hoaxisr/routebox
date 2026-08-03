package awg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// kernelAwg3EnableInput mirrors awg3EnableInput (singbox_awg3_test.go) for the
// kernel path: header protection on, every S-padding at the fork-required
// minimum of 12 (validateHPKConstraint), plus CPA/RAT and the device-timers.
func kernelAwg3EnableInput() EnableInput {
	in := goodEnableInput()
	in.HeaderProtection = true
	in.Obf = Obfuscation{S1: 12, S2: 12, S3: 12, S4: 12, CPA: "10-20", RAT: "120",
		RekeyTimeout: "5", RejectAfterTime: "180", KeepaliveTimeout: "25", MaxHandshakeAttempts: "18"}
	return in
}

func TestEnableKernel_AWG3Capable_RendersHeaderProtectionAndCPA(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	m.SetKernelSupportsAWG3(func() bool { return true })
	f.outputs["awg show awg-rb0"] = "interface: awg-rb0\n  listening port: 51820\n"
	f.outputs["iptables -t nat -S"] = "-N RBOX-AWG-NAT\n"

	if err := m.Enable(context.Background(), kernelAwg3EnableInput()); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	data, _ := os.ReadFile(m.confPath)
	for _, want := range []string{"ContentPaddingAddition = 10-20", "RekeyAfterTime = 120",
		"RekeyTimeout = 5", "RejectAfterTime = 180", "KeepaliveTimeout = 25", "MaxHandshakeAttempts = 18"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("kernel server conf missing %q on an awg3-capable host:\n%s", want, data)
		}
	}
	if !strings.Contains(string(data), "HeaderProtectionKey = ") {
		t.Fatalf("kernel server conf missing HeaderProtectionKey on an awg3-capable host:\n%s", data)
	}
	if m.headerKey == "" || m.headerKey != m.store.HeaderKey() {
		t.Fatalf("header key not persisted: m.headerKey=%q store=%q", m.headerKey, m.store.HeaderKey())
	}
}

func TestEnableKernel_HeaderProtection_RequiresCapability(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	// kernelSupports3Fn left nil (unset) -> fail-closed.
	err := m.Enable(context.Background(), kernelAwg3EnableInput())
	if err == nil {
		t.Fatal("enable with header protection must fail on an awg3-incapable kernel host")
	}
	if !strings.Contains(err.Error(), "awg3") {
		t.Fatalf("error must name the awg3 kernel requirement, got: %v", err)
	}
	if _, statErr := os.Stat(m.confPath); statErr == nil {
		data, _ := os.ReadFile(m.confPath)
		if strings.Contains(string(data), "HeaderProtectionKey") {
			t.Fatalf("HPK must not be emitted on an awg3-incapable kernel host:\n%s", data)
		}
	}
}

func TestEnableKernel_AWG3Capable_RequiresS12(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	m.SetKernelSupportsAWG3(func() bool { return true })
	in := kernelAwg3EnableInput()
	in.Obf.S1 = 4 // below the fork's S>=12 rule
	if err := m.Enable(context.Background(), in); err == nil {
		t.Fatal("enable with header protection and S1<12 must fail even on an awg3-capable host")
	}
}

// When unsupported, CPA/RAT/device-timers must still be silently stripped
// (pre-existing behaviour, unaffected by capability) rather than erroring —
// only header protection hard-gates.
func TestEnableKernel_AWG3Incapable_StripsObfWithoutError(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	m.SetKernelSupportsAWG3(func() bool { return false })
	f.outputs["awg show awg-rb0"] = "interface: awg-rb0\n  listening port: 51820\n"
	f.outputs["iptables -t nat -S"] = "-N RBOX-AWG-NAT\n"
	in := goodEnableInput()
	in.Obf = Obfuscation{CPA: "10-20", RAT: "120"}
	if err := m.Enable(context.Background(), in); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	data, _ := os.ReadFile(m.confPath)
	if strings.Contains(string(data), "ContentPaddingAddition") {
		t.Fatalf("awg3-incapable kernel host must not render CPA/RAT:\n%s", data)
	}
}

func TestRehydrateKernel_AWG3Capable_KeepsFieldsAndRestoresHeaderKey(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	m.SetKernelSupportsAWG3(func() bool { return true })
	if err := os.MkdirAll(filepath.Dir(m.confPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.confPath, []byte("[Interface]\nPrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEs=\nListenPort = 51820\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.store.SetHeaderKey("Zm9vYmFyYmF6cXV1eA=="); err != nil {
		t.Fatal(err)
	}
	f.outputs["awg show awg-rb0"] = "interface: awg-rb0\n  listening port: 51820\n"
	in := goodEnableInput()
	in.HeaderProtection = true
	in.Obf = Obfuscation{Jc: 4, S1: 12, S2: 12, S3: 12, S4: 12,
		CPA: "200-400", RAT: "120",
		RekeyTimeout: "5", RejectAfterTime: "180", KeepaliveTimeout: "25", MaxHandshakeAttempts: "18"}
	m.Rehydrate(context.Background(), in)

	if m.obf.CPA != "200-400" || m.obf.RAT != "120" {
		t.Fatalf("rehydrate must keep AWG3 obf on an awg3-capable kernel host, got %+v", m.obf)
	}
	if !m.headerProtection || m.headerKey != "Zm9vYmFyYmF6cXV1eA==" {
		t.Fatalf("rehydrate must restore the header key, got protection=%v key=%q", m.headerProtection, m.headerKey)
	}
}

func TestStatus_KernelConfigDirty_HeaderProtectionToggleGatedOnCapability(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	m.SetKernelSupportsAWG3(func() bool { return true })
	f.outputs["awg show awg-rb0"] = "interface: awg-rb0\n  listening port: 51820\n"
	f.outputs["iptables -t nat -S"] = "-N RBOX-AWG-NAT\n"
	desired := goodEnableInput()
	m.desired = func() EnableInput { return desired }
	if err := m.Enable(context.Background(), desired); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if m.Status(context.Background()).ConfigDirty {
		t.Fatal("freshly enabled must not be dirty")
	}
	desired.HeaderProtection = true
	desired.Obf = Obfuscation{S1: 12, S2: 12, S3: 12, S4: 12}
	if !m.Status(context.Background()).ConfigDirty {
		t.Fatal("toggling header_protection alone must flag ConfigDirty on an awg3-capable kernel host")
	}
}

func TestClientConfFor_KernelAWG3Capable_IncludesHeaderKey(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	m.SetKernelSupportsAWG3(func() bool { return true })
	f.outputs["awg show awg-rb0"] = "interface: awg-rb0\n  listening port: 51820\n"
	f.outputs["iptables -t nat -S"] = "-N RBOX-AWG-NAT\n"
	if err := m.Enable(context.Background(), kernelAwg3EnableInput()); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	m.store.Put(Peer{PublicKey: "peerpub", PrivateKey: "peerpriv", PresharedKey: "psk", Address: "10.10.0.2/32", Name: "p1"})
	conf, err := m.RenderClientConf("peerpub", "vpn.example.com")
	if err != nil {
		t.Fatalf("RenderClientConf: %v", err)
	}
	if !strings.Contains(conf, "HeaderProtectionKey = "+m.store.HeaderKey()) {
		t.Fatalf("client conf must carry the header key on an awg3-capable kernel host:\n%s", conf)
	}
	if !strings.Contains(conf, "ContentPaddingAddition = 10-20") {
		t.Fatalf("client conf must carry CPA on an awg3-capable kernel host:\n%s", conf)
	}
}
