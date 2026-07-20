package awg

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"time"
)

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
	LastError   string      `json:"last_error,omitempty"`
}

// EnableInput is the RAW operator submission; Enable canonicalises every field.
type EnableInput struct {
	Subnet     string      `json:"subnet"`
	ListenPort int         `json:"listen_port"`
	MTU        int         `json:"mtu"`
	DNS        []string    `json:"dns"`
	WANIface   string      `json:"wan_iface"` // optional override; "" -> DetectWAN
	Obf        Obfuscation `json:"obf"`
	ObfPreset  string      `json:"obf_preset"`
	// HeaderProtection toggles the AWG3 header-protection key (singbox backend,
	// awg3+ binary only). Obf carries the awg3 CPA/RAT strings.
	HeaderProtection bool `json:"header_protection"`
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
// Status() can report progress. The trigger returns the orchestrator's terminal
// result; the Task-10 HTTP handler runs Enable in a goroutine so its own request
// returns promptly while Status() reflects each phase.
func (m *Manager) Enable(ctx context.Context, in EnableInput) error {
	if !m.beginEnable() {
		return fmt.Errorf("enable already in progress")
	}
	defer m.endEnable()
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
	sc := ServerConf{
		PrivateKey: priv, Address: serverIP + maskSuffix(subnet), ListenPort: port, MTU: mtu,
		Subnet: subnet, WAN: wan, Iface: m.iface, Obf: obf,
	}
	if err := m.writeConfAtomic(RenderServer(sc, m.peerLines())); err != nil {
		return m.enableFail(err.Error())
	}
	// Persist canonical values for later peer ops.
	m.mu.Lock()
	m.subnet, m.serverIP, m.listenPort, m.mtu, m.wan, m.dns, m.serverPriv, m.obf =
		subnet, serverIP, port, mtu, wan, dns, priv, obf
	m.obfPreset = in.ObfPreset
	m.mu.Unlock()

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
	return nil
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

func (m *Manager) enableFail(msg string) error {
	m.mu.Lock()
	m.enabled, m.lastErr, m.phase = false, msg, PhaseFailed
	m.mu.Unlock()
	return fmt.Errorf("%s", msg)
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
	m.mu.Unlock()
	if phase == "" {
		phase = PhaseIdle
	}
	// ConfigDirty = enabled and the saved settings differ from what is running, on
	// any field that needs an interface restart (subnet/port/mtu/wan/obf). DNS is
	// client-only (regenerated at config download), so it never marks dirty.
	configDirty := false
	if enabled && desired != nil {
		d := desired()
		configDirty = d.Subnet != subnet || d.ListenPort != port || d.MTU != mtu ||
			(d.WANIface != "" && d.WANIface != wan) || d.Obf != obf || d.ObfPreset != obfPreset
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
	for _, ts := range m.iface_Handshakes(ctx) {
		if isOnline(ts, now) {
			online++
		}
	}
	var rx, tx int64
	for _, x := range m.iface_Transfer(ctx) {
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
		Backend: "kernel",
		Module:  mod, Enabled: enabled, Phase: phase, IfaceUp: ifaceUp, ListenPort: port,
		PublicHost: m.publicHost, PeerCount: len(peers), Online: online, Rx: rx, Tx: tx, WANIface: wan,
		NATOrphan: rulesPresent && !ifaceUp, ConfigDirty: configDirty, LastError: lastErr,
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
	out := Obfuscation{
		Jc: o.Jc, Jmin: o.Jmin, Jmax: o.Jmax,
		S1: o.S1, S2: o.S2, S3: o.S3, S4: o.S4,
	}
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
