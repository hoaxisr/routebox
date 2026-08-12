package awg

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"routebox/backend/internal/awg/cps"
	"routebox/backend/internal/config"
	"routebox/backend/internal/util"
)

// PeerSummary is the secret-free API/UI view of a peer.
//
// On kernel, the liveness pair and Rx/Tx come from a real handshake and
// interface counters (awg show). On singbox they come from the WireGuard
// device's own UAPI state via peerStatsFn — a real handshake too, just
// reached over the Clash API instead of a kernel interface — falling back to
// livenessFn's traffic-derived approximation (see listPeersSingbox) only on
// an amnezia-box binary that predates that route.
type PeerSummary struct {
	Name          string `json:"name"`
	PublicKey     string `json:"public_key"`
	Address       string `json:"address"`
	LastHandshake int64  `json:"last_handshake"` // unix seconds; 0 = never seen
	Online        bool   `json:"online"`         // last seen within onlineWindowSec
	Rx            int64  `json:"rx"`             // cumulative bytes received (since iface up)
	Tx            int64  `json:"tx"`             // cumulative bytes sent (since iface up)
	ExpiresAt     int64  `json:"expires_at"`     // unix sec; 0 = never expires
}

// onlineWindowSec: a peer counts as "online" if its last handshake is newer than
// this. WireGuard rekeys roughly every ~120s while a tunnel is active, so 180s is a
// forgiving liveness window.
const onlineWindowSec = 180

// lastSeenLookbackSec bounds the singbox backend's liveness query. The roster
// shows "last seen 5h ago" for anything inside it and "never" beyond, so a day
// is the useful horizon — and it keeps the query off a month of history.
const lastSeenLookbackSec = 86400

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

	// singbox backend wiring (SetBackend/SetConfigSync). backend selects the peer-op
	// branch; cfgSync reconciles the managed endpoint in the sing-box config; applyFn
	// reloads after a real change; supportsFn gates on binary capability (awg2.1+).
	backend    string
	cfgSync    ConfigSyncer
	applyFn    func() error
	supportsFn func() bool

	// AWG3 wiring: supports3Fn gates the three awg3-only fields on binary
	// capability (nil = supported, tests inject a fake); headerKey is the active
	// header-protection key ("" = off), guarded by m.mu like obf.
	// headerProtection is the enabled-time toggle snapshot, compared against
	// desired().HeaderProtection by statusSingbox's ConfigDirty (I1) — headerKey
	// alone can't stand in for it (the key is persisted and may exist while the
	// toggle is off).
	supports3Fn      func() bool
	headerKey        string
	headerProtection bool

	// kernelSupports3Fn gates the kernel backend's own awg3 fields (CPA/RAT,
	// the four device-timers, and HeaderProtectionKey) on the host's kernel
	// module + awg-quick/tools capability (KernelSupportsAWG3). nil (unset) =
	// unsupported, fail-closed like KernelBackendUnsupported elsewhere — a
	// pre-awg3 awg-quick hard-fails on unknown [Interface] keys, so guessing
	// "supported" is the wrong default.
	kernelSupports3Fn func() bool

	// peerStatsFn is the singbox backend's real handshake/tx/rx signal: the
	// WireGuard device's own UAPI state via amnezia-box's /awg/{tag}/peers
	// (see SetPeerStats). Returns ErrAwgPeerStatsUnsupported on a pre-patch
	// amnezia-box binary (no such route) — listPeersSingbox falls back to
	// livenessFn in that case, and to "everyone offline" if neither is wired.
	peerStatsFn func() (map[string]PeerStat, error)

	// lastPeerStatsErr dedupes fetch-error logging for peerStatsFn the same
	// way traffic.Sampler dedupes its own: ListPeers can be polled every few
	// seconds, and a persistent failure (secret rotated, amnezia-box down)
	// must log once, not once per poll. Touched only under mu.
	lastPeerStatsErr string

	// livenessFn answers "when did this tunnel IP last move bytes", and exists
	// for the singbox backend as a fallback for amnezia-box binaries that
	// predate peerStatsFn (see SetPeerLiveness). nil = nothing wired, and
	// every peer reads offline — what the roster did before either existed.
	livenessFn func(since int64) map[string]int64

	// IPv6-broker wiring (dual-stack NAT-free AWG): ipv6Broker is the operator's
	// desire (from settings); v6Active is desire AND a passed egress preflight AND
	// mtu>=1280; ulaPrefix is the persisted /64 used to derive every v6 address
	// (zero when v6Active is false). probeFn is the injectable egress preflight;
	// nil defaults to defaultEgressProbe().ok (tests inject a fake).
	ipv6Broker bool
	v6Active   bool
	ulaPrefix  netip.Prefix
	probeFn    func() bool

	// addMu serialises every operation that writes what is served: the peer ops,
	// both Enable branches, and the sweep. Holders keep it across the apply that
	// follows the write, and the price of that is worth stating plainly, because
	// it is paid by an unrelated request.
	//
	// applyAwgReload asks the process manager to Reload and, if the reload fails,
	// Restarts the service — `systemctl restart` plus a 500ms verify sleep. So a
	// peer add arriving during an Enable can wait seconds on this mutex, and it
	// waits without regard to its request's context: a client that has already
	// given up still gets a late answer instead of an early error.
	//
	// Making the wait ctx-aware means a cap-1 channel semaphore in place of the
	// mutex, and it was not done, for two reasons. The waiting itself is correct
	// — a peer add landing mid-restart would write into a config being reloaded
	// and trigger a second reload of its own, and serialising that is what the
	// lock is for — so the fix would only shorten the wait for requests nobody is
	// listening to any more. And it would give every peer op a new early-exit
	// path that returns before doing anything, in a package whose entire failure
	// history is partial states. If the wait ever needs to be interruptible, the
	// semaphore wants a sync.Once-guarded lazy init: a nil channel in a
	// zero-value Manager blocks forever, and there are &Manager{} literals about.
	addMu sync.Mutex

	mu       sync.Mutex  // guards enabled/lastErr/phase/inFlight (distinct from addMu)
	enabled  bool        // last successful Enable left the tunnel up
	lastErr  string      // last orchestrator error (surfaced in Status)
	phase    EnablePhase // current orchestrator phase (phased Status)
	inFlight bool        // single-flight: an Enable orchestrator is running
}

