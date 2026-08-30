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
	"time"
)

// enableTimeout bounds a whole Enable run once its context is detached from the
// caller's. Generous — it has to cover moduleInstallTimeout plus bring-up — but
// finite, so a wedged apt or a hung awg-quick cannot leave the manager in-flight
// forever.
const enableTimeout = 40 * time.Minute

// EnablePhase is the orchestrator's current step, surfaced via Status so the UI
// can show progress while the [ensure→keygen→render→up→health-gate] tail runs.
type EnablePhase string

const (
	PhaseIdle       EnablePhase = "idle"
	PhaseValidating EnablePhase = "validating"
	PhaseInstalling EnablePhase = "installing"
	PhaseRendering  EnablePhase = "rendering"
	PhaseStarting   EnablePhase = "starting"
	PhaseHealth     EnablePhase = "health-check"
	PhaseReady      EnablePhase = "ready"
	PhaseFailed     EnablePhase = "failed"
)

// AWGStatus is the aggregate status surfaced at GET /api/awg/status.
type AWGStatus struct {
	Backend     string      `json:"backend"` // "kernel" | "singbox"
	Module      State       `json:"module"`
	Enabled     bool        `json:"enabled"`
	Phase       EnablePhase `json:"phase"`
	IfaceUp     bool        `json:"iface_up"`
	ListenPort  int         `json:"listen_port"`
	PublicHost  string      `json:"public_host"`
	PeerCount   int         `json:"peer_count"`
	Online      int         `json:"online"` // peers with a handshake within onlineWindowSec
	Rx          int64       `json:"rx"`     // server total bytes received (sum of peers, since iface up)
	Tx          int64       `json:"tx"`     // server total bytes sent (sum of peers, since iface up)
	WANIface    string      `json:"wan_iface"`
	NATOrphan   bool        `json:"nat_orphan"`
	ConfigDirty bool        `json:"config_dirty"` // enabled & saved settings differ from running -> needs Apply
	IPv6Active  bool        `json:"ipv6_active"`  // broker desired AND egress preflight passed
	// KernelUnavailable names why this system cannot run the kernel backend, or
	// is empty when it can. The panel offers a backend choice, and on a system
	// without the tools or CAP_NET_ADMIN every choice of kernel ends in the same
	// refusal at save time — so the picker refuses it up front instead.
	KernelUnavailable string `json:"kernel_unavailable,omitempty"`
	// KernelAWG3Available reports whether the kernel backend's host has a
	// confirmed awg3-capable module + awg-quick/tools pairing (always false on
	// the singbox backend, which has its own supports3Fn gate).
	KernelAWG3Available bool `json:"kernel_awg3_available,omitempty"`
	// KernelAWG31Available is the same for AWG 3.1, which added the two device
	// flags. Separate because a 3.0 pairing clears the field above and still
	// ignores those flags without an error.
	KernelAWG31Available bool   `json:"kernel_awg31_available,omitempty"`
	LastError            string `json:"last_error,omitempty"`
}

// EnableInput is the RAW operator submission; Enable canonicalises every field.
type EnableInput struct {
	Subnet     string      `json:"subnet"`
	ListenPort int         `json:"listen_port"`
	MTU        int         `json:"mtu"`
	DNS        []string    `json:"dns"`
	WANIface   string      `json:"wan_iface"` // optional override; "" -> (*Manager).detectWAN
	Obf        Obfuscation `json:"obf"`
	ObfPreset  string      `json:"obf_preset"`
	// HeaderProtection toggles the AWG3 header-protection key (singbox backend,
	// awg3+ binary only). Obf carries the awg3 CPA/RAT strings.
	HeaderProtection bool `json:"header_protection"`
	IPv6Broker       bool `json:"ipv6_broker"`
	// ClientKeepalive is export-only: the PersistentKeepalive written into client
	// .conf/QR/vpn:// exports and the sing-box client endpoint. "N" seconds, or an
	// AWG 3.0 "lo-hi" range the peer redraws on every timer arm. "" => 25. It never
	// touches the server device, so it is deliberately absent from ConfigDirty.
	ClientKeepalive string `json:"client_keepalive"`
}

