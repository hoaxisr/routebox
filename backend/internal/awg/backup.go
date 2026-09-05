package awg

import (
	"errors"
	"fmt"
	"net/netip"
	"time"

	"routebox/backend/internal/settings"
)

// Backup is the portable AWG server state (#97): the operator's server settings
// plus everything peers.toml holds. Secrets are plaintext, exactly as on disk.
type Backup struct {
	Version    int                  `json:"version"`
	ExportedAt int64                `json:"exported_at"`
	Settings   settings.AwgSettings `json:"settings"`
	ServerKey  string               `json:"server_key"`
	HeaderKey  string               `json:"header_key,omitempty"`
	ULAPrefix  string               `json:"ula_prefix,omitempty"`
	Peers      []Peer               `json:"peers"`
}

const backupVersion = 1

// ErrServerUp is Restore's refusal while the server is enabled or an Enable is
// in flight; the API maps it to 409.
var ErrServerUp = errors.New("disable the AWG server before restoring a backup")

// Snapshot captures the current server state. Enabled/Configured are state, not
// configuration, and WANIface is bound to this host — none of them belongs on
// another box, so the copy is scrubbed. The kernel backend never persists the
// server key to peers.toml (it lives in the .conf), so the in-memory key is the
// fallback: without it the restored server would mint a new identity.
func (m *Manager) Snapshot(s settings.AwgSettings) Backup {
	s.Enabled, s.Configured, s.WANIface = false, false, ""
	key := m.store.ServerKey()
	if key == "" {
		m.mu.Lock()
		key = m.serverPriv
		m.mu.Unlock()
	}
	return Backup{
		Version: backupVersion, ExportedAt: time.Now().Unix(), Settings: s,
		ServerKey: key, HeaderKey: m.store.HeaderKey(), ULAPrefix: m.store.ULAPrefix(),
		Peers: m.store.List(),
	}
}

// Restore validates the backup and replaces the peer store wholesale. Refused
// while the server is up: the live interface/endpoint was rendered from the
// store being thrown away, and Enable is the only path that re-renders it.
// Settings are the caller's to persist (the settings package stays awg-agnostic).
func (m *Manager) Restore(b Backup) error {
	m.mu.Lock()
	busy := m.enabled || m.inFlight
	m.mu.Unlock()
	if busy {
		return ErrServerUp
	}
	if b.Version != backupVersion {
		return fmt.Errorf("unsupported backup version %d (want %d)", b.Version, backupVersion)
	}
	subnet, err := ValidateSubnet(b.Settings.Subnet)
	if err != nil {
		return fmt.Errorf("settings.subnet: %w", err)
	}
	pfx := netip.MustParsePrefix(subnet)
	if _, err := PublicFromPrivate(b.ServerKey); err != nil {
		return fmt.Errorf("server_key: %w", err)
	}
	seen := make(map[string]bool, len(b.Peers))
	for i, p := range b.Peers {
		if _, err := ValidatePublicKey(p.PublicKey); err != nil {
			return fmt.Errorf("peers[%d].public_key: %w", i, err)
		}
		if seen[p.PublicKey] {
			return fmt.Errorf("peers[%d]: duplicate public key", i)
		}
		seen[p.PublicKey] = true
		if _, err := PublicFromPrivate(p.PrivateKey); err != nil {
			return fmt.Errorf("peers[%d].private_key: %w", i, err)
		}
		if _, err := ValidatePeerName(p.Name); err != nil {
			return fmt.Errorf("peers[%d].name: %w", i, err)
		}
		addr, err := netip.ParsePrefix(p.Address)
		if err != nil || !pfx.Contains(addr.Addr()) {
			return fmt.Errorf("peers[%d].address %q is not inside subnet %s", i, p.Address, subnet)
		}
	}
	return m.store.Replace(b.ServerKey, b.HeaderKey, b.ULAPrefix, b.Peers)
}
