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

// Status must surface the installed module's version for the UI, and must
// leave it empty when no version could be read — never guess one. Module, not
// the version, is what says whether the module is installed at all.
func TestStatusKernel_SurfacesModuleVersion(t *testing.T) {
	t.Run("loaded => version reported, module ready", func(t *testing.T) {
		f := newFakeRunner()
		m := newEnableManager(t, f)
		m.SetKernelModuleInfo(func() KernelModuleInfo {
			return KernelModuleInfo{Version: "3.0.20260731-04", Detected: true, Loaded: true}
		})
		st := m.Status(context.Background())
		if st.KernelModuleVersion != "3.0.20260731-04" {
			t.Fatalf("KernelModuleVersion = %q, want %q", st.KernelModuleVersion, "3.0.20260731-04")
		}
		if st.Module != StateReady {
			t.Fatalf("Module = %q, want %q — a version read out of sysfs proves the kernel runs it", st.Module, StateReady)
		}
	})

	// modinfo answers for a .ko that cannot load at all (unsigned under Secure
	// Boot, built for another kernel). The version is worth reporting; readiness
	// is not — claiming it would hide the warning AND its install link until
	// Turn on fails.
	t.Run("on disk but not loaded => version reported, module still not-installed", func(t *testing.T) {
		f := newFakeRunner()
		m := newEnableManager(t, f)
		m.SetKernelModuleInfo(func() KernelModuleInfo {
			return KernelModuleInfo{Version: "3.0.20260731-04", Detected: true}
		})
		st := m.Status(context.Background())
		if st.KernelModuleVersion != "3.0.20260731-04" {
			t.Fatalf("KernelModuleVersion = %q, want the on-disk version", st.KernelModuleVersion)
		}
		if st.Module != StateNotInstalled {
			t.Fatalf("Module = %q, want %q — a .ko on disk is not a module in the kernel", st.Module, StateNotInstalled)
		}
	})

	t.Run("not installed => empty version and module not-installed", func(t *testing.T) {
		f := newFakeRunner()
		m := newEnableManager(t, f)
		m.SetKernelModuleInfo(func() KernelModuleInfo { return KernelModuleInfo{} })
		st := m.Status(context.Background())
		if st.KernelModuleVersion != "" {
			t.Fatalf("KernelModuleVersion = %q, want empty when the module is not installed", st.KernelModuleVersion)
		}
		if st.Module != StateNotInstalled {
			t.Fatalf("Module = %q, want %q", st.Module, StateNotInstalled)
		}
	})

	// The fail-closed read: the module IS loaded (the interface is up) but its
	// version could not be read. The UI must not call that "not installed" —
	// that is a red error over a running tunnel.
	t.Run("loaded but version unreadable => empty version, module ready", func(t *testing.T) {
		f := newFakeRunner()
		f.outputs["awg show awg-rb0"] = "interface: awg-rb0\n  listening port: 51820\n"
		m := newEnableManager(t, f)
		m.SetKernelModuleInfo(func() KernelModuleInfo { return KernelModuleInfo{} })
		st := m.Status(context.Background())
		if st.KernelModuleVersion != "" {
			t.Fatalf("KernelModuleVersion = %q, want empty — an unreadable version is not a guessable one", st.KernelModuleVersion)
		}
		if st.Module != StateReady {
			t.Fatalf("Module = %q, want %q on a live interface", st.Module, StateReady)
		}
	})

	// A .ko on disk is not a finished install: modinfo can already answer while
	// the DKMS build is still running, and it keeps answering after a load that
	// failed. Only a stale NotInstalled may be upgraded from a read version.
	for _, tc := range []struct {
		name  string
		state State
	}{
		{"installing", StateInstalling},
		{"failed", StateFailed},
	} {
		t.Run("detected version does not overwrite "+tc.name, func(t *testing.T) {
			f := newFakeRunner()
			m := newEnableManager(t, f)
			m.module.setState(tc.state)
			m.SetKernelModuleInfo(func() KernelModuleInfo {
				return KernelModuleInfo{Version: "3.1.20260812", Detected: true, Loaded: true}
			})
			if st := m.Status(context.Background()); st.Module != tc.state {
				t.Fatalf("Module = %q, want %q — a version on disk must not claim the install finished", st.Module, tc.state)
			}
		})
	}
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

// A host whose module clears 3.0 but not 3.1: Enable strips the two 3.1 flags on
// the way in, so the saved-vs-running compare has to strip them too. Otherwise a
// settings file carrying them (saved while on the singbox backend, then switched)
// pins the Apply banner on for good — #74's symptom by a different route.
func TestStatus_KernelConfigDirty_Awg31FlagsStrippedOnIncapableHost(t *testing.T) {
	f := newFakeRunner()
	m := newEnableManager(t, f)
	m.SetKernelSupportsAWG3(func() bool { return true })
	m.SetKernelSupportsAWG31(func() bool { return false })
	f.outputs["awg show awg-rb0"] = "interface: awg-rb0\n  listening port: 51820\n"
	f.outputs["iptables -t nat -S"] = "-N RBOX-AWG-NAT\n"
	desired := goodEnableInput()
	desired.Obf.RandomTrailers = true
	desired.Obf.DisableCookies = true
	m.desired = func() EnableInput { return desired }
	if err := m.Enable(context.Background(), desired); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if m.Status(context.Background()).ConfigDirty {
		t.Fatal("3.1 flags the host cannot take must not read dirty forever")
	}
}
