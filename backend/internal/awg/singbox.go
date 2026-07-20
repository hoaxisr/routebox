package awg

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/netip"
	"strings"

	"routebox/backend/internal/awg/cps"
	"routebox/backend/internal/config"
)

// ConfigSyncer reconciles the RouteBox-managed AWG server endpoint in the active
// sing-box config. Satisfied by *config.Manager (SyncAwgEndpointActive).
type ConfigSyncer interface {
	SyncAwgEndpointActive(tag string, spec *config.AwgServerSpec) (bool, error)
}

// SetBackend selects the peer-op backend: "kernel" (awg-quick/module) or
// "singbox" (managed endpoint in the sing-box config).
func (m *Manager) SetBackend(b string) { m.mu.Lock(); m.backend = b; m.mu.Unlock() }

// backendIs is the lock-safe read counterpart of SetBackend: the op guards and
// the 30s sweep ticker read the backend concurrently with runtime switches from
// the settings handler, so a bare m.backend read is a data race under -race.
func (m *Manager) backendIs(b string) bool { m.mu.Lock(); defer m.mu.Unlock(); return m.backend == b }

// SetConfigSync wires the config-sync callback, the post-change apply (reload)
// hook, and the binary-capability gate used by the singbox backend.
func (m *Manager) SetConfigSync(s ConfigSyncer, apply func() error, supports func() bool) {
	m.mu.Lock()
	m.cfgSync, m.applyFn, m.supportsFn = s, apply, supports
	m.mu.Unlock()
}

// renderServerSpec snapshots the live server config + non-expired store peers into
// a config.AwgServerSpec. Returns nil when the server key is unset (not enabled).
func (m *Manager) renderServerSpec() *config.AwgServerSpec {
	m.mu.Lock()
	priv, ip, subnet, port, mtu, obf := m.serverPriv, m.serverIP, m.subnet, m.listenPort, m.mtu, m.obf
	m.mu.Unlock()
	if priv == "" {
		return nil
	}
	now := m.store.now()
	var peers []config.AwgServerPeer
	for _, p := range m.store.List() {
		if p.ExpiresAt != 0 && now >= p.ExpiresAt {
			continue
		}
		peers = append(peers, config.AwgServerPeer{
			PublicKey: p.PublicKey, PresharedKey: p.PresharedKey, AllowedIP: p.Address,
		})
	}
	return &config.AwgServerSpec{
		PrivateKey: priv,
		Address:    ip + maskSuffix(subnet),
		ListenPort: port,
		MTU:        mtu,
		Obf: map[string]interface{}{
			"jc": obf.Jc, "jmin": obf.Jmin, "jmax": obf.Jmax,
			"s1": obf.S1, "s2": obf.S2, "s3": obf.S3, "s4": obf.S4,
			"h1": obf.H1, "h2": obf.H2, "h3": obf.H3, "h4": obf.H4,
		},
		Peers: peers,
	}
}

// singboxSync renders the managed endpoint and applies ONLY if it changed.
func (m *Manager) singboxSync() error {
	if m.cfgSync == nil {
		return fmt.Errorf("config sync not wired")
	}
	spec := m.renderServerSpec()
	changed, err := m.cfgSync.SyncAwgEndpointActive(config.ManagedAwgServerTag, spec)
	if err != nil {
		return err
	}
	if changed && m.applyFn != nil {
		return m.applyFn()
	}
	return nil
}

// addPeerSingbox allocates from the STORE only (no .conf on the singbox backend),
// persists the secret, then syncs the managed endpoint. On sync failure the stored
// peer is deleted again (compensation) so the store never claims a peer that is
// not live. Caller holds m.addMu.
func (m *Manager) addPeerSingbox(ctx context.Context, name string) (PeerSummary, error) {
	used := m.usedFromStore()
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
	peer := Peer{PublicKey: pub, PrivateKey: priv, PresharedKey: psk, Address: allowedIP, Name: name}
	if err := m.store.Put(peer); err != nil {
		return PeerSummary{}, err
	}
	if err := m.singboxSync(); err != nil {
		_ = m.store.Delete(pub) // compensation: don't leave a peer that isn't live
		return PeerSummary{}, err
	}
	return PeerSummary{Name: name, PublicKey: pub, Address: allowedIP}, nil
}

