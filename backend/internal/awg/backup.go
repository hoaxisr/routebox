package awg

import (
	"encoding/base64"
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

// ErrNoServerKey is Snapshot's refusal on a host that never enabled the server:
// a backup with an empty key would only be rejected on the target.
var ErrNoServerKey = errors.New("the AWG server has never been enabled here, nothing to export")

// Snapshot captures the current server state. Enabled/Configured are state, not
// configuration, and WANIface is bound to this host — none of them belongs on
// another box, so the copy is scrubbed. Backend is written RESOLVED: a legacy
// kernel install carries "" and the target would resolve that to singbox, since
// the restored store holds a server key. The kernel backend never persists the
// server key to peers.toml (it lives in the .conf), so the in-memory key is the
// fallback.
func (m *Manager) Snapshot(s settings.AwgSettings) (Backup, error) {
	s.Enabled, s.Configured, s.WANIface = false, false, ""
	s.Backend = m.ResolveBackend(s.Backend)
	key := m.store.ServerKey()
	if key == "" {
		m.mu.Lock()
		key = m.serverPriv
		m.mu.Unlock()
	}
	if key == "" {
		return Backup{}, ErrNoServerKey
	}
	return Backup{
		Version: backupVersion, ExportedAt: time.Now().Unix(), Settings: s,
		ServerKey: key, HeaderKey: m.store.HeaderKey(), ULAPrefix: m.store.ULAPrefix(),
		Peers: m.store.List(),
	}, nil
}

// Restore validates the backup and replaces the peer store wholesale. Refused
// while the server is up: the live interface/endpoint was rendered from the
// store being thrown away, and Enable is the only path that re-renders it.
// Settings are the caller's to persist (the settings package stays awg-agnostic).
//
// Validation is strict because PSK/header key/address go verbatim into
// awg-rb0.conf (a newline there is a PostUp line run by root) and into the
// sing-box config (a bad value there fails the WHOLE proxy's config load).
func (m *Manager) Restore(b Backup) error {
	m.mu.Lock()
	busy := m.enabled || m.inFlight
	m.mu.Unlock()
	if busy {
		return ErrServerUp
	}
	if err := validateBackup(b); err != nil {
		return err
	}
	if err := m.store.Replace(b.ServerKey, b.HeaderKey, b.ULAPrefix, b.Peers); err != nil {
		return err
	}
	// The kernel path keeps the server key in memory (Enable sets it, Disable
	// keeps it, Rehydrate refills it from the old .conf on boot); the restored
	// identity must win over all of that — see serverKeypair too.
	m.mu.Lock()
	m.serverPriv = b.ServerKey
	m.mu.Unlock()
	return nil
}

func validateBackup(b Backup) error {
	if b.Version != backupVersion {
		return fmt.Errorf("unsupported backup version %d (want %d)", b.Version, backupVersion)
	}
	subnet, err := ValidateSubnet(b.Settings.Subnet)
	if err != nil {
		return fmt.Errorf("settings.subnet: %w", err)
	}
	pfx := netip.MustParsePrefix(subnet)
	network := pfx.Masked().Addr()
	server := network.Next()
	broadcast := lastAddr(pfx)
	if _, err := PublicFromPrivate(b.ServerKey); err != nil {
		return fmt.Errorf("server_key: %w", err)
	}
	if err := validateOptionalKey(b.HeaderKey); err != nil {
		return fmt.Errorf("header_key: %w", err)
	}
	if b.ULAPrefix != "" {
		ula, err := netip.ParsePrefix(b.ULAPrefix)
		if err != nil || !ula.Addr().Is6() || ula.Bits() != 64 {
			return fmt.Errorf("ula_prefix %q: want an IPv6 /64", b.ULAPrefix)
		}
	}
	seenKey := make(map[string]bool, len(b.Peers))
	seenAddr := make(map[netip.Addr]bool, len(b.Peers))
	for i, p := range b.Peers {
		if _, err := ValidatePublicKey(p.PublicKey); err != nil {
			return fmt.Errorf("peers[%d].public_key: %w", i, err)
		}
		if seenKey[p.PublicKey] {
			return fmt.Errorf("peers[%d]: duplicate public key", i)
		}
		seenKey[p.PublicKey] = true
		if _, err := PublicFromPrivate(p.PrivateKey); err != nil {
			return fmt.Errorf("peers[%d].private_key: %w", i, err)
		}
		if err := validateOptionalKey(p.PresharedKey); err != nil {
			return fmt.Errorf("peers[%d].preshared_key: %w", i, err)
		}
		if _, err := ValidatePeerName(p.Name); err != nil {
			return fmt.Errorf("peers[%d].name: %w", i, err)
		}
		if p.ExpiresAt < 0 || p.CreatedAt < 0 {
			return fmt.Errorf("peers[%d]: expires_at/created_at must not be negative", i)
		}
		addr, err := netip.ParsePrefix(p.Address)
		if err != nil || !addr.Addr().Is4() || addr.Bits() != 32 {
			return fmt.Errorf("peers[%d].address %q: want an IPv4 /32", i, p.Address)
		}
		ip := addr.Addr()
		switch {
		case !pfx.Contains(ip):
			return fmt.Errorf("peers[%d].address %q is not inside subnet %s", i, p.Address, subnet)
		case ip == server:
			return fmt.Errorf("peers[%d].address %q is the server address", i, p.Address)
		case ip == network:
			return fmt.Errorf("peers[%d].address %q is the network address", i, p.Address)
		case ip == broadcast:
			return fmt.Errorf("peers[%d].address %q is the broadcast address", i, p.Address)
		case seenAddr[ip]:
			return fmt.Errorf("peers[%d]: duplicate address %q", i, p.Address)
		}
		seenAddr[ip] = true
	}
	return nil
}

// validateOptionalKey accepts "" or a 32-byte std-base64 key (PSK, header key).
func validateOptionalKey(s string) error {
	if s == "" {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(raw) != 32 {
		return errors.New("want a 32-byte base64 key")
	}
	return nil
}

// lastAddr is the highest address of an IPv4 prefix (its broadcast address).
func lastAddr(p netip.Prefix) netip.Addr {
	a := p.Masked().Addr().As4()
	for i := p.Bits(); i < 32; i++ {
		a[i/8] |= 1 << (7 - i%8)
	}
	return netip.AddrFrom4(a)
}