// beginEnable claims the single-flight slot. Returns false if an orchestrator is
// already running (the trigger must refuse rather than double-run). The HTTP
// handler (Task 10) launches Enable in a goroutine and returns promptly; this
// guard makes a second trigger a no-op-with-busy-error instead of a double-run.
func (m *Manager) beginEnable() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inFlight {
		return false
	}
	m.inFlight = true
	return true
}

// endEnable releases the single-flight slot.
func (m *Manager) endEnable() {
	m.mu.Lock()
	m.inFlight = false
	m.mu.Unlock()
}

// Busy reports whether an Enable orchestrator is currently in flight or the
// manager sits in a transitional (non-terminal) phase. The backend-switch guard
// keys on it: switching awg.backend mid-enable would finish the orchestrator on
// the OLD branch (kernel iface + NAT up) while Status routes to the new one — a
// live, panel-invisible interface.
func (m *Manager) Busy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inFlight {
		return true
	}
	switch m.phase {
	case PhaseValidating, PhaseInstalling, PhaseRendering, PhaseStarting, PhaseHealth:
		return true
	}
	return false
}

// setPhase records the current orchestrator phase for a phased Status.
func (m *Manager) setPhase(p EnablePhase) {
	m.mu.Lock()
	m.phase = p
	m.mu.Unlock()
}

