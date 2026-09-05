package awg

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"routebox/backend/internal/settings"
)

// testHeaderKey is a 32-byte std-base64 value, as an AWG3 header key must be.
const testHeaderKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

func backupSettings() settings.AwgSettings {
	return settings.AwgSettings{
		Enabled: true, Configured: true, WANIface: "ens3", Interface: "awg-rb0",
		Subnet: "10.10.0.0/24", ListenPort: 51820, MTU: 1420, DNS: []string{"1.1.1.1"},
		ObfPreset: "web", Obf: settings.AwgObf{Jc: 4, H1: "5-10"}, ServerHost: "vpn.example.com",
	}
}

func seedBackupStore(t *testing.T, m *Manager) (serverKey string, peer Peer) {
	t.Helper()
	serverKey, _, _ = Generate(rand.Reader)
	priv, pub, _ := Generate(rand.Reader)
	psk, _ := GeneratePSK(rand.Reader)
	peer = Peer{PublicKey: pub, PrivateKey: priv, PresharedKey: psk, Address: "10.10.0.2/32", Name: "phone", ExpiresAt: 42}
	if err := m.store.SetServerKey(serverKey); err != nil {
		t.Fatal(err)
	}
	if err := m.store.SetHeaderKey(testHeaderKey); err != nil {
		t.Fatal(err)
	}
	if err := m.store.Put(peer); err != nil {
		t.Fatal(err)
	}
	return serverKey, peer
}

// A backup taken on one box and restored on a fresh one must reproduce the
// peer store exactly (same server identity, same peer secrets) and carry the
// server settings minus the host-bound/state fields.
func TestBackupRoundTrip(t *testing.T) {
	src := newTestManager(t, newFakeRunner())
	serverKey, peer := seedBackupStore(t, src)

	b, _ := src.Snapshot(backupSettings())
	if b.Version != 1 || b.ServerKey != serverKey || b.HeaderKey != testHeaderKey || len(b.Peers) != 1 {
		t.Fatalf("snapshot = %+v", b)
	}
	if b.Settings.Enabled || b.Settings.Configured || b.Settings.WANIface != "" {
		t.Fatalf("state/host-bound fields must be scrubbed: %+v", b.Settings)
	}
	if b.Settings.Subnet != "10.10.0.0/24" || b.Settings.Obf.H1 != "5-10" || b.Settings.ServerHost != "vpn.example.com" {
		t.Fatalf("server settings must survive: %+v", b.Settings)
	}

	// Through JSON, as the file on disk would be.
	raw, _ := json.Marshal(b)
	var back Backup
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.Peers[0].PrivateKey != peer.PrivateKey || back.Peers[0].ExpiresAt != 42 {
		t.Fatalf("peer secrets must survive JSON: %+v", back.Peers[0])
	}

	dst := newTestManager(t, newFakeRunner())
	if err := dst.Restore(back); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if dst.store.ServerKey() != serverKey || dst.store.HeaderKey() != testHeaderKey {
		t.Fatalf("server identity not restored: key=%q hdr=%q", dst.store.ServerKey(), dst.store.HeaderKey())
	}
	got, ok := dst.store.Get(peer.PublicKey)
	if !ok || got.PrivateKey != peer.PrivateKey || got.Address != peer.Address || got.Name != "phone" || got.ExpiresAt != 42 {
		t.Fatalf("peer not restored: %+v ok=%v", got, ok)
	}
	// And it is on disk, not just in memory.
	reload := NewStore(dst.store.GetPath())
	if err := reload.Load(); err != nil {
		t.Fatal(err)
	}
	if reload.ServerKey() != serverKey || len(reload.List()) != 1 {
		t.Fatal("restore must persist to peers.toml")
	}
}

// Restore replaces the store wholesale: a peer that exists only on the target
// must not survive, or the roster would show clients the backup never knew.
func TestRestoreReplacesExistingPeers(t *testing.T) {
	src := newTestManager(t, newFakeRunner())
	seedBackupStore(t, src)
	b, _ := src.Snapshot(backupSettings())

	dst := newTestManager(t, newFakeRunner())
	_, stray, _ := Generate(rand.Reader)
	dst.store.Put(Peer{PublicKey: stray, PrivateKey: "x", Address: "10.10.0.9/32", Name: "stray"})
	if err := dst.Restore(b); err != nil {
		t.Fatal(err)
	}
	if _, ok := dst.store.Get(stray); ok {
		t.Fatal("stray peer must be gone after restore")
	}
}

