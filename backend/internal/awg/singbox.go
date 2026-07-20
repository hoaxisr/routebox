package awg

import (
	"context"
	"crypto/rand"
	"encoding/base64"
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

// SetSupportsAWG3 wires the awg3 binary-capability gate for the three awg3-only
// endpoint fields (cpa/rat/header_protection_key). nil (unset) = supported.
func (m *Manager) SetSupportsAWG3(fn func() bool) {
	m.mu.Lock()
	m.supports3Fn = fn
	m.mu.Unlock()
}

// renderServerSpec snapshots the live server config + non-expired store peers into
// a config.AwgServerSpec. Returns nil when the server is disabled OR the server
// key is unset. The enabled gate is what makes Disable STICK: disableSingbox
// keeps m.serverPriv (key reuse is intended), so gating on the key alone let the
// 30s sweep re-render a full spec and resurrect a disabled server (Bug C1).
func (m *Manager) renderServerSpec() *config.AwgServerSpec {
	m.mu.Lock()
	enabled := m.enabled
	priv, ip, subnet, port, mtu, obf := m.serverPriv, m.serverIP, m.subnet, m.listenPort, m.mtu, m.obf
	headerKey, s3fn := m.headerKey, m.supports3Fn
	m.mu.Unlock()
	if !enabled || priv == "" {
		return nil
	}
	supports3 := s3fn == nil || s3fn()
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
	obfMap := map[string]interface{}{
		"jc": obf.Jc, "jmin": obf.Jmin, "jmax": obf.Jmax,
		"s1": obf.S1, "s2": obf.S2, "s3": obf.S3, "s4": obf.S4,
		"h1": obf.H1, "h2": obf.H2, "h3": obf.H3, "h4": obf.H4,
	}
	// Additivity: on a pre-awg3 binary NONE of the three awg3 fields may appear,
	// or the config load breaks. Gate them all on supports3.
	if supports3 {
		if obf.CPA != "" {
			obfMap["content_padding_addition"] = obf.CPA
		}
		if obf.RAT != "" {
			obfMap["rekey_after_time"] = obf.RAT
		}
	} else {
		headerKey = ""
	}
	return &config.AwgServerSpec{
		PrivateKey:          priv,
		Address:             ip + maskSuffix(subnet),
		ListenPort:          port,
		MTU:                 mtu,
		HeaderProtectionKey: headerKey,
		Obf:                 obfMap,
		Peers:               peers,
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
	// AWG3 header protection: hard-gate on binary capability (emitting the field
	// on an old binary breaks the config load), enforce the fork's S>=8 rule, and
	// ensure a persisted header key so client exports survive restarts.
	hpk := ""
	if in.HeaderProtection {
		if m.supports3Fn != nil && !m.supports3Fn() {
			return m.enableFail("header protection requires an awg3 binary")
		}
		if err := validateHPKConstraint(obf, true); err != nil {
			return m.enableFail(err.Error())
		}
		if hpk = m.store.HeaderKey(); hpk == "" {
			raw := make([]byte, 32)
			if _, err := rand.Read(raw); err != nil {
				return m.enableFail(err.Error())
			}
			hpk = base64.StdEncoding.EncodeToString(raw)
			if err := m.store.SetHeaderKey(hpk); err != nil {
				return m.enableFail(err.Error())
			}
		}
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
	m.headerKey = hpk // "" when header protection is off
	// renderServerSpec gates on m.enabled, so it MUST be true BEFORE the sync
	// below or the spec renders nil and Enable writes nothing. On sync failure
	// enableFail rolls it back to false (so a later sweep stays a no-op).
	m.enabled = true
	m.mu.Unlock()

	if err := m.singboxSync(); err != nil {
		return m.enableFail("apply: " + err.Error()) // enableFail sets enabled=false
	}
	m.mu.Lock()
	m.lastErr, m.phase = "", PhaseReady
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
	headerKey, s3fn := m.headerKey, m.supports3Fn
	m.mu.Unlock()
	serverPub, err := PublicFromPrivate(priv)
	if err != nil {
		return nil, err
	}
	// Same awg3 gate as renderServerSpec: the export may be pasted into another
	// box running a pre-awg3 binary, so strip cpa/rat/hpk when unsupported.
	if s3fn != nil && !s3fn() {
		obf.CPA, obf.RAT, headerKey = "", "", ""
	}
	return BuildClientEndpoint(ClientEndpointSpec{
		Tag:                 "awg-" + SanitizeName(name),
		PrivateKey:          p.PrivateKey,
		Address:             p.Address,
		MTU:                 mtu,
		Obf:                 obf,
		Mimic:               cps.Mimic(preset),
		HeaderProtectionKey: headerKey,
		ServerPub:           serverPub,
		PSK:                 p.PresharedKey,
		Host:                host,
		Port:                port,
	}), nil
}

// RehydrateSingbox restores in-memory render state from saved settings + the
// persisted store server key after a process restart, WITHOUT touching the
// sing-box config. enabled comes from the PERSISTED settings.Awg.Enabled (the
// Enable/Disable handlers write it) — NOT from "a server key exists": the key is
// deliberately kept across Disable for reuse, so deriving enabled from it made
// Disable not survive a RouteBox restart (Bug C1). A missing key still forces
// disabled (nothing could be served anyway).
func (m *Manager) RehydrateSingbox(in EnableInput, enabled bool) {
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
	// Restore the header key alongside the server key so a restart keeps exporting
	// the SAME HPK — but only when header protection is desired (Enable with it off
	// sets m.headerKey="", and rehydrate must not resurrect it). Same S>=8 gate as
	// enableSingbox (Bug M1): settings persist BEFORE Enable validates, so an
	// invalid header_protection+S<8 combo can be on disk — restoring the key for
	// it would make the sweep render a spec the fork rejects at config load.
	// Dropping the HPK yields a degraded-but-loadable config instead.
	hpk := ""
	if in.HeaderProtection && validateHPKConstraint(obf, true) == nil {
		hpk = m.store.HeaderKey()
	}
	m.mu.Lock()
	m.subnet, m.serverIP, m.listenPort, m.mtu, m.obf, m.serverPriv = subnet, serverIP, port, mtu, obf, priv
	m.obfPreset = in.ObfPreset
	m.headerKey = hpk
	m.enabled = enabled && priv != ""
	if m.enabled {
		m.phase = PhaseReady
	}
	m.mu.Unlock()
}
