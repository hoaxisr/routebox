package awg

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/netip"
	"os"
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

// BackendName reports the active backend ("kernel" | "singbox") under m.mu,
// mirroring Status().Backend WITHOUT the kernel path's exec storm (awg show /
// iptables). Hot-path guards (e.g. awgSingboxDraftBlocked, the .conf HPK gate)
// key on it instead of running the full Status.
func (m *Manager) BackendName() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.backend == "singbox" {
		return "singbox"
	}
	return "kernel"
}

// PrepareBackendSwitch decommissions the OUTGOING backend's runtime before a
// validated backend switch is persisted. The handler's Enabled==false guard is
// not enough: an enabled-but-inactive awg-quick unit, a manually-launched iface
// (`awg-quick up` outside the unit), or a broken `awg` tool all read as
// "disabled" while the kernel tunnel is (or will be, at next boot) alive — and
// once Status routes to the new backend, that leftover is panel-invisible and
// competes with the singbox endpoint for the UDP listen port. Idempotent; a
// same-backend call is a no-op.
func (m *Manager) PrepareBackendSwitch(ctx context.Context, target string) error {
	if m.BackendName() == target {
		return nil
	}
	switch target {
	case "singbox":
		// kernel -> singbox: stop the iface (whatever launched it) and clear the
		// unit's boot persistence.
		return m.iface_Down(ctx)
	case "kernel":
		// singbox -> kernel: drop the managed endpoint from the active config so a
		// disable that failed mid-way (or hand-edited settings) can't leave the
		// fork serving AWG alongside the kernel iface.
		return m.decommissionSingbox()
	}
	return fmt.Errorf("invalid backend %q", target)
}

// decommissionSingbox removes the managed AWG endpoint from the ACTIVE sing-box
// config (reloading only on a real change). Shared by disableSingbox, the
// singbox->kernel switch, and the boot-time residue reconcile. A nil cfgSync
// (not wired yet) is a no-op, not an error.
func (m *Manager) decommissionSingbox() error {
	m.mu.Lock()
	s, apply := m.cfgSync, m.applyFn
	m.mu.Unlock()
	if s == nil {
		return nil
	}
	changed, err := s.SyncAwgEndpointActive(config.ManagedAwgServerTag, nil)
	if err != nil {
		return err
	}
	if changed && apply != nil {
		return apply()
	}
	return nil
}