// The kernel backend keeps the server key in the .conf and in memory, never in
// peers.toml — the snapshot must still carry it, or the restored server mints
// a new identity and every client .conf dies.
func TestSnapshotExportsKernelServerKey(t *testing.T) {
	m := newTestManager(t, newFakeRunner())
	priv, _, _ := Generate(rand.Reader)
	m.serverPriv = priv
	if b, _ := m.Snapshot(backupSettings()); b.ServerKey != priv {
		t.Fatalf("server_key = %q, want the in-memory kernel key", b.ServerKey)
	}
}

// The other half of the same bug: after a restore the kernel Enable must pick
// the key up from peers.toml instead of generating one.
func TestKernelServerKeypairFallsBackToStore(t *testing.T) {
	m := newTestManager(t, newFakeRunner())
	priv, pub, _ := Generate(rand.Reader)
	m.store.SetServerKey(priv)
	gotPriv, gotPub, err := m.serverKeypair(context.Background())
	if err != nil || gotPriv != priv || gotPub != pub {
		t.Fatalf("serverKeypair = %q/%q/%v, want restored key", gotPriv, gotPub, err)
	}
}

func TestRestoreValidation(t *testing.T) {
	src := newTestManager(t, newFakeRunner())
	seedBackupStore(t, src)
	good, _ := src.Snapshot(backupSettings())

	cases := []struct {
		name   string
		mutate func(b *Backup)
		want   string
	}{
		{"version", func(b *Backup) { b.Version = 2 }, "version"},
		{"bad subnet", func(b *Backup) { b.Settings.Subnet = "nope" }, "subnet"},
		{"bad server key", func(b *Backup) { b.ServerKey = "not-a-key" }, "server_key"},
		{"peer outside subnet", func(b *Backup) { b.Peers[0].Address = "192.168.1.2/32" }, "subnet"},
		{"peer bad private key", func(b *Backup) { b.Peers[0].PrivateKey = "zz" }, "private_key"},
		{"duplicate peer", func(b *Backup) { b.Peers = append(b.Peers, b.Peers[0]) }, "duplicate"},
		{"empty name", func(b *Backup) { b.Peers[0].Name = "" }, "name"},
		// PSK and header key are written verbatim into awg-rb0.conf: a newline
		// there is a new [Interface]/PostUp line run by root.
		{"psk injection", func(b *Backup) { b.Peers[0].PresharedKey = "x\n[Interface]\nPostUp = id" }, "preshared_key"},
		{"bad header key", func(b *Backup) { b.HeaderKey = "nope\nPostUp = id" }, "header_key"},
		{"peer mask not /32", func(b *Backup) { b.Peers[0].Address = "10.10.0.5/24" }, "/32"},
		{"peer is the server", func(b *Backup) { b.Peers[0].Address = "10.10.0.1/32" }, "server"},
		{"peer is the network", func(b *Backup) { b.Peers[0].Address = "10.10.0.0/32" }, "network"},
		{"peer is broadcast", func(b *Backup) { b.Peers[0].Address = "10.10.0.255/32" }, "broadcast"},
		{"duplicate address", func(b *Backup) {
			_, pub, _ := Generate(rand.Reader)
			p := b.Peers[0]
			p.PublicKey = pub
			b.Peers = append(b.Peers, p)
		}, "address"},
		{"negative expiry", func(b *Backup) { b.Peers[0].ExpiresAt = -1 }, "expires_at"},
		{"v4 ula", func(b *Backup) { b.ULAPrefix = "10.0.0.0/8" }, "ula_prefix"},
		{"ula not /64", func(b *Backup) { b.ULAPrefix = "fd00::/48" }, "ula_prefix"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(good)
			var b Backup
			json.Unmarshal(raw, &b) // deep copy
			tc.mutate(&b)
			dst := newTestManager(t, newFakeRunner())
			err := dst.Restore(b)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want mention of %q", err, tc.want)
			}
			if dst.store.ServerKey() != "" || len(dst.store.List()) != 0 {
				t.Fatal("a rejected restore must not touch the store")
			}
		})
	}
}