// ---- Manager state: the commit rule every operation has to follow ----
//
// Read this before adding an operation that writes any field of the Manager
// above (the render parameters, enabled, or the store).
//
// NOTHING is committed to memory before the write it describes has succeeded,
// and when a write fails the WHOLE pre-operation snapshot goes back — not the
// fields that look most relevant to the failure.
//
// "The write" is the durable artefact the operation exists to produce: the
// server .conf on the kernel backend, the managed endpoint in the sing-box
// config on the singbox one, peers.toml for a secret. If the operation must
// publish state BEFORE it writes (renderServerSpec gates on enabled, so
// enableSingbox has to set it first), then snapshot everything you are about to
// overwrite (renderStateLocked), commit, write, and on failure put the snapshot
// back whole (restoreRenderStateLocked). Record the failure in lastErr/phase and
// nothing else: those two are the report, and they are the one thing a rollback
// must NOT undo.
//
// Which is also the rule for enabled specifically, since it is the field people
// reach for when an operation fails. Only an operation that CHANGED what is
// served may write it on a failure path: teardownFail, because it brought the
// interface down; markDisabled, because the endpoint is out of the config; a
// snapshot restore, because it is putting back a value the operation itself had
// published. enableFail and enableFailCause deliberately do not touch it — a
// failed attempt to change what is served is not a statement about what is
// being served.
//
// It holds for the background paths too, not just the operator-triggered ones.
// The 30s sweep re-probes IPv6 egress and commits the result before it writes,
// so it snapshots and restores like everything else; "the next tick will fix it"
// is not a licence, because the readers of that flag (RenderClientConf,
// ClientEndpoint) are gated on nothing and will hand out a wrong client config
// for the whole interval.
//
// Piecemeal rollbacks are what this rule exists to stop, because the fields have
// different readers with different gates. renderServerSpec and peerLines gate on
// enabled; RenderClientConf and ClientEndpoint gate on nothing at all. Put one
// field back and leave another and the panel starts describing a server that
// does not exist — a .conf naming a port nobody listens on, an expiry the
// endpoint never got, a client that the store says is live and sing-box has
// never heard of.
//
// It matters more here than in a plain UI cache because a 30s sweep
// (SweepExpired) re-derives the served config from exactly this state. A field
// left committed after a failed write is not a stale display value: it is an
// instruction, and the sweep will carry it out.
//
// Worked examples, each one a bug that reached a release: a failed re-Enable
// clearing enabled and having the sweep tear down a live endpoint; a failed
// Enable keeping the new port so every .conf and QR named a port nobody served;
// a renewal recorded in the store that the endpoint never accepted, leaving a
// peer the panel calls active and the server drops; a disable that removed the
// endpoint but stayed enabled because only the reload failed, so the sweep put
// the server back.

