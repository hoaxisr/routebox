package awg

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"routebox/backend/internal/awg/cps"
)

// PeerSummary is the secret-free API/UI view of a peer.
type PeerSummary struct {
	Name          string `json:"name"`
	PublicKey     string `json:"public_key"`
	Address       string `json:"address"`
	LastHandshake int64  `json:"last_handshake"` // unix seconds; 0 = never handshaked
	Online        bool   `json:"online"`         // handshake within onlineWindowSec
	Rx            int64  `json:"rx"`             // cumulative bytes received (since iface up)
	Tx            int64  `json:"tx"`             // cumulative bytes sent (since iface up)
}

// onlineWindowSec: a peer counts as "online" if its last handshake is newer than
// this. WireGuard rekeys roughly every ~120s while a tunnel is active, so 180s is a
// forgiving liveness window.
const onlineWindowSec = 180

// isOnline reports whether a last-handshake unix ts is recent enough to be "online".
func isOnline(ts, now int64) bool {
	return ts > 0 && now-ts < onlineWindowSec
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
	obfPreset  string // active profile name, for client-config CPS mimicry
	wan        string

	// Enable-orchestrator state (Task 9). module ensures the kernel module;
	// sysClassNet is the /sys/class/net root for ValidateWANIface; dns/serverPriv
	// are canonical values persisted for later peer ops.
	module      *ModuleManager
	sysClassNet string
	dns         []string
	serverPriv  string

	// desired returns the operator's saved server config (from settings), used by
	// Status to compute ConfigDirty. nil -> dirty is never reported.
	desired func() EnableInput

	addMu sync.Mutex // serialises the whole add-peer critical section

	mu       sync.Mutex  // guards enabled/lastErr/phase/inFlight (distinct from addMu)
	enabled  bool        // last successful Enable left the tunnel up
	lastErr  string      // last orchestrator error (surfaced in Status)
	phase    EnablePhase // current orchestrator phase (phased Status)
	inFlight bool        // single-flight: an Enable orchestrator is running
}

// ErrSubnetExhausted is returned by AddPeer when no /32 host remains free.
var ErrSubnetExhausted = fmt.Errorf("subnet exhausted")

// Config seeds a Manager's non-runtime fields (canonical values are otherwise
// re-derived on Enable). publicHost is purely informational in Status; the
// authoritative host for client .confs comes from the live settings at render
// time, passed to RenderClientConf.
type Config struct {
	Iface      string
	Subnet     string
	ServerIP   string
	ListenPort int
	MTU        int
	DNS        []string
	WANIface   string
	PublicHost string
}

// NewManager wires a production Manager: a Runner, the on-demand ModuleManager,
// the secret Store under baseDir, and the server .conf path. baseDir holds both
// the .conf and peers.toml (e.g. "/etc/routebox/amneziawg"). PSK temp files use
// os.TempDir(); sysClassNet is the real /sys/class/net for WAN validation.
func NewManager(run Runner, baseDir string, cfg Config) *Manager {
	iface := cfg.Iface
	if iface == "" {
		iface = "awg-rb0"
	}
	return &Manager{
		run:         run,
		confPath:    filepath.Join(baseDir, iface+".conf"),
		store:       NewStore(filepath.Join(baseDir, "peers.toml")),
		iface:       iface,
		pskTmpDir:   os.TempDir(),
		subnet:      cfg.Subnet,
		serverIP:    cfg.ServerIP,
		listenPort:  cfg.ListenPort,
		mtu:         cfg.MTU,
		dns:         cfg.DNS,
		wan:         cfg.WANIface,
		publicHost:  cfg.PublicHost,
		module:      NewModuleManager(run, ""),
		sysClassNet: "/sys/class/net",
	}
}

// Store exposes the secret store so the wiring layer can Load() it at startup.
func (m *Manager) Store() *Store { return m.store }

// SetDesired wires the saved-config getter used for ConfigDirty (see Status).
func (m *Manager) SetDesired(f func() EnableInput) {
	m.mu.Lock()
	m.desired = f
	m.mu.Unlock()
}

// peerLines maps the stored peers to renderable [Peer] blocks, so a full conf render
// (Enable/Apply) keeps every existing peer in the file. Without it, Enable rendered
// `nil` and the conf lost all peers — they survived live (awg set) but vanished on the
// next awg-quick restart/reboot.
func (m *Manager) peerLines() []PeerLine {
	peers := m.store.List()
	out := make([]PeerLine, 0, len(peers))
	for _, p := range peers {
		out = append(out, PeerLine{Name: p.Name, PublicKey: p.PublicKey, PSK: p.PresharedKey, AllowedIP: p.Address})
	}
	return out
}

// parseInterfacePrivateKey extracts PrivateKey from the [Interface] section of a
// rendered .conf (stops at the first [Peer]).
func parseInterfacePrivateKey(conf string) string {
	for _, line := range strings.Split(conf, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[Peer]") {
			break
		}
		if strings.HasPrefix(line, "PrivateKey") {
			if i := strings.Index(line, "="); i >= 0 {
				return strings.TrimSpace(line[i+1:])
			}
		}
	}
	return ""
}