func TestRestoreRefusesWhileEnabled(t *testing.T) {
	src := newTestManager(t, newFakeRunner())
	seedBackupStore(t, src)
	b, _ := src.Snapshot(backupSettings())
	dst := newTestManager(t, newFakeRunner())
	dst.enabled = true
	if err := dst.Restore(b); err == nil || !strings.Contains(err.Error(), "disable") {
		t.Fatalf("err = %v, want refusal naming disable", err)
	}
}

// A kernel host that ran AWG before keeps the OLD key in memory (Enable sets
// it, Disable does not clear it, Rehydrate refills it from the old .conf on
// every boot). The restored key must win, or the restored clients are dead.
func TestRestoreOverridesInMemoryKernelKey(t *testing.T) {
	src := newTestManager(t, newFakeRunner())
	serverKey, _ := seedBackupStore(t, src)
	b, _ := src.Snapshot(backupSettings())

	dst := newTestManager(t, newFakeRunner())
	old, _, _ := Generate(rand.Reader)
	dst.serverPriv = old
	if err := dst.Restore(b); err != nil {
		t.Fatal(err)
	}
	// Simulate a RouteBox restart between Restore and Enable: Rehydrate would
	// put the old .conf key back into memory.
	dst.serverPriv = old
	got, _, err := dst.serverKeypair(context.Background())
	if err != nil || got != serverKey {
		t.Fatalf("serverKeypair = %q, want the restored key %q", got, serverKey)
	}
}

// A legacy kernel install has backend "" in settings; on the target
// ResolveBackend("") would pick singbox because the restored store now holds
// a server key. The snapshot must carry the backend the source actually runs.
func TestSnapshotResolvesEmptyBackend(t *testing.T) {
	m := newTestManager(t, newFakeRunner())
	os.MkdirAll(filepath.Dir(m.confPath), 0700)
	os.WriteFile(m.confPath, []byte("[Interface]\nPrivateKey = x\n"), 0600)
	m.serverPriv, _, _ = Generate(rand.Reader)
	s := backupSettings()
	s.Backend = ""
	if b, _ := m.Snapshot(s); b.Settings.Backend != "kernel" {
		t.Fatalf("backend = %q, want kernel (resolved from the .conf)", b.Settings.Backend)
	}
}

// The file is kept by operators for years: pin the wire names, not just a
// Go-to-Go round trip.
func TestBackupWireFormat(t *testing.T) {
	src := newTestManager(t, newFakeRunner())
	seedBackupStore(t, src)
	snap, _ := src.Snapshot(backupSettings())
	raw, _ := json.Marshal(snap)
	var m map[string]any
	json.Unmarshal(raw, &m)
	for _, k := range []string{"version", "exported_at", "settings", "server_key", "header_key", "peers"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing top-level key %q in %s", k, raw)
		}
	}
	peer := m["peers"].([]any)[0].(map[string]any)
	for _, k := range []string{"public_key", "private_key", "preshared_key", "address", "name", "created_at", "expires_at"} {
		if _, ok := peer[k]; !ok {
			t.Fatalf("missing peer key %q in %s", k, raw)
		}
	}
	if _, ok := m["settings"].(map[string]any)["subnet"]; !ok {
		t.Fatalf("settings must use the toml/json names: %s", raw)
	}
}

func TestStoreReplaceStampsCreatedAt(t *testing.T) {
	s := newTestStore(t)
	if err := s.Replace("k", "", "", []Peer{{PublicKey: "P", Address: "10.10.0.2/32", Name: "a"}}); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.Get("P"); p.CreatedAt != 1700000000 {
		t.Fatalf("created_at = %d, want the store clock", p.CreatedAt)
	}
}

func TestSnapshotRefusesWithoutServerKey(t *testing.T) {
	m := newTestManager(t, newFakeRunner())
	if _, err := m.Snapshot(backupSettings()); !errors.Is(err, ErrNoServerKey) {
		t.Fatalf("err = %v, want ErrNoServerKey", err)
	}
}