// ErrSubnetExhausted is returned by AddPeer when no /32 host remains free.
var ErrSubnetExhausted = fmt.Errorf("subnet exhausted")

// ErrPeerNotFound is returned by RenewPeer when the pubkey has no stored secret.
var ErrPeerNotFound = fmt.Errorf("peer not found")

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
	now := m.store.now()
	peers := m.store.List()
	out := make([]PeerLine, 0, len(peers))
	for _, p := range peers {
		if p.ExpiresAt != 0 && now >= p.ExpiresAt {
			continue // suspended: keep it out of the conf so Enable/Apply can't resurrect it
		}
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
	kAwg3 := m.kernelSupports3Fn != nil && m.kernelSupports3Fn()
	if !kAwg3 {
		// CPA/RAT + the four device-timers never render into the awg-quick conf on
		// an awg3-incapable host — restoring them into m.obf would make
		// RenderClientConf/RenderServer emit unknown [Interface] keys that a
		// pre-awg3 awg-quick hard-fails on. Strip them here exactly as kernel-Enable
		// does (RehydrateSingbox deliberately KEEPS them; so does this one once the
		// host is awg3-capable).
		obf.stripAwg3()
	}
	mtu, _ := ValidateMTU(in.MTU) // non-critical for render; 0 -> omitted
	dns, _ := ValidateDNS(in.DNS) // empty stays empty; the client conf omits the line

	// Restore the header key alongside the server key so a restart keeps
	// exporting the SAME HPK — but only when both header protection is desired
	// AND the host is awg3-capable (Enable with either false sets m.headerKey="",
	// and rehydrate must not resurrect it). Same S>=12 gate as kernel-Enable
	// (settings persist BEFORE Enable validates, so an invalid combo can be on
	// disk); mirrors RehydrateSingbox's Bug M1 fix.
	hpk := ""
	if kAwg3 && in.HeaderProtection {
		if err := validateHPKConstraint(obf, true); err == nil {
			hpk = m.store.HeaderKey()
		} else {
			log.Printf("awg: rehydrate: header protection is enabled in settings but %v — dropping the header key (degraded, loadable config)", err)
		}
	}

	up := false
	if out, _, e := m.run.Run(ctx, "awg", "show", m.iface); e == nil && strings.Contains(out, "listening port") {
		up = true
	}

	m.mu.Lock()
	m.serverPriv, m.subnet, m.serverIP, m.listenPort, m.mtu, m.dns, m.obf, m.wan =
		priv, subnet, serverIP, port, mtu, dns, obf, in.WANIface
	m.obfPreset = in.ObfPreset
	m.headerKey, m.headerProtection = hpk, kAwg3 && in.HeaderProtection
	m.enabled = up
	if up {
		m.phase = PhaseReady
	}
	m.mu.Unlock()
}

// AddPeer validates the name, allocates a /32 (from the on-disk .conf under the
// mutex), applies the peer live, appends the [Peer] block, and persists the
// secret. Rolls back the live add if persistence fails. Returns a secret-free
// summary. Subnet exhausted -> ErrSubnetExhausted; unusable name -> ErrInvalidName.
//
// The name is STORED AS TYPED (ValidatePeerName only trims and rejects) — running
// it through util.SanitizeName here is what used to turn every peer named "Ноутбук"
// into a peer named "name". Safe-token reduction happens where a token is
// actually required: PeerTag, the .conf comment, the download filename.
func (m *Manager) AddPeer(ctx context.Context, rawName string) (PeerSummary, error) {
	name, err := ValidatePeerName(rawName)
	if err != nil {
		return PeerSummary{}, err
	}
	m.addMu.Lock()
	defer m.addMu.Unlock()

	if m.backendIs("singbox") {
		return m.addPeerSingbox(ctx, name)
	}

	// Snapshot under m.mu, as addPeerSingbox does: Enable/Rehydrate write
	// serverIP/subnet under m.mu, and addMu (held here) does not order against
	// Rehydrate at all.
	m.mu.Lock()
	serverIP, subnet := m.serverIP, m.subnet
	m.mu.Unlock()
	used, err := m.usedFromConf()
	if err != nil {
		return PeerSummary{}, err
	}
	used = append(used, m.usedFromStore()...) // also reserve suspended peers' IPs (NextFree dedupes)
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
	if err := m.admit(ctx, peer); err != nil {
		return PeerSummary{}, err
	}
	if err := m.store.Put(peer); err != nil {
		// Roll the live+conf add back. If that fails too the peer is live with no
		// secret anywhere — invisible to the panel, undeletable through it, and
		// not foreign to the sweep only until the next Enable re-renders the conf
		// without it. The singbox branch logs the same case; discarding it here
		// left no trace at all.
		if serr := m.suspend(ctx, pub); serr != nil {
			log.Printf("awg: peer %s could not be persisted and could not be rolled back off %s: %v (store: %v)", pub, m.iface, serr, err)
		}
		return PeerSummary{}, err
	}
	return PeerSummary{Name: name, PublicKey: pub, Address: allowedIP}, nil
}