// Rehydrate restores in-memory render state (serverPriv/obf/subnet/…) from the
// persisted .conf + saved settings after a process restart, WITHOUT touching
// awg-quick. The Manager is otherwise "cold" on boot (serverPriv/obf are set only by
// Enable), so RenderClientConf/AddPeer 500 until a re-enable. enabled reflects the
// live interface (awg show), so the panel shows running/stopped correctly. Best-effort:
// any parse/validation problem leaves the Manager cold rather than crashing boot.
func (m *Manager) Rehydrate(ctx context.Context, in EnableInput) {
	data, err := os.ReadFile(m.confPath)
	if err != nil {
		return // no conf -> never configured / nothing to restore
	}
	priv := parseInterfacePrivateKey(string(data))
	if priv == "" {
		return
	}
	subnet, err := ValidateSubnet(in.Subnet)
	if err != nil {
		return
	}
	port, err := ValidateListenPort(in.ListenPort)
	if err != nil {
		return
	}
	serverIP, err := firstHost(subnet)
	if err != nil {
		return
	}
	obf, err := validateObf(in.Obf)
	if err != nil {
		return
	}
	mtu, _ := ValidateMTU(in.MTU) // non-critical for render; 0 -> omitted
	dns, _ := ValidateDNS(in.DNS) // RenderClientConf falls back to 1.1.1.1 if empty

	up := false
	if out, _, e := m.run.Run(ctx, "awg", "show", m.iface); e == nil && strings.Contains(out, "listening port") {
		up = true
	}

	m.mu.Lock()
	m.serverPriv, m.subnet, m.serverIP, m.listenPort, m.mtu, m.dns, m.obf, m.wan =
		priv, subnet, serverIP, port, mtu, dns, obf, in.WANIface
	m.obfPreset = in.ObfPreset
	m.enabled = up
	if up {
		m.phase = PhaseReady
	}
	m.mu.Unlock()
}

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

// ListPeers returns secret-free summaries from the store (sorted by PublicKey via
// the store's List). The PeerSummary type CANNOT serialise private/preshared keys.
func (m *Manager) ListPeers(ctx context.Context) []PeerSummary {
	hs := m.iface_Handshakes(ctx)
	xf := m.iface_Transfer(ctx)
	now := time.Now().Unix()
	out := []PeerSummary{}
	for _, p := range m.store.List() {
		ts := hs[p.PublicKey]
		x := xf[p.PublicKey]
		out = append(out, PeerSummary{
			Name: p.Name, PublicKey: p.PublicKey, Address: p.Address,
			LastHandshake: ts, Online: isOnline(ts, now), Rx: x.rx, Tx: x.tx,
		})
	}
	return out
}

// PeerConfig reports whether a stored secret exists for pub (existence check the
// handler runs BEFORE the public-host check, so a missing peer is a 404 not a 503).
func (m *Manager) PeerConfig(pub string) (name string, ok bool) {
	p, ok := m.store.Get(pub)
	if !ok {
		return "", false
	}
	return p.Name, true
}

// RenderClientConf builds the client .conf from the stored secret, the derived
// server public key, and the configured obfuscation/DNS/MTU. host is the validated
// public host; the Endpoint is assembled IPv6-safe (bare v6 host bracketed).
func (m *Manager) RenderClientConf(pub, host string) (string, error) {
	p, ok := m.store.Get(pub)
	if !ok {
		return "", fmt.Errorf("no such peer")
	}
	m.mu.Lock()
	serverPriv, dns, mtu, obf, port, preset := m.serverPriv, m.dns, m.mtu, m.obf, m.listenPort, m.obfPreset
	m.mu.Unlock()
	serverPub, err := PublicFromPrivate(serverPriv)
	if err != nil {
		return "", err
	}
	if len(dns) == 0 {
		dns = []string{"1.1.1.1"}
	}
	return BuildClient(ClientConf{
		PrivateKey: p.PrivateKey,
		Address:    p.Address,
		DNS:        dns,
		MTU:        mtu,
		Obf:        obf,
		Mimic:      cps.Mimic(preset),
		ServerPub:  serverPub,
		Endpoint:   joinHostPort(host, port),
		AllowedIPs: []string{"0.0.0.0/0"},
		Keepalive:  25,
		PSK:        p.PresharedKey,
	})
}

// joinHostPort joins a host (domain or IP) and port, bracketing IPv6 literals so
// BuildClient (which passes Endpoint verbatim) never emits a bare-v6 "host:port".
func joinHostPort(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// RemovePeer removes a peer live and from the on-disk .conf + secret store.
func (m *Manager) RemovePeer(ctx context.Context, pub string) error {
	if _, err := ValidatePublicKey(pub); err != nil {
		return err
	}
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
