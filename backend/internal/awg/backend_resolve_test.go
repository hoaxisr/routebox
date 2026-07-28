package awg

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

// seedKernelConf writes a kernel-backend .conf holding priv as the interface key.
func seedKernelConf(t *testing.T, m *Manager, priv string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(m.confPath), 0700); err != nil {
		t.Fatal(err)
	}
	body := "[Interface]\nPrivateKey = " + priv + "\nListenPort = 51820\n"
	if err := os.WriteFile(m.confPath, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func testKey(t *testing.T) string {
	t.Helper()
	priv, _, err := Generate(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv
}

// Issue #43: upgrading a pre-0.23 install (kernel-only, no awg.backend setting)
// silently flipped the panel to the sing-box backend, which tore down the live
// kernel tunnel and minted a fresh server identity. The reverse must not happen
// either: an install that has since been rebuilt on sing-box keeps its stale
// kernel .conf forever (nothing deletes it), so the .conf alone cannot decide.
func TestResolveBackend(t *testing.T) {
	cases := []struct {
		name      string
		setting   string
		seedConf  bool
		serverKey bool
		want      string
	}{
		{"explicit kernel wins", "kernel", false, false, "kernel"},
		{"explicit singbox wins over a kernel conf", "singbox", true, false, "singbox"},
		{"unset, fresh install", "", false, false, "singbox"},
		{"unset, legacy kernel install", "", true, false, "kernel"},
		{"unset, singbox key beats a stale kernel conf", "", true, true, "singbox"},
		{"unset, singbox key without a conf", "", false, true, "singbox"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestManager(t, newFakeRunner())
			if tc.seedConf {
				seedKernelConf(t, m, testKey(t))
			}
			if tc.serverKey {
				if err := m.store.SetServerKey(testKey(t)); err != nil {
					t.Fatal(err)
				}
			}
			if got := m.ResolveBackend(tc.setting); got != tc.want {
				t.Fatalf("ResolveBackend(%q) = %q, want %q", tc.setting, got, tc.want)
			}
		})
	}
}

// A .conf that cannot be stat'ed for any reason other than "not there" must read
// as present: guessing "absent" is the branch that decommissions a running
// kernel tunnel.
func TestResolveBackendUnreadableConfDirStaysKernel(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: mode 0000 does not deny stat")
	}
	m := newTestManager(t, newFakeRunner())
	dir := filepath.Dir(m.confPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0700) })
	if got := m.ResolveBackend(""); got != "kernel" {
		t.Fatalf("ResolveBackend(\"\") = %q, want kernel", got)
	}
}

// A kernel->singbox switch must keep the server identity: every issued client
// .conf/QR pins the server public key, so a fresh key silently kills every peer.
func TestAdoptKernelServerKey(t *testing.T) {
	priv := testKey(t)
	cases := []struct {
		name string
		conf string
		want string
	}{
		{"no conf on disk", "", ""},
		{"interface key", "[Interface]\nPrivateKey = " + priv + "\n", priv},
		{"no private key at all", "[Interface]\nListenPort = 51820\n", ""},
		{"key belongs to a peer, not the interface", "[Interface]\nListenPort = 51820\n\n[Peer]\nPrivateKey = " + priv + "\n", ""},
		// Unvalidated adoption would persist this to peers.toml as the permanent
		// server identity AND emit it into the active config, which amnezia-box
		// then fails to load — taking down all proxying, not just AWG.
		{"malformed key", "[Interface]\nPrivateKey = not-a-key\n", ""},
		{"key of the wrong length", "[Interface]\nPrivateKey = aGVsbG8=\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestManager(t, newFakeRunner())
			if tc.conf != "" {
				if err := os.MkdirAll(filepath.Dir(m.confPath), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(m.confPath, []byte(tc.conf), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := m.adoptKernelServerKey(); got != tc.want {
				t.Fatalf("adoptKernelServerKey() = %q, want %q", got, tc.want)
			}
		})
	}
}

// The symptom of #43 is what Enable does, not what the helper returns: enabling
// the singbox backend on a host that ran the kernel one must reuse the kernel
// server key, so the endpoint the fork serves keeps the public key every issued
// client config pins.
func TestSingbox_EnableAdoptsKernelServerKey(t *testing.T) {
	m, fs, _ := newSingboxMgrDisabled(t)
	m.confPath = filepath.Join(t.TempDir(), "awg-rb0.conf")
	priv := testKey(t)
	seedKernelConf(t, m, priv)

	if err := m.Enable(context.Background(), singboxEnableInput()); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if got := m.store.ServerKey(); got != priv {
		t.Fatalf("persisted server key = %q, want the kernel key %q", got, priv)
	}
	if fs.lastSpec == nil {
		t.Fatal("Enable synced no server spec")
	}
	if fs.lastSpec.PrivateKey != priv {
		t.Fatalf("endpoint private key = %q, want the kernel key %q", fs.lastSpec.PrivateKey, priv)
	}
}

// The same path must fail safe: an unusable key in the .conf is never persisted
// or served — Enable falls through to a freshly generated one.
func TestSingbox_EnableRejectsUnusableKernelKey(t *testing.T) {
	m, fs, _ := newSingboxMgrDisabled(t)
	m.confPath = filepath.Join(t.TempDir(), "awg-rb0.conf")
	seedKernelConf(t, m, "not-a-key")

	if err := m.Enable(context.Background(), singboxEnableInput()); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	got := m.store.ServerKey()
	if got == "not-a-key" || got == "" {
		t.Fatalf("persisted server key = %q, want a freshly generated one", got)
	}
	if _, err := PublicFromPrivate(got); err != nil {
		t.Fatalf("persisted server key is unusable: %v", err)
	}
	if fs.lastSpec == nil || fs.lastSpec.PrivateKey != got {
		t.Fatalf("endpoint key does not match the persisted one: %#v", fs.lastSpec)
	}
}