// SetPeerLiveness wires the singbox backend's substitute for a handshake: a
// lookup of tunnel IP -> the unix second it was last seen moving bytes, over
// everything at or after `since`. Wired from the traffic store in main; nil in
// tests and on panels with no monitoring. Used only as a fallback when
// peerStatsFn is unset or reports ErrAwgPeerStatsUnsupported.
func (m *Manager) SetPeerLiveness(fn func(since int64) map[string]int64) {
	m.mu.Lock()
	m.livenessFn = fn
	m.mu.Unlock()
}

// SetPeerStats wires the singbox backend's real per-peer handshake/tx/rx
// signal — see peerStatsFn. nil (unset) falls straight to the livenessFn
// fallback.
func (m *Manager) SetPeerStats(fn func() (map[string]PeerStat, error)) {
	m.mu.Lock()
	m.peerStatsFn = fn
	m.mu.Unlock()
}

// tunnelIP strips the mask from a stored peer address ("10.10.0.2/32" ->
// "10.10.0.2"), yielding the key traffic is recorded under. An unparseable
// address is returned unchanged — it cannot match anything, which is the right
// answer for a value that should not be in the store.
func tunnelIP(addr string) string {
	if pfx, err := netip.ParsePrefix(addr); err == nil {
		return pfx.Addr().String()
	}
	return addr
}

// ListPeers returns secret-free summaries from the store (sorted by PublicKey via
// the store's List). The PeerSummary type CANNOT serialise private/preshared keys.
func (m *Manager) ListPeers(ctx context.Context) []PeerSummary {
	if m.backendIs("singbox") {
		return m.listPeersSingbox()
	}
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
			ExpiresAt: p.ExpiresAt,
		})
	}
	return out
}

