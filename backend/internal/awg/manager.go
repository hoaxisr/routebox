package awg

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PeerSummary is the secret-free API/UI view of a peer.
type PeerSummary struct {
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
	Address   string `json:"address"`
}

// Manager orchestrates the awg-rb0 interface: it owns the single add-peer mutex,
// the secret store, and all Runner-backed iface operations.
type Manager struct {
	run        Runner
	confPath   string
	store      *Store
	iface      string
	pskTmpDir  string
	subnet     string
	serverIP   string
	listenPort int
	mtu        int
	publicHost string
	obf        Obfuscation
	wan        string

	addMu sync.Mutex // serialises the whole add-peer critical section
}

// ErrSubnetExhausted is returned by AddPeer when no /32 host remains free.
var ErrSubnetExhausted = fmt.Errorf("subnet exhausted")

// AddPeer validates the name, allocates a /32 (from the on-disk .conf under the
// mutex), applies the peer live, appends the [Peer] block, and persists the
// secret. Rolls back the live add if persistence fails. Returns a secret-free
// summary. Subnet exhausted -> ErrSubnetExhausted.
func (m *Manager) AddPeer(ctx context.Context, rawName string) (PeerSummary, error) {
	name := SanitizeName(rawName)
	m.addMu.Lock()
	defer m.addMu.Unlock()

	used, err := m.usedFromConf()
	if err != nil {
		return PeerSummary{}, err
	}
	server, err := netip.ParseAddr(m.serverIP)
	if err != nil {
		return PeerSummary{}, err
	}
	ip, err := NextFree(m.subnet, used, server)
	if err != nil {
		if strings.Contains(err.Error(), "exhausted") {
			return PeerSummary{}, ErrSubnetExhausted
		}
		return PeerSummary{}, err
	}
	allowedIP := ip.String() + "/32"

	priv, pub, err := Generate(rand.Reader)
	if err != nil {
		return PeerSummary{}, err
	}
	psk, err := GeneratePSK(rand.Reader)
	if err != nil {
		return PeerSummary{}, err
	}

	// PSK temp file (0600), removed after the live set.
	pskFile := filepath.Join(m.pskTmpDir, strconv.FormatInt(time.Now().UnixNano(), 10)+".psk")
	if err := os.WriteFile(pskFile, []byte(psk+"\n"), 0600); err != nil {
		return PeerSummary{}, err
	}
	defer os.Remove(pskFile)

	if err := m.iface_SetPeer(ctx, pub, pskFile, allowedIP); err != nil {
		return PeerSummary{}, fmt.Errorf("awg set: %w", err)
	}

	// Append [Peer] to the .conf; rollback the live add on failure.
	line := PeerLine{Name: name, PublicKey: pub, PSK: psk, AllowedIP: allowedIP}
	if err := m.appendPeerToConf(line); err != nil {
		_ = m.iface_RemovePeer(ctx, pub)
		return PeerSummary{}, err
	}
	if err := m.store.Put(Peer{PublicKey: pub, PrivateKey: priv, PresharedKey: psk, Address: allowedIP, Name: name}); err != nil {
		_ = m.iface_RemovePeer(ctx, pub)
		_ = m.rewriteConfWithout(pub)
		return PeerSummary{}, err
	}
	return PeerSummary{Name: name, PublicKey: pub, Address: allowedIP}, nil
}

// RemovePeer removes a peer live and from the on-disk .conf + secret store.
func (m *Manager) RemovePeer(ctx context.Context, pub string) error {
	m.addMu.Lock()
	defer m.addMu.Unlock()

	if err := m.iface_RemovePeer(ctx, pub); err != nil {
		return fmt.Errorf("awg set remove: %w", err)
	}
	if err := m.rewriteConfWithout(pub); err != nil {
		return err
	}
	return m.store.Delete(pub)
}

// usedFromConf reads the assigned /32s from the on-disk .conf (source of truth).
func (m *Manager) usedFromConf() ([]netip.Addr, error) {
	data, err := os.ReadFile(m.confPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var used []netip.Addr
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "AllowedIPs = "); ok {
			if p, err := netip.ParsePrefix(strings.TrimSpace(v)); err == nil {
				used = append(used, p.Addr())
			}
		}
	}
	return used, nil
}

// appendPeerToConf appends a rendered [Peer] block to the .conf, atomically (0600).
// It reuses renderPeerBlock (the single [Peer] renderer shared with RenderServer).
func (m *Manager) appendPeerToConf(p PeerLine) error {
	data, err := os.ReadFile(m.confPath)
	if err != nil {
		return err
	}
	return m.writeConfAtomic(string(data) + renderPeerBlock(p))
}

// rewriteConfWithout re-renders the .conf with the peer whose PublicKey == pub
// removed (used on rollback). It drops the [Peer] stanza containing that key.
func (m *Manager) rewriteConfWithout(pub string) error {
	data, err := os.ReadFile(m.confPath)
	if err != nil {
		return err
	}
	blocks := strings.Split(string(data), "\n[Peer]")
	kept := blocks[:1] // the [Interface] head
	for _, blk := range blocks[1:] {
		if !strings.Contains(blk, "PublicKey = "+pub) {
			kept = append(kept, blk)
		}
	}
	return m.writeConfAtomic(strings.Join(kept, "\n[Peer]"))
}

// writeConfAtomic writes the server .conf via a 0600 temp file + rename (dir 0700).
func (m *Manager) writeConfAtomic(content string) error {
	dir := filepath.Dir(m.confPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp := m.confPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, m.confPath)
}
