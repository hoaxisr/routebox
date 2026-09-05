package awg

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"strings"
	"testing"

	"routebox/backend/internal/settings"
)

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
	peer = Peer{PublicKey: pub, PrivateKey: priv, PresharedKey: "PSK==", Address: "10.10.0.2/32", Name: "phone", ExpiresAt: 42}
	if err := m.store.SetServerKey(serverKey); err != nil {
		t.Fatal(err)
	}
	if err := m.store.SetHeaderKey("HDR=="); err != nil {
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

	b := src.Snapshot(backupSettings())
	if b.Version != 1 || b.ServerKey != serverKey || b.HeaderKey != "HDR==" || len(b.Peers) != 1 {
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
	if dst.store.ServerKey() != serverKey || dst.store.HeaderKey() != "HDR==" {
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
	b := src.Snapshot(backupSettings())

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
	if b := m.Snapshot(backupSettings()); b.ServerKey != priv {
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
	good := src.Snapshot(backupSettings())

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
	b := src.Snapshot(backupSettings())
	dst := newTestManager(t, newFakeRunner())
	dst.enabled = true
	if err := dst.Restore(b); err == nil || !strings.Contains(err.Error(), "disable") {
		t.Fatalf("err = %v, want refusal naming disable", err)
	}
}