// listPeersSingbox fills the roster's liveness from the WireGuard device's
// own UAPI state (peerStatsFn) when available, falling back to recorded
// traffic (livenessFn) only on an amnezia-box binary that predates that
// route.
//
// sing-box serves AWG from a userspace endpoint: there is no awg-rb0 for
// `awg show` to query, so the kernel path above cannot be reused directly.
// amnezia-box (hoaxisr/amnezia-box) exposes the same handshake/tx/rx state
// `wg show`/`awg show` would read off a kernel interface through its Clash
// API instead (GET /awg/{tag}/peers) — see FetchAwgPeerStats — which is why
// this can key straight off PublicKey exactly like the kernel path does.
//
// The livenessFn fallback approximates liveness from the newest per-minute
// traffic bucket for the peer's tunnel IP instead — coarser (minute
// resolution) and prone to reading a connected-but-quiet peer as offline
// after onlineWindowSec, where a real handshake would keep rekeying — used
// only when peerStatsFn is unset or reports ErrAwgPeerStatsUnsupported.
func (m *Manager) listPeersSingbox() []PeerSummary {
	m.mu.Lock()
	statsFn, liveFn := m.peerStatsFn, m.livenessFn
	lastErr := m.lastPeerStatsErr
	m.mu.Unlock()

	now := time.Now().Unix()
	var stats map[string]PeerStat
	if statsFn != nil {
		var err error
		stats, err = statsFn()
		errText := ""
		if err != nil {
			errText = err.Error()
		}
		if errText != lastErr {
			if err != nil && !errors.Is(err, ErrAwgPeerStatsUnsupported) {
				log.Printf("awg: singbox peer stats unavailable, falling back to traffic-based liveness: %v", err)
			}
			m.mu.Lock()
			m.lastPeerStatsErr = errText
			m.mu.Unlock()
		}
	}

	var seen map[string]int64
	if stats == nil && liveFn != nil {
		seen = liveFn(now - lastSeenLookbackSec)
	}

	out := []PeerSummary{}
	for _, p := range m.store.List() {
		var ts, rx, tx int64
		if stat, ok := stats[p.PublicKey]; ok {
			ts, rx, tx = stat.LastHandshake, stat.RxBytes, stat.TxBytes
		} else if stats == nil {
			ts = seen[tunnelIP(p.Address)]
		}
		out = append(out, PeerSummary{
			Name: p.Name, PublicKey: p.PublicKey, Address: p.Address,
			LastHandshake: ts, Online: isOnline(ts, now), Rx: rx, Tx: tx,
			ExpiresAt: p.ExpiresAt,
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

// clientConfFor assembles the peer's client config: the stored secret, the derived
// server public key, the locked snapshot of the server's obfuscation/DNS/MTU, and
// the IPv6-broker address mapping. It is the single source of truth for every
// client export — the .conf and the Amnezia link must not describe the same peer
// differently. Returns the peer too, for its name.
func (m *Manager) clientConfFor(pub, host string) (ClientConf, Peer, error) {
	p, ok := m.store.Get(pub)
	if !ok {
		return ClientConf{}, Peer{}, fmt.Errorf("no such peer")
	}
	m.mu.Lock()
	serverPriv, dns, mtu, obf, port, preset, headerKey := m.serverPriv, m.dns, m.mtu, m.obf, m.listenPort, m.obfPreset, m.headerKey
	broker, ula, s3fn, kernelS3fn, backend := m.v6Active, m.ulaPrefix, m.supports3Fn, m.kernelSupports3Fn, m.backend
	m.mu.Unlock()
	serverPub, err := PublicFromPrivate(serverPriv)
	if err != nil {
		return ClientConf{}, Peer{}, err
	}
	// Gate on the ACTIVE backend's own capability signal, not the other one's:
	// supports3Fn probes the sing-box amnezia-box binary and is meaningless for a
	// kernel peer, kernelSupports3Fn probes the kernel module+tools and is
	// meaningless for a singbox peer. Same gate as renderServerSpec/ClientEndpoint
	// (singbox) and kernel-Enable/Rehydrate (kernel) respectively. A nil s3fn means
	// "supported" (legacy singbox stance); a nil kernelS3fn means "unsupported"
	// (fail-closed, matching KernelBackendUnsupported's stance elsewhere).
	supported := s3fn == nil || s3fn()
	if backend == "kernel" {
		supported = kernelS3fn != nil && kernelS3fn()
	}
	if !supported {
		obf.stripAwg3()
		headerKey = ""
	}
	// No header key = header protection is off = the server is AWG 2.0 as far as a
	// client can tell, so the export must carry no AWG3-only key. It used to keep
	// the device-timers and the content padding whenever the BINARY could do awg3,
	// which made every non-off obfuscation preset produce a config the iOS
	// AmneziaWG refuses outright ("not an AmneziaWG configuration", #64) and AWGM
	// labels 3.0 (#60). The server keeps its own timers and padding — both are
	// sender-local, the peer never has to match them.
	// A "lo-hi" PersistentKeepalive is AWG 3.0-only too, and it is configured with
	// no reference to header protection at all — so it leaked into "2.0" exports by
	// the same route until it was collapsed here.
	keepalive := m.clientKeepalive()
	if headerKey == "" {
		obf.stripAwg3()
		keepalive = collapseRange(keepalive)
	}
	// No fallback resolver: an empty field means the client keeps its own DNS.
	// Inventing 1.1.1.1 here silently overrode routing that worked without it,
	// and the UI never showed the value it was substituting (#45).
	addr6, allowed := "", []string{"0.0.0.0/0"}
	if broker {
		if a, err := netip.ParsePrefix(p.Address); err == nil {
			if m6, err := MapV4ToV6(ula, a.Addr()); err == nil {
				addr6 = m6.String() + "/128"
				allowed = []string{"0.0.0.0/0", "::/0"}
			}
		}
	}
	return ClientConf{
		PrivateKey:          p.PrivateKey,
		Address:             p.Address,
		Address6:            addr6,
		DNS:                 dns,
		MTU:                 mtu,
		Obf:                 obf,
		Mimic:               cps.Mimic(preset),
		ServerPub:           serverPub,
		Endpoint:            joinHostPort(host, port),
		AllowedIPs:          allowed,
		Keepalive:           keepalive,
		PSK:                 p.PresharedKey,
		HeaderProtectionKey: headerKey,
	}, p, nil
}

// DefaultClientKeepalive is the PersistentKeepalive every client export used
// before the value became configurable; still the fallback when it is unset or
// unusable.
const DefaultClientKeepalive = "25"

// clientKeepalive resolves the PersistentKeepalive for client exports from the
// live saved settings (not from Enable-time state: it never reaches the server
// device, so a change applies to the next export without re-enabling anything).
// Canonicalised through the same UintRange parser the AWG3 fields use, so an
// impossible value falls back to 25 rather than producing an unimportable .conf.
func (m *Manager) clientKeepalive() string {
	m.mu.Lock()
	desired := m.desired
	m.mu.Unlock()
	if desired == nil {
		return DefaultClientKeepalive
	}
	v, err := ValidateUintRange(desired().ClientKeepalive)
	if err != nil || v == "" {
		return DefaultClientKeepalive
	}
	return v
}

// RenderClientConf builds the client .conf. host is the validated public host; the
// Endpoint is assembled IPv6-safe (bare v6 host bracketed).
func (m *Manager) RenderClientConf(pub, host string) (string, error) {
	c, _, err := m.clientConfFor(pub, host)
	if err != nil {
		return "", err
	}
	return BuildClient(c)
}

// RenderVPNLink builds the Amnezia vpn:// link for the same peer. Returns an error
// wrapping ErrLinkUnrepresentable when the peer's parameters cannot be expressed in
// Amnezia's schema; the caller should surface that message rather than a 500.
func (m *Manager) RenderVPNLink(pub, host string) (string, error) {
	c, p, err := m.clientConfFor(pub, host)
	if err != nil {
		return "", err
	}
	return AmneziaLink(c, p.Name)
}

// joinHostPort joins a host (domain or IP) and port, bracketing IPv6 literals so
// BuildClient (which passes Endpoint verbatim) never emits a bare-v6 "host:port".
func joinHostPort(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// admit applies a peer to the live interface and writes its [Peer] block to the
// conf (idempotent: any existing block for the same key is replaced, so calling it
// on an already-live peer cannot duplicate the stanza). Shared by AddPeer (new
// peer) and RenewPeer (re-admitting a suspended peer from its stored secret).
// Caller holds m.addMu.
func (m *Manager) admit(ctx context.Context, p Peer) error {
	pskFile := filepath.Join(m.pskTmpDir, strconv.FormatInt(time.Now().UnixNano(), 10)+".psk")
	// Classified like every other write: the PSK goes through a file because
	// `awg set` will not take a key on the command line, and a temp directory
	// that cannot be written is the same kind of problem as a config that cannot.
	if err := util.ClassifyWriteErr(pskFile, os.WriteFile(pskFile, []byte(p.PresharedKey+"\n"), 0600)); err != nil {
		return err
	}
	defer os.Remove(pskFile)
	if err := m.iface_SetPeer(ctx, p.PublicKey, pskFile, p.Address); err != nil {
		return fmt.Errorf("awg set: %w", err)
	}
	if err := m.rewriteConfWithout(p.PublicKey); err != nil { // drop any stale block (idempotent upsert)
		_ = m.iface_RemovePeer(ctx, p.PublicKey)
		return err
	}
	if err := m.appendPeerToConf(PeerLine{Name: p.Name, PublicKey: p.PublicKey, PSK: p.PresharedKey, AllowedIP: p.Address}); err != nil {
		_ = m.iface_RemovePeer(ctx, p.PublicKey)
		return err
	}
	return nil
}

// suspend removes a peer from the live interface and the on-disk conf, LEAVING the
// store secret intact. Shared by RemovePeer (which then deletes the secret) and
// SweepExpired (which keeps it). Caller holds m.addMu.
func (m *Manager) suspend(ctx context.Context, pub string) error {
	if err := m.iface_RemovePeer(ctx, pub); err != nil {
		return fmt.Errorf("awg set remove: %w", err)
	}
	return m.rewriteConfWithout(pub)
}

// RemovePeer removes a peer live and from the on-disk .conf + secret store.
// RemovePeer live-removes the peer and returns its tunnel Address (e.g. "10.10.64.2/32",
// empty if the peer was unknown) so the caller can purge its per-source traffic history.
func (m *Manager) RemovePeer(ctx context.Context, pub string) (string, error) {
	if _, err := ValidatePublicKey(pub); err != nil {
		return "", err
	}
	m.addMu.Lock()
	defer m.addMu.Unlock()
	prev, existed := m.store.Get(pub)
	addr := ""
	if existed {
		addr = prev.Address
	}
	if m.backendIs("singbox") {
		if err := m.store.Delete(pub); err != nil {
			return "", err
		}
		if err := m.singboxSync(); err != nil {
			// Compensation, mirroring addPeerSingbox: the endpoint still carries
			// this peer, so dropping its secret would strand a live client the
			// panel can no longer see or delete.
			if existed {
				if perr := m.store.Put(prev); perr != nil {
					// Both writes failed: the peer is live in sing-box and its
					// secret is gone from the store. Nothing left to do but say so.
					log.Printf("awg: peer %s removed from the store but its endpoint could not be rewritten, and restoring it failed: %v (sync: %v)", pub, perr, err)
				}
			}
			return "", err
		}
		return addr, nil
	}
	if err := m.suspend(ctx, pub); err != nil {
		return "", err
	}
	if err := m.store.Delete(pub); err != nil {
		// Mirror of the singbox branch, other way round: here the interface and
		// the .conf are the write that succeeded and the store is the one that
		// failed, so the peer is off everything that serves it while the store —
		// and the panel — still calls it an active client. Nothing heals that: it
		// has not expired, and the store owning it is exactly what makes it OURS
		// to the foreign-peer sweep. admit is idempotent, so putting it back is
		// the whole compensation.
		//
		// Except for a SUSPENDED peer, which lived in the store alone: peerLines
		// keeps it out of the conf and the sweep took it off the interface, so
		// re-admitting it would restore more than there was and put an expired
		// key back into service until the next tick noticed. Its store entry is
		// already back — Delete rolls its own save failure back — and that is the
		// whole of what it had.
		now := m.store.now()
		suspended := prev.ExpiresAt != 0 && now >= prev.ExpiresAt
		if existed && !suspended {
			if aerr := m.admit(ctx, prev); aerr != nil {
				log.Printf("awg: peer %s could not be deleted from the store, and re-admitting it failed: %v (store: %v)", pub, aerr, err)
			}
		}
		return "", err
	}
	return addr, nil
}

// RenewPeer sets a peer's ExpiresAt and ensures it is admitted to the live
// interface + conf. expiresAt is 0 (never) or a future unix ts (the handler
// validates), so the peer is always active afterward; admit is idempotent, so this
// is safe whether the peer was suspended or already live. ErrPeerNotFound if unknown.
func (m *Manager) RenewPeer(ctx context.Context, pub string, expiresAt int64) error {
	if _, err := ValidatePublicKey(pub); err != nil {
		return err
	}
	m.addMu.Lock()
	defer m.addMu.Unlock()
	p, ok := m.store.Get(pub)
	if !ok {
		return ErrPeerNotFound
	}
	prev := p
	p.ExpiresAt = expiresAt
	if err := m.store.Put(p); err != nil {
		return err
	}
	if m.backendIs("singbox") {
		if err := m.singboxSync(); err != nil {
			// The endpoint kept the old expiry, so the store must too — otherwise
			// the panel shows a renewal that never reached sing-box.
			if perr := m.store.Put(prev); perr != nil {
				log.Printf("awg: peer %s expiry could not be synced, and restoring the old value failed: %v (sync: %v)", pub, perr, err)
			}
			return err
		}
		return nil
	}
	if err := m.admit(ctx, p); err != nil {
		// Mirror of the singbox branch above. admit rolls its own changes back,
		// so the interface and the .conf still carry the old expiry — and the
		// store must too, or the panel shows a renewal the peer never got. The
		// sweep cannot heal this: it only suspends peers whose expiry has passed.
		if perr := m.store.Put(prev); perr != nil {
			log.Printf("awg: peer %s could not be re-admitted, and restoring its old expiry failed: %v (admit: %v)", pub, perr, err)
		}
		return err
	}
	return nil
}

// SweepExpired reconciles the live interface with the store, in both directions:
// it suspends every peer whose ExpiresAt has passed (keeping its store secret for
// later renewal) and strips every peer that is ours nowhere. Idempotent, and a
// no-op when the interface is down (iface_ShowPeers errors → return). Run by the
// sweep ticker.
//
// It costs one `awg show` per tick even when nothing is expired — the earlier
// early-out on an empty expiry set is what left the foreign-peer pass running
// only after an Enable, i.e. never on a box that works.
func (m *Manager) SweepExpired(ctx context.Context) {
	if m.backendIs("singbox") {
		// Re-preflight the IPv6 broker on every sweep: a v6 egress that appears or
		// disappears between Enable/Rehydrate and now must flip v6Active so the
		// next singboxSync (change-gated, so an unchanged spec is a no-op) picks it
		// up without requiring an operator re-Apply.
		//
		// The probe is a network dial (up to ~6s when egress is down) and MUST run
		// before addMu is taken, or it stalls concurrent AddPeer for its duration.
		m.mu.Lock()
		broker, ula, probe := m.ipv6Broker, m.ulaPrefix, m.probeFn
		m.mu.Unlock()
		// Only re-arm v6Active once a ULA prefix already exists (assigned by a
		// prior successful Enable/Rehydrate) — never fabricate one here, that
		// stays enableSingbox's job.
		probeNeeded := broker && ula.IsValid()
		var probeResult bool
		if probeNeeded {
			if probe == nil {
				probe = defaultEgressProbe().ok
			}
			probeResult = probe()
		}

		m.addMu.Lock()
		defer m.addMu.Unlock()
		// The probe result is a state commit ahead of a write, so it follows the
		// same rule as the rest of the package: snapshot, commit, and put the
		// snapshot back if the write does not land (see the commit rule above).
		// The sweep being its own retry loop is not licence to skip it —
		// RenderClientConf and ClientEndpoint read v6Active without gating on
		// anything, so a flip the endpoint never accepted has every .conf and QR
		// issued in the next 30 seconds promise addresses the server does not route.
		prevV6, v6Committed := false, false
		if probeNeeded {
			m.mu.Lock()
			// Re-check under the lock (not the stale snapshot): a concurrent
			// Disable/toggle between the probe and here must not resurrect
			// v6Active without a currently-valid ULA prefix.
			if m.ipv6Broker && m.ulaPrefix.IsValid() {
				prevV6, v6Committed = m.v6Active, true
				m.v6Active = probeResult
			}
			m.mu.Unlock()
		}
		// Best-effort self-heal: expired peers drop out of renderServerSpec, and a
		// disabled server renders nil (no-op). A failure here means the active
		// config silently diverges from the store — log it, don't swallow it.
		//
		// Except a read-only config: that is a standing, known condition already
		// reported at startup, in GET /api/status and on every interactive
		// attempt (409). Repeating it every 30s would bury the failures that
		// actually need reading — and on a router it is flash wear for a line
		// nobody can act on differently. A config that turns unwritable AFTER
		// startup still logs here: the verdict is stale, so the write fails with
		// a raw permission error, not with ErrReadOnly.
		if err := m.singboxSync(); err != nil {
			if v6Committed {
				m.mu.Lock()
				m.v6Active = prevV6
				m.mu.Unlock()
			}
			if !errors.Is(err, config.ErrReadOnly) {
				log.Printf("awg: singbox sweep sync: %v", err)
			}
		}
		return
	}
	m.addMu.Lock()
	defer m.addMu.Unlock()
	// The listing is taken UNDER addMu and reused for both passes below. AddPeer
	// admits a peer to the interface and only then does store.Put, so a listing
	// taken before the lock could show a peer no store knows yet — which the
	// foreign pass would read as foreign and strip off, leaving a client that can
	// never connect while the panel reports it fine.
	live, err := m.iface_ShowPeers(ctx)
	if err != nil {
		return // interface down / not enabled — nothing to enforce now
	}
	now := m.store.now()
	expired := map[string]bool{}
	for _, p := range m.store.List() {
		if p.ExpiresAt != 0 && now >= p.ExpiresAt {
			expired[p.PublicKey] = true
		}
	}
	// The expiry map is built from a store snapshot taken under addMu, and every
	// writer of ExpiresAt (AddPeer/RemovePeer/RenewPeer) holds addMu too, so no
	// peer can be renewed out from under this loop and be suspended anyway —
	// off the interface with the store calling it active, which nothing heals.
	for _, pub := range live {
		if !expired[pub] {
			continue
		}
		if err := m.suspend(ctx, pub); err != nil {
			log.Printf("awg: suspend expired peer %s: %v", pub, err)
		}
	}
	// Same pass, other direction: peers on the interface that are ours nowhere.
	// This used to run only off the tail of a successful Enable, so on a box that
	// works — nobody re-Applies a working server — a leftover of a crash or of a
	// manual `awg set` stayed live indefinitely. The suspensions above only make
	// that worse: `live` still names them, but they are ours (the store keeps
	// their secrets), so the foreign pass leaves them alone.
	m.sweepForeignLivePeersLocked(ctx, live)
}

// usedFromStore returns the /32 host addresses of ALL stored peers (including
// suspended ones, which are absent from the conf). Reserving these prevents a new
// peer from grabbing a suspended peer's IP and breaking its later renewal.
func (m *Manager) usedFromStore() []netip.Addr {
	var used []netip.Addr
	for _, p := range m.store.List() {
		if pre, err := netip.ParsePrefix(p.Address); err == nil {
			used = append(used, pre.Addr())
		}
	}
	return used
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
//
// The failure is classified: the .conf shares its directory with peers.toml, so
// the same read-only mount takes both — and the kernel peer operations write the
// .conf BEFORE the store, so without this the store's guard never gets a say and
// the user got a 500 naming nothing while the badge was already up.
func (m *Manager) writeConfAtomic(content string) error {
	return util.ClassifyWriteErr(m.confPath, m.writeConf(content))
}

func (m *Manager) writeConf(content string) error {
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