// enableSingbox validates the operator fields (same canonicalisers as kernel),
// version-gates the binary, ensures a server key, syncs+applies. No module, no
// awg-quick, no NAT — the fork's endpoint does everything in-process.
func (m *Manager) enableSingbox(ctx context.Context, in EnableInput) error {
	if m.supportsFn != nil && !m.supportsFn() {
		return m.enableFail("binary too old: AWG server needs amnezia-box awg2.1+")
	}
	subnet, err := ValidateSubnet(in.Subnet)
	if err != nil {
		return m.enableFail("subnet: " + err.Error())
	}
	port, err := ValidateListenPort(in.ListenPort)
	if err != nil {
		return m.enableFail("listen_port: " + err.Error())
	}
	mtu, err := ValidateMTU(in.MTU)
	if err != nil {
		return m.enableFail("mtu: " + err.Error())
	}
	obf, err := validateObf(in.Obf)
	if err != nil {
		return m.enableFail("obfuscation: " + err.Error())
	}
	serverIP, err := firstHost(subnet)
	if err != nil {
		return m.enableFail(err.Error())
	}
	// Server key: reuse the persisted one, else generate + persist to peers.toml.
	priv := m.store.ServerKey()
	if priv == "" {
		if priv, _, err = Generate(rand.Reader); err != nil {
			return m.enableFail(err.Error())
		}
		if err := m.store.SetServerKey(priv); err != nil {
			return m.enableFail(err.Error())
		}
	}
	m.mu.Lock()
	m.subnet, m.serverIP, m.listenPort, m.mtu, m.obf, m.serverPriv = subnet, serverIP, port, mtu, obf, priv
	m.obfPreset = in.ObfPreset
	m.mu.Unlock()

	if err := m.singboxSync(); err != nil {
		return m.enableFail("apply: " + err.Error())
	}
	m.mu.Lock()
	m.enabled, m.lastErr, m.phase = true, "", PhaseReady
	m.mu.Unlock()
	return nil
}

// disableSingbox removes the managed endpoint and reloads.
func (m *Manager) disableSingbox(ctx context.Context) error {
	if m.cfgSync == nil {
		return fmt.Errorf("config sync not wired")
	}
	changed, err := m.cfgSync.SyncAwgEndpointActive(config.ManagedAwgServerTag, nil)
	if err != nil {
		return err
	}
	if changed && m.applyFn != nil {
		if err := m.applyFn(); err != nil {
			return err
		}
	}
	m.mu.Lock()
	m.enabled, m.phase = false, PhaseIdle
	m.mu.Unlock()
	return nil
}

// statusSingbox reports a kernel-field-free status: running = we have a server key
// (enabled) AND the process is alive. Liveness/NAT/module are not applicable.
func (m *Manager) statusSingbox(ctx context.Context) AWGStatus {
	m.mu.Lock()
	enabled, lastErr, port, phase := m.enabled, m.lastErr, m.listenPort, m.phase
	m.mu.Unlock()
	if phase == "" {
		phase = PhaseIdle
	}
	return AWGStatus{
		Backend:    "singbox",
		Enabled:    enabled,
		Phase:      phase,
		ListenPort: port,
		PublicHost: m.publicHost,
		PeerCount:  len(m.store.List()),
		Module:     StateReady, // module not applicable; report ready so UI doesn't warn
		LastError:  lastErr,
	}
}

// ClientEndpoint builds the client sing-box endpoint JSON for a stored peer:
// the paste-into-another-RouteBox export (Task 8 BuildClientEndpoint).
func (m *Manager) ClientEndpoint(pub, name, host string) (map[string]interface{}, error) {
	p, ok := m.store.Get(pub)
	if !ok {
		return nil, fmt.Errorf("no such peer")
	}
	m.mu.Lock()
	priv, obf, mtu, port, preset := m.serverPriv, m.obf, m.mtu, m.listenPort, m.obfPreset
	m.mu.Unlock()
	serverPub, err := PublicFromPrivate(priv)
	if err != nil {
		return nil, err
	}
	return BuildClientEndpoint(ClientEndpointSpec{
		Tag:        "awg-" + SanitizeName(name),
		PrivateKey: p.PrivateKey,
		Address:    p.Address,
		MTU:        mtu,
		Obf:        obf,
		Mimic:      cps.Mimic(preset),
		ServerPub:  serverPub,
		PSK:        p.PresharedKey,
		Host:       host,
		Port:       port,
	}), nil
}

// RehydrateSingbox restores in-memory render state from saved settings + the
// persisted store server key after a process restart, WITHOUT touching the
// sing-box config. enabled = a server key exists (Enable ran at some point).
func (m *Manager) RehydrateSingbox(in EnableInput) {
	subnet, err := ValidateSubnet(in.Subnet)
	if err != nil {
		return
	}
	serverIP, err := firstHost(subnet)
	if err != nil {
		return
	}
	port, _ := ValidateListenPort(in.ListenPort)
	mtu, _ := ValidateMTU(in.MTU)
	obf, err := validateObf(in.Obf)
	if err != nil {
		return
	}
	priv := m.store.ServerKey()
	m.mu.Lock()
	m.subnet, m.serverIP, m.listenPort, m.mtu, m.obf, m.serverPriv = subnet, serverIP, port, mtu, obf, priv
	m.obfPreset = in.ObfPreset
	m.enabled = priv != ""
	if m.enabled {
		m.phase = PhaseReady
	}
	m.mu.Unlock()
}