// Enable: validate -> install -> keygen -> render(canonical) -> up -> health gate
// -> re-assert 0600. Any failure -> full teardown (no orphan NAT). The .conf is
// rendered ONLY from canonical (validated) values.
//
// Enable is single-flight (concurrent triggers are refused) and tracks a phase so
// Status() can report progress. The HTTP handler calls it synchronously and the
// panel polls Status() alongside, which is what makes the caller's cancellation
// dangerous: on the kernel backend this runs for minutes (apt + a DKMS build),
// and a browser tab that gave up, or a reverse proxy that timed out, would
// otherwise cancel the request's context in the middle of the orchestrator.
//
// So the context is detached HERE, once, for the whole run — not around the
// install alone. Cancelling after the install completes is just as bad: keygen,
// iface_Up and the health gate would fail instantly with "context canceled" over
// a module that installed fine, and a cancel landing after iface_Up would take
// the teardown commands down with it and leave exactly the orphan NAT the rest
// of this file exists to prevent. A ceiling still applies, so a wedged run
// cannot hold the single-flight claim for the life of the process.
func (m *Manager) Enable(ctx context.Context, in EnableInput) error {
	if !m.beginEnable() {
		return fmt.Errorf("enable already in progress")
	}
	defer m.endEnable()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), enableTimeout)
	defer cancel()
	m.setPhase(PhaseValidating)

	if m.backendIs("singbox") {
		return m.enableSingbox(ctx, in)
	}

	// ---- Validation (canonicalise EVERY operator field before any render/run) ----
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
	dns, err := ValidateDNS(in.DNS)
	if err != nil {
		return m.enableFail("dns: " + err.Error())
	}
	// Obfuscation is also operator-controlled and lands in the root-shell .conf —
	// canonicalise it BEFORE anything is rendered, building obf from the returns.
	obf, err := validateObf(in.Obf)
	if err != nil {
		return m.enableFail("obfuscation: " + err.Error())
	}
	wan := in.WANIface
	if wan == "" {
		if wan, err = m.detectWAN(ctx); err != nil {
			return m.enableFail("wan detect: " + err.Error())
		}
	}
	if wan, err = ValidateWANIface(wan, m.sysClassNet); err != nil {
		return m.enableFail("wan_iface: " + err.Error())
	}

	// ---- Orchestrator tail: install -> keygen -> render -> up -> health gate ----
	m.setPhase(PhaseInstalling)
	if m.module != nil {
		if err := m.module.Ensure(ctx); err != nil {
			return m.enableFail(err.Error())
		}
	}

	priv, _, err := m.serverKeypair(ctx)
	if err != nil {
		return m.enableFail(err.Error())
	}
	serverIP, err := firstHost(subnet)
	if err != nil {
		return m.enableFail(err.Error())
	}

	// Build ServerConf from CANONICAL values ONLY (Task 5 RenderServer). Every
	// PostUp-bound field comes from a Validate* return: subnet<-ValidateSubnet,
	// wan<-ValidateWANIface, port<-ValidateListenPort, mtu<-ValidateMTU,
	// obf<-validateObf. Raw in.* is never threaded into the conf.
	m.setPhase(PhaseRendering)
	// awg3 (content padding / rekey / header protection) only renders on the
	// kernel path when the host's module + awg-quick/tools have both confirmed
	// awg3 capability (KernelSupportsAWG3) — a pre-awg3 awg-quick hard-fails on
	// unknown [Interface] keys, so an unsupported host gets the same silent
	// strip this path always used.
	kAwg3 := m.kernelSupports3Fn != nil && m.kernelSupports3Fn()
	hpk := ""
	if in.HeaderProtection {
		if !kAwg3 {
			return m.enableFail("header protection requires an awg3-capable kernel module + tools (v3.x)")
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
	if kAwg3 && !(m.kernelSupports31Fn != nil && m.kernelSupports31Fn()) {
		obf.stripAwg31()
	}
	if !kAwg3 {
		obf.stripAwg3()
	}
	sc := ServerConf{
		PrivateKey: priv, Address: serverIP + maskSuffix(subnet), ListenPort: port, MTU: mtu,
		Subnet: subnet, WAN: wan, Iface: m.iface, Obf: obf, HeaderProtectionKey: hpk,
	}
	// The full rewrite and the commit of what it rendered from go under addMu,
	// together. AddPeer holds that lock around a read-modify-write of the SAME
	// file (allocate a /32 from the conf, `awg set`, rewrite without the key,
	// append the block) and allocates from m.subnet/m.serverIP, so an unlocked
	// Enable could drop a peer that AddPeer had already put live, or hand AddPeer
	// the old subnet for a conf that is about to describe a different one.
	// Committing inside the same critical section closes the second window: the
	// file and the parameters it was rendered from become visible as one step.
	//
	// Lock order is addMu -> m.mu, the same as everywhere else in the package
	// (AddPeer -> addPeerSingbox, SweepExpired's singbox branch); nothing under
	// m.mu reaches back for addMu.
	if err := func() error {
		m.addMu.Lock()
		defer m.addMu.Unlock()
		if err := m.writeConfAtomic(RenderServer(sc, m.peerLines())); err != nil {
			return err
		}
		// Persist canonical values for later peer ops.
		m.mu.Lock()
		defer m.mu.Unlock()
		m.subnet, m.serverIP, m.listenPort, m.mtu, m.wan, m.dns, m.serverPriv, m.obf =
			subnet, serverIP, port, mtu, wan, dns, priv, obf
		m.obfPreset = in.ObfPreset
		m.headerKey, m.headerProtection = hpk, in.HeaderProtection
		return nil
	}(); err != nil {
		return m.enableFail(err.Error())
	}

	m.setPhase(PhaseStarting)
	if err := m.iface_Up(ctx); err != nil {
		return m.teardownFail(ctx, "iface up: "+err.Error())
	}
	m.setPhase(PhaseHealth)
	if err := m.healthGate(ctx); err != nil {
		return m.teardownFail(ctx, err.Error())
	}
	_ = os.Chmod(m.confPath, 0600) // awg-quick may have touched it

	m.mu.Lock()
	m.enabled, m.lastErr, m.phase = true, "", PhaseReady
	m.mu.Unlock()

	// Sweep the live interface (kernel backend only — the singbox backend has no
	// kernel iface to compare against): peers left behind by a crash or by manual
	// `awg set` edits. "Ours" is conf ∪ store, so suspended peers are never touched.
	// Best-effort by design: the tunnel is already up and serving, so neither a
	// failed `awg show` nor a failed removal may fail Enable — but neither is
	// swallowed either.
	m.sweepForeignLivePeers(ctx)
	return nil
}

// sweepForeignLivePeers is the Enable-tail entry point: it takes addMu and does
// its own listing. SweepExpired already holds the lock and has a listing, so it
// calls sweepForeignLivePeersLocked directly — taking addMu twice would deadlock.
//
// addMu is taken BEFORE the live listing, not after: AddPeer admits the peer to
// the interface and only then does store.Put, so a listing from inside that
// window names a live peer that no store knows yet — which reads as foreign and
// gets stripped off, leaving a client that the panel reports as fine but that
// can never connect until the next Enable/renew.
func (m *Manager) sweepForeignLivePeers(ctx context.Context) {
	m.addMu.Lock()
	defer m.addMu.Unlock()

	live, err := m.iface_ShowPeers(ctx)
	if err != nil {
		log.Printf("awg: could not list live peers of %s for reconcile: %v", m.iface, err)
		return
	}
	m.sweepForeignLivePeersLocked(ctx, live)
}

// sweepForeignLivePeersLocked removes from the live interface every peer that is
// neither in the rendered conf nor in the secret store. Failures are logged, not
// escalated: this is a hygiene step, not a correctness gate for its callers.
//
// Caller holds addMu AND obtained live under it (see sweepForeignLivePeers for
// why the order matters). Lock order is addMu -> store.mu, the same as AddPeer's
// (admit -> store.Put); nothing under this lock reaches back for addMu or m.mu.
func (m *Manager) sweepForeignLivePeersLocked(ctx context.Context, live []string) {
	for _, pub := range m.store.Reconcile(live) {
		if err := m.iface_RemovePeer(ctx, pub); err != nil {
			log.Printf("awg: could not remove foreign peer %s from %s: %v", pub, m.iface, err)
		}
	}
}

// healthGate asserts the iface is up+listening and the RBOX-AWG-* chains exist.
func (m *Manager) healthGate(ctx context.Context) error {
	out, _, err := m.run.Run(ctx, "awg", "show", m.iface)
	if err != nil || !strings.Contains(out, "listening port") {
		return fmt.Errorf("interface %s did not come up", m.iface)
	}
	rules, _, err := m.run.Run(ctx, "iptables", "-t", "nat", "-S")
	if err != nil || !strings.Contains(rules, "RBOX-AWG-NAT") {
		return fmt.Errorf("NAT chains not installed")
	}
	return nil
}

// teardownFail tears the interface down (PostDown reverts NAT) and records failure.
func (m *Manager) teardownFail(ctx context.Context, msg string) error {
	_ = m.iface_Down(ctx) // runs PostDown: flushes RBOX-AWG-* chains
	m.mu.Lock()
	m.enabled, m.lastErr, m.phase = false, msg, PhaseFailed
	m.mu.Unlock()
	return fmt.Errorf("%s", msg)
}

// enableFail records a failed Enable. It writes lastErr/phase — the REPORT —
// and deliberately leaves m.enabled alone: whether a server is being served is
// decided by what reached the config/interface, not by the outcome of the last
// attempt to change it. A failed re-configure of a running server used to clear
// the flag here, which handed the 30s sweep an instruction to tear the live
// endpoint down (renderServerSpec gates on m.enabled). Callers that DID change
// what is served say so themselves: teardownFail after bringing the interface
// down, restoreRenderStateLocked when putting a pre-Enable snapshot back.
func (m *Manager) enableFail(msg string) error {
	m.mu.Lock()
	m.lastErr, m.phase = msg, PhaseFailed
	m.mu.Unlock()
	return fmt.Errorf("%s", msg)
}

// enableFailCause is enableFail for a failure that already has an error behind
// it: the cause is WRAPPED, not flattened into a string, so callers can still
// classify it (a read-only config must reach the API as a conflict, not as an
// opaque enable failure). Same hands-off treatment of m.enabled.
func (m *Manager) enableFailCause(prefix string, cause error) error {
	msg := prefix + ": " + cause.Error()
	m.mu.Lock()
	m.lastErr, m.phase = msg, PhaseFailed
	m.mu.Unlock()
	return fmt.Errorf("%s: %w", prefix, cause)
}

// Disable stops the interface (PostDown reverts NAT).
func (m *Manager) Disable(ctx context.Context) error {
	if m.backendIs("singbox") {
		return m.disableSingbox(ctx)
	}
	if err := m.iface_Down(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	m.enabled, m.phase = false, PhaseIdle
	m.mu.Unlock()
	return nil
}

// Status aggregates module + interface + NAT-orphan state. nat_orphan = RBOX-AWG-*
// rules present while the interface is down.
func (m *Manager) Status(ctx context.Context) AWGStatus {
	if m.backendIs("singbox") {
		return m.statusSingbox(ctx)
	}
	m.mu.Lock()
	enabled, lastErr, port, phase, wan := m.enabled, m.lastErr, m.listenPort, m.phase, m.wan
	subnet, mtu, obf, desired, obfPreset := m.subnet, m.mtu, m.obf, m.desired, m.obfPreset
	hp, kernelSupports3Fn := m.headerProtection, m.kernelSupports3Fn
	kernelSupports31Fn := m.kernelSupports31Fn
	m.mu.Unlock()
	if phase == "" {
		phase = PhaseIdle
	}
	kAwg3 := kernelSupports3Fn != nil && kernelSupports3Fn()
	// ConfigDirty = enabled and the saved settings differ from what is running, on
	// any field that needs an interface restart (subnet/port/mtu/wan/obf). DNS is
	// client-only (regenerated at config download), so it never marks dirty.
	configDirty := false
	if enabled && desired != nil {
		d := desired()
		dObf, runObf := d.Obf, obf
		if !kAwg3 {
			// CPA/RAT + device-timers never render into the awg-quick conf on an
			// awg3-incapable host — a difference there must NOT flag dirty here, or
			// the Apply banner becomes permanent (a kernel restart is a no-op for
			// them). Zero them on both operands before the struct compare. The
			// singbox dirty compare (statusSingbox) deliberately KEEPS them, as does
			// this one once the host is awg3-capable.
			dObf.stripAwg3()
			runObf.stripAwg3()
		} else if !(kernelSupports31Fn != nil && kernelSupports31Fn()) {
			// Same argument one version up: Enable strips the 3.1 flags on a host
			// whose module clears 3.0 but not 3.1, so leaving them in the saved
			// operand alone makes the banner permanent (#74 on the kernel backend —
			// reachable when the flags were saved on the singbox backend first).
			dObf.stripAwg31()
			runObf.stripAwg31()
		}
		configDirty = d.Subnet != subnet || d.ListenPort != port || d.MTU != mtu ||
			(d.WANIface != "" && d.WANIface != wan) || dObf != runObf || d.ObfPreset != obfPreset ||
			(kAwg3 && d.HeaderProtection != hp)
	}
	ifaceUp := false
	if out, _, err := m.run.Run(ctx, "awg", "show", m.iface); err == nil && strings.Contains(out, "listening port") {
		ifaceUp = true
	}
	// A live interface IS the server running, full stop. After a reboot RouteBox can
	// start before awg-quick@ brings the iface up (systemd ordering), so the boot-time
	// Rehydrate snapshot of enabled/phase is stale-false. Trust the live iface over the
	// snapshot, else the UI shows "Stopped" over a working tunnel until a manual re-enable.
	if ifaceUp {
		enabled, phase = true, PhaseReady
	}
	rulesPresent := false
	if rules, _, err := m.run.Run(ctx, "iptables", "-t", "nat", "-S"); err == nil && strings.Contains(rules, "RBOX-AWG-NAT") {
		rulesPresent = true
	}
	peers, _ := m.iface_ShowPeers(ctx)
	online := 0
	now := time.Now().Unix()
	handshakes, _ := m.iface_Handshakes(ctx)
	for _, ts := range handshakes {
		if isOnline(ts, now) {
			online++
		}
	}
	var rx, tx int64
	transfer, _ := m.iface_Transfer(ctx)
	for _, x := range transfer {
		rx += x.rx
		tx += x.tx
	}
	mod := StateNotInstalled
	if m.module != nil {
		mod = m.module.Status().State
	}
	// A live iface proves the module is loaded; the in-memory module state is only set
	// by Ensure()/loaded() and is stale-NotInstalled after a clean boot (no re-enable).
	if ifaceUp && mod != StateReady {
		mod = StateReady
	}
	return AWGStatus{
		Backend:           "kernel",
		KernelUnavailable: KernelBackendUnsupported(),
		Module:            mod, Enabled: enabled, Phase: phase, IfaceUp: ifaceUp, ListenPort: port,
		PublicHost: m.publicHost, PeerCount: len(peers), Online: online, Rx: rx, Tx: tx, WANIface: wan,
		NATOrphan: rulesPresent && !ifaceUp, ConfigDirty: configDirty, LastError: lastErr,
		KernelAWG3Available:  kAwg3,
		KernelAWG31Available: m.kernelSupports31Fn != nil && m.kernelSupports31Fn(),
	}
}

// detectWAN parses `ip route show default` (pure parseDefaultRoute from Task 5).
func (m *Manager) detectWAN(ctx context.Context) (string, error) {
	out, _, err := m.run.Run(ctx, "ip", "route", "show", "default")
	if err != nil {
		return "", err
	}
	return parseDefaultRoute(out)
}

// serverKeypair prefers a persisted key; else `awg genkey`/`awg pubkey`; else the
// in-Go Generate (the deterministic, tested fallback). The returned private key is
// canonical std-base64 (never raw operator input).
func (m *Manager) serverKeypair(ctx context.Context) (priv, pub string, err error) {
	m.mu.Lock()
	persisted := m.serverPriv
	m.mu.Unlock()
	if persisted != "" {
		if pub, err = PublicFromPrivate(persisted); err == nil {
			return persisted, pub, nil
		}
	}
	if out, _, e := m.run.Run(ctx, "awg", "genkey"); e == nil && strings.TrimSpace(out) != "" {
		priv = strings.TrimSpace(out)
		// Derive the public key in-Go (no shell pipe); validates priv decodes.
		if pub, e2 := PublicFromPrivate(priv); e2 == nil {
			return priv, pub, nil
		}
	}
	return Generate(rand.Reader)
}

// validateObf canonicalises operator-controlled obfuscation fields. Numeric J/S
// fields are bounded ints (already typed, so structurally safe); the H1-H4 string
// fields are the injection surface and MUST pass ValidateHField (digits or a
// "lo-hi" range). Returns a fresh Obfuscation built ONLY from validated values.
func validateObf(o Obfuscation) (Obfuscation, error) {
	for _, n := range []int{o.Jc, o.Jmin, o.Jmax, o.S1, o.S2, o.S3, o.S4} {
		if n < 0 || n > 65535 {
			return Obfuscation{}, fmt.Errorf("numeric obfuscation field %d out of range (0-65535)", n)
		}
	}
	// Jmin must be strictly below Jmax (junk size range). Only when junk is on.
	if o.Jmax != 0 && o.Jmin >= o.Jmax {
		return Obfuscation{}, fmt.Errorf("jmin (%d) must be < jmax (%d)", o.Jmin, o.Jmax)
	}
	// AWG: a handshake-init padded by S1 must not match a response padded by S2,
	// or the two packet sizes collide and become fingerprintable.
	if o.S1 != 0 && o.S2 != 0 && o.S1+56 == o.S2 {
		return Obfuscation{}, fmt.Errorf("s1+56 must not equal s2 (got s1=%d s2=%d)", o.S1, o.S2)
	}
	// Start from the input and overwrite only what canonicalisation changes.
	// Listing the carried-over fields by hand is how the two AWG 3.1 flags got
	// dropped for a release (#74): they never reached the rendered config, and the
	// running snapshot missing them could never equal the saved settings, so the
	// Apply banner stayed up for good. A copy cannot lose a field added later.
	out := o
	for i, h := range []*string{&o.H1, &o.H2, &o.H3, &o.H4} {
		if *h == "" {
			continue
		}
		v, err := ValidateHField(*h)
		if err != nil {
			return Obfuscation{}, fmt.Errorf("H%d: %w", i+1, err)
		}
		switch i {
		case 0:
			out.H1 = v
		case 1:
			out.H2 = v
		case 2:
			out.H3 = v
		case 3:
			out.H4 = v
		}
	}
	// AWG reserves header magics 1-4 for real WG message types; H1-H4 must be
	// distinct and > 4, else the iface fails to come up (a confusing teardown
	// instead of a clear 400). Only check non-empty fields (all-empty = obf off).
	seen := map[string]bool{}
	for i, h := range []string{out.H1, out.H2, out.H3, out.H4} {
		if h == "" {
			continue
		}
		if h == "1" || h == "2" || h == "3" || h == "4" {
			return Obfuscation{}, fmt.Errorf("H%d: values 1-4 are reserved", i+1)
		}
		if seen[h] {
			return Obfuscation{}, fmt.Errorf("H%d: duplicate header value %q", i+1, h)
		}
		seen[h] = true
	}
	// AWG 3.0 CPA/RAT are UintRange strings; validate + canonicalise when set.
	if o.CPA != "" {
		v, err := ValidateUintRange(o.CPA)
		if err != nil {
			return Obfuscation{}, fmt.Errorf("CPA: %w", err)
		}
		out.CPA = v
	}
	if o.RAT != "" {
		v, err := ValidateUintRange(o.RAT)
		if err != nil {
			return Obfuscation{}, fmt.Errorf("RAT: %w", err)
		}
		out.RAT = v
	}
	// AWG3 device-timers are UintRange strings too; validate + canonicalise when set.
	for _, f := range []struct {
		name string
		src  string
		dst  *string
	}{
		{"rekey_timeout", o.RekeyTimeout, &out.RekeyTimeout},
		{"reject_after_time", o.RejectAfterTime, &out.RejectAfterTime},
		{"keepalive_timeout", o.KeepaliveTimeout, &out.KeepaliveTimeout},
		{"max_handshake_attempts", o.MaxHandshakeAttempts, &out.MaxHandshakeAttempts},
	} {
		if f.src == "" {
			continue
		}
		v, err := ValidateUintRange(f.src)
		if err != nil {
			return Obfuscation{}, fmt.Errorf("%s: %w", f.name, err)
		}
		*f.dst = v
	}
	return out, nil
}

// firstHost returns the ".1" host address of a canonical IPv4 subnet (e.g.
// "10.10.0.0/24" -> "10.10.0.1").
func firstHost(subnet string) (string, error) {
	p, err := netip.ParsePrefix(subnet)
	if err != nil {
		return "", err
	}
	return p.Masked().Addr().Next().String(), nil
}

// maskSuffix returns the "/bits" suffix of a canonical prefix so the [Interface]
// Address carries the subnet mask (e.g. "/24"). subnet is already canonical.
func maskSuffix(subnet string) string {
	if i := strings.LastIndex(subnet, "/"); i >= 0 {
		return subnet[i:]
	}
	return ""
}

// Task 10 (handler) assembles the client Endpoint as "<public_host>:<listen_port>"
// and passes it verbatim to conf_client.BuildClient. A bare-IPv6 public_host MUST
// be bracketed there (e.g. "[::1]:51820") since BuildClient does not bracket. This
// orchestrator does not assemble the Endpoint, so the bracketing lives in Task 10.