// ReconcileBackendResidue heals leftovers of the INACTIVE backend at boot. The
// switch path (PrepareBackendSwitch) protects new switches, but installs that
// switched on older builds — or crashed mid-switch — can already be in the split
// state: settings say "singbox" while awg-quick@<iface> is still enabled/running
// (the reported bug: the kernel backend kept working after the switch), or say
// "kernel" while an orphaned managed endpoint sits in the active config.
// Best-effort: failures are logged, never fatal to boot.
func (m *Manager) ReconcileBackendResidue(ctx context.Context) {
	if m.backendIs("singbox") {
		// Only touch the kernel unit/iface when RouteBox ever rendered its conf —
		// the conf file marks the iface name as ours, so an operator's unrelated
		// awg-quick setup is never torn down.
		if _, err := os.Stat(m.confPath); err != nil {
			return
		}
		unitEnabled := m.kernelUnitEnabled(ctx)
		ifacePresent := m.kernelIfacePresent(ctx)
		if !unitEnabled && !ifacePresent {
			return
		}
		log.Printf("awg: backend is singbox but the kernel runtime is still around (unit enabled=%v, iface %s present=%v) — decommissioning", unitEnabled, m.iface, ifacePresent)
		if err := m.iface_Down(ctx); err != nil {
			log.Printf("awg: decommission kernel residue: %v", err)
		}
		return
	}
	// kernel backend: drop an orphaned managed endpoint from the active config.
	if err := m.decommissionSingbox(); err != nil {
		log.Printf("awg: remove orphaned singbox endpoint: %v", err)
	}
}

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
	broker, ula := m.v6Active, m.ulaPrefix
	m.mu.Unlock()
	if !enabled || priv == "" {
		return nil
	}
	supports3 := s3fn == nil || s3fn()
	// Server v6 CIDR: ParseAddr (not Must*) so a bad stored value degrades to
	// no-v6 instead of a render panic.
	serverV6CIDR := ""
	if broker {
		if sip, err := netip.ParseAddr(ip); err == nil {
			if s6, err := MapV4ToV6(ula, sip); err == nil {
				serverV6CIDR = fmt.Sprintf("%s/%d", s6, ula.Bits())
			}
		}
	}
	now := m.store.now()
	var peers []config.AwgServerPeer
	for _, p := range m.store.List() {
		if p.ExpiresAt != 0 && now >= p.ExpiresAt {
			continue
		}
		v6 := ""
		if broker {
			if a, err := netip.ParsePrefix(p.Address); err == nil {
				if m6, err := MapV4ToV6(ula, a.Addr()); err == nil {
					v6 = m6.String() + "/128"
				}
			}
		}
		peers = append(peers, config.AwgServerPeer{
			PublicKey: p.PublicKey, PresharedKey: p.PresharedKey, AllowedIP: p.Address, AllowedIP6: v6,
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
		if obf.RekeyTimeout != "" {
			obfMap["rekey_timeout"] = obf.RekeyTimeout
		}
		if obf.RejectAfterTime != "" {
			obfMap["reject_after_time"] = obf.RejectAfterTime
		}
		if obf.KeepaliveTimeout != "" {
			obfMap["keepalive_timeout"] = obf.KeepaliveTimeout
		}
		if obf.MaxHandshakeAttempts != "" {
			obfMap["max_handshake_attempts"] = obf.MaxHandshakeAttempts
		}
	} else {
		headerKey = ""
	}
	return &config.AwgServerSpec{
		PrivateKey:          priv,
		Address:             ip + maskSuffix(subnet),
		Address6:            serverV6CIDR,
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
	// Snapshot under m.mu: enableSingbox writes serverIP/subnet under m.mu, and
	// addMu (held by our caller) does not order against it.
	m.mu.Lock()
	serverIP, subnet := m.serverIP, m.subnet
	m.mu.Unlock()
	used := m.usedFromStore()
	server, err := netip.ParseAddr(serverIP)
	if err != nil {
		return PeerSummary{}, err
	}
	ip, err := NextFree(subnet, used, server)
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
	// Snapshot the capability gates under m.mu (their setters write under m.mu;
	// a bare read here races a runtime re-wire) and invoke after unlock, exactly
	// as renderServerSpec does.
	m.mu.Lock()
	supportsFn, supports3Fn := m.supportsFn, m.supports3Fn
	m.mu.Unlock()
	if supportsFn != nil && !supportsFn() {
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
	// on an old binary breaks the config load), enforce the fork's S>=12 rule, and
	// ensure a persisted header key so client exports survive restarts.
	hpk := ""
	if in.HeaderProtection {
		if supports3Fn != nil && !supports3Fn() {
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

	// IPv6 broker: MTU floor, egress preflight, once-generated ULA prefix.
	v6Active := false
	var ula netip.Prefix
	if in.IPv6Broker {
		if mtu < 1280 {
			return m.enableFail(fmt.Sprintf("ipv6 broker requires mtu >= 1280 (got %d)", mtu))
		}
		probe := m.probeFn
		if probe == nil {
			probe = defaultEgressProbe().ok
		}
		if probe() {
			p := m.store.ULAPrefix()
			if p == "" {
				gen, err := GenerateULAPrefix()
				if err != nil {
					return m.enableFail(err.Error())
				}
				p = gen.String()
				if err := m.store.SetULAPrefix(p); err != nil {
					return m.enableFail(err.Error())
				}
			}
			ula, _ = netip.ParsePrefix(p)
			v6Active = true
		} else {
			log.Printf("awg: ipv6 broker requested but no working IPv6 egress — serving IPv4 only")
		}
	}

	m.mu.Lock()
	m.subnet, m.serverIP, m.listenPort, m.mtu, m.obf, m.serverPriv = subnet, serverIP, port, mtu, obf, priv
	m.obfPreset = in.ObfPreset
	m.headerKey = hpk // "" when header protection is off
	m.headerProtection = in.HeaderProtection
	m.ipv6Broker, m.v6Active, m.ulaPrefix = in.IPv6Broker, v6Active, ula
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
	if err := m.decommissionSingbox(); err != nil {
		return err
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
	subnet, mtu, obf, obfPreset, hp, desired := m.subnet, m.mtu, m.obf, m.obfPreset, m.headerProtection, m.desired
	m.mu.Unlock()
	if phase == "" {
		phase = PhaseIdle
	}
	// ConfigDirty mirrors the kernel Status: enabled AND the saved settings differ
	// from the running snapshot on any field that needs a re-apply — subnet, port,
	// mtu, obf (CPA/RAT ride in the struct compare), the obf preset (client-export
	// mimicry), and the header-protection toggle (a shared secret flip). WAN/DNS
	// are not applicable on singbox.
	configDirty := false
	if enabled && desired != nil {
		d := desired()
		configDirty = d.Subnet != subnet || d.ListenPort != port || d.MTU != mtu ||
			d.Obf != obf || d.ObfPreset != obfPreset || d.HeaderProtection != hp
	}
	return AWGStatus{
		Backend:     "singbox",
		Enabled:     enabled,
		Phase:       phase,
		ListenPort:  port,
		PublicHost:  m.publicHost,
		PeerCount:   len(m.store.List()),
		Module:      StateReady, // module not applicable; report ready so UI doesn't warn
		ConfigDirty: configDirty,
		LastError:   lastErr,
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
		obf.RekeyTimeout, obf.RejectAfterTime, obf.KeepaliveTimeout, obf.MaxHandshakeAttempts = "", "", "", ""
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
	// sets m.headerKey="", and rehydrate must not resurrect it). Same S>=12 gate as
	// enableSingbox (Bug M1): settings persist BEFORE Enable validates, so an
	// invalid header_protection+S<12 combo can be on disk — restoring the key for
	// it would make the sweep render a spec the fork rejects at config load.
	// Dropping the HPK yields a degraded-but-loadable config instead.
	hpk := ""
	if in.HeaderProtection {
		if err := validateHPKConstraint(obf, true); err == nil {
			hpk = m.store.HeaderKey()
		} else {
			log.Printf("awg: rehydrate: header protection is enabled in settings but %v — dropping the header key (degraded, loadable config)", err)
		}
	}
	m.mu.Lock()
	m.subnet, m.serverIP, m.listenPort, m.mtu, m.obf, m.serverPriv = subnet, serverIP, port, mtu, obf, priv
	m.obfPreset = in.ObfPreset
	m.headerKey = hpk
	// Snapshot the toggle from the SAME saved settings desired() reads, so a
	// restart does not fabricate a ConfigDirty (I1).
	m.headerProtection = in.HeaderProtection
	m.enabled = enabled && priv != ""
	if m.enabled {
		m.phase = PhaseReady
	}
	m.mu.Unlock()
}
