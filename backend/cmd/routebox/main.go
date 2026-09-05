package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"

	"routebox/backend/internal/api"
	"routebox/backend/internal/auth"
	"routebox/backend/internal/awg"
	"routebox/backend/internal/clients"
	"routebox/backend/internal/config"
	"routebox/backend/internal/embedded"
	"routebox/backend/internal/geoip"
	"routebox/backend/internal/mtproto"
	"routebox/backend/internal/panelcert"
	"routebox/backend/internal/process"
	"routebox/backend/internal/settings"
	"routebox/backend/internal/subscriptions"
	"routebox/backend/internal/traffic"
	"routebox/backend/internal/updates"
	"routebox/backend/internal/users"
	"routebox/backend/internal/util"
	"routebox/backend/internal/v2stats"
)

// Version is stamped at build time via -ldflags "-X main.Version=…".
// "dev" for plain `go build` / `go run`.
var Version = "dev"

// minimalSingboxConfig is a valid, startable sing-box/amnezia-box config (no inbounds
// + a direct outbound). Scaffolded in vps mode when config.json is absent so the
// amnezia-box service doesn't crash-loop ("read config: no such file") before the
// panel writes a real config on first apply.
const minimalSingboxConfig = `{"log":{"level":"info","timestamp":true},"inbounds":[],"outbounds":[{"tag":"direct","type":"direct"}],"route":{"auto_detect_interface":true,"final":"direct"}}`

// panelCertDir is the canonical PEM mirror of the panel's TLS cert, so server
// inbounds can reuse it via a "use panel cert" option.
const panelCertDir = "/etc/routebox/panel-cert"

var (
	settingsPath = flag.String("settings", "", "Path to routebox.toml settings file (auto-detected if not specified)")
	// Legacy flags - override settings file if specified
	configPath = flag.String("config", "", "Path to amnezia-box config file (overrides settings)")
	listenAddr = flag.String("listen", "", "HTTP listen address (overrides settings)")
	clashAddr  = flag.String("clash", "", "Clash API address (overrides settings)")
	geoipPath  = flag.String("geoip", "", "Path to GeoIP MMDB database (overrides settings)")
	binaryPath = flag.String("binary", "", "Path to the amnezia-box binary (overrides settings; auto-detected if unset)")
	modeFlag   = flag.String("mode", "", "panel mode: router (default) or vps")
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("routebox version %s\n", Version)
		return
	}
	// panel-url exists because the secret address is announced once, on the first
	// start, and a container log rotates. Whoever has the machine can ask again.
	if len(os.Args) > 1 && os.Args[1] == "panel-url" {
		if err := flag.CommandLine.Parse(os.Args[2:]); err != nil {
			os.Exit(2)
		}
		sm, err := settings.NewManager(*settingsPath)
		if err != nil {
			log.Fatalf("Failed to load settings: %v", err)
		}
		url := panelURL(sm.Get())
		if url == "" {
			fmt.Fprintln(os.Stderr, "This install has no panel gate: it was not brought up by the out-of-the-box bootstrap, so the panel answers on its own listen address.")
			os.Exit(1)
		}
		fmt.Println(url)
		return
	}
	flag.Parse()

	// Load settings
	settingsMgr, err := settings.NewManager(*settingsPath)
	if err != nil {
		log.Fatalf("Failed to load settings: %v", err)
	}
	cfg := settingsMgr.Get()

	// Resolve effective mode: CLI flag > settings > default
	effectiveMode := cfg.Server.Mode
	if *modeFlag != "" {
		effectiveMode = *modeFlag
	}
	if effectiveMode == "" {
		effectiveMode = "router"
	}

	// Root is only required in router mode, where routebox itself creates the
	// TUN interface. VPS mode never touches TUN — amnezia-box's own inbounds
	// bind ports like any other userspace process — so it can run unprivileged
	// (e.g. under a container's PUID/PGID user).
	if effectiveMode == "router" && os.Geteuid() != 0 {
		fmt.Println()
		fmt.Println("╔══════════════════════════════════════════════════════════════════╗")
		fmt.Println("║  ERROR: Root privileges required                                 ║")
		fmt.Println("╠══════════════════════════════════════════════════════════════════╣")
		fmt.Println("║  routebox must run as root to create TUN interfaces.             ║")
		fmt.Println("║                                                                  ║")
		fmt.Println("║  Please run with sudo:                                           ║")
		fmt.Println("║     sudo ./routebox                                              ║")
		fmt.Println("║                                                                  ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════════╝")
		fmt.Println()
		os.Exit(1)
	}

	// Print settings info
	if settingsMgr.GetPath() != "" {
		fmt.Printf("Settings: %s\n", settingsMgr.GetPath())
	}

	// Initialize process manager
	procMgr := process.NewManager()

	// Resolve the amnezia-box binary: CLI flag > settings > auto-detect. An
	// explicit path wins over the systemd ExecStart the manager prefers on its
	// own — pinning it is how a packaged install (the Docker image keeps the
	// binary on the writable /config volume so the panel can update it) says
	// where the binary it actually runs lives.
	resolvedBinaryPath := *binaryPath
	if resolvedBinaryPath == "" {
		resolvedBinaryPath = cfg.Singbox.BinaryPath
	}
	if replaced := procMgr.PinBinaryPath(resolvedBinaryPath); replaced != "" {
		log.Printf("amnezia-box binary pinned to %s (detected %s). Status, version and updates all follow the pinned path.", resolvedBinaryPath, replaced)
	}

	// Resolve config path: CLI flag > settings > auto-detect > default
	resolvedConfigPath := *configPath
	if resolvedConfigPath == "" {
		resolvedConfigPath = cfg.Singbox.ConfigPath
	}
	if resolvedConfigPath == "" {
		// Try to detect from running process or systemd service
		if detected := procMgr.GetDetectedConfigPath(); detected != "" {
			resolvedConfigPath = detected
			fmt.Printf("Auto-detected config path: %s\n", resolvedConfigPath)
		} else {
			// Fallback to default (try Entware path first)
			for _, fallback := range []string{
				"/opt/etc/sing-box/config.json",
				"/opt/etc/amnezia-box/config.json",
				"/etc/amnezia-box/config.json",
			} {
				if _, err := os.Stat(filepath.Dir(fallback)); err == nil {
					resolvedConfigPath = fallback
					break
				}
			}
			if resolvedConfigPath == "" {
				resolvedConfigPath = "/etc/amnezia-box/config.json"
			}
		}
	}

	// Сверка с systemd-юнитом: если он стартует amnezia-box с другим конфигом,
	// всякая правка уходит в файл, который процесс не читает. Ничего не чиним
	// автоматически — фиксируем расхождение, панель предложит лечение.
	// Сравнение (с нормализацией путей) живёт в SetConfigPaths — здесь только
	// сообщаем о результате, чтобы условие не разъехалось между двумя местами.
	procMgr.SetConfigPaths(resolvedConfigPath, procMgr.UnitConfigPath())
	if ours, unit, mismatched := procMgr.ConfigMismatch(); mismatched {
		log.Printf("WARNING: config path mismatch — RouteBox edits %s, systemd unit starts amnezia-box with %s. Start/Restart/Reload are blocked until this is resolved in the panel.", ours, unit)
	}

	// Ensure config directory exists
	configDir := filepath.Dir(resolvedConfigPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		log.Fatalf("Failed to create config directory %s: %v", configDir, err)
	}

	// Initialize config manager
	var cfgMgr *config.Manager

	if _, statErr := os.Stat(resolvedConfigPath); statErr == nil {
		// Config exists - load it
		var err error
		cfgMgr, err = config.NewManager(resolvedConfigPath)
		if err != nil {
			log.Printf("Warning: Could not load config from %s: %v", resolvedConfigPath, err)
			cfgMgr = config.NewEmptyManager(resolvedConfigPath)
		}
	} else if effectiveMode == "vps" && os.Getenv(allInOneEnv) != "" {
		// Out-of-the-box install: the same moment the minimal config below is
		// written, except the whole server is planned instead — every inbound, the
		// stub site and the panel behind its gate. Fatal on failure: the operator
		// asked for a configured server, and a panel that came up with an empty
		// config instead would look like it worked.
		a, err := planAllInOne(cfg, settingsMgr.GetPath(), resolvedConfigPath, orDefault(*listenAddr, cfg.Network.Listen))
		if err != nil {
			log.Fatalf("%v", err)
		}
		if err := runAllInOne(settingsMgr, a, os.Stdout); err != nil {
			log.Fatalf("out-of-the-box bootstrap: %v", err)
		}
		// The bootstrap writes [server] and [network]; cfg was read before it.
		cfg = settingsMgr.Get()
		m, lerr := config.NewManager(resolvedConfigPath)
		if lerr != nil {
			log.Fatalf("the planned config at %s does not load: %v", resolvedConfigPath, lerr)
		}
		cfgMgr = m
	} else if effectiveMode == "vps" {
		// VPS/panel mode: the installer enables the amnezia-box service at install time,
		// which crash-loops ("read config: no such file") until a config exists. Scaffold
		// a minimal valid sing-box config so amnezia-box starts cleanly on its next
		// auto-restart; the panel overwrites it on the first apply.
		if err := os.WriteFile(resolvedConfigPath, []byte(minimalSingboxConfig), 0644); err != nil {
			log.Printf("Warning: could not scaffold minimal config %s: %v", resolvedConfigPath, err)
			cfgMgr = config.NewEmptyManager(resolvedConfigPath)
		} else if m, lerr := config.NewManager(resolvedConfigPath); lerr == nil {
			fmt.Printf("Scaffolded minimal amnezia-box config at %s\n", resolvedConfigPath)
			cfgMgr = m
		} else {
			cfgMgr = config.NewEmptyManager(resolvedConfigPath)
		}
	} else {
		// Router mode: the setup wizard creates the config.
		cfgMgr = config.NewEmptyManager(resolvedConfigPath)
	}

	// Clean up any stale draft from previous session
	cfgMgr.CleanupDraft()

	// Use the currently detected sing-box/amnezia-box binary for config
	// validation — provider, not a snapshot, so later detection is picked up
	cfgMgr.SetCheckBinaryProvider(procMgr.GetBinaryPath)

	// Set config path for process manager
	procMgr.SetConfigPath(resolvedConfigPath)

	// Resolve Clash API address: CLI flag > settings > auto-detect
	resolvedClashAddr := *clashAddr
	if resolvedClashAddr == "" {
		resolvedClashAddr = cfg.Singbox.ClashAPI
	}
	if resolvedClashAddr == "" {
		resolvedClashAddr = resolveClashAddrFromConfig(cfgMgr)
	}
	// Clash API secret for in-process clients (discovery loop, traffic sampler).
	// Resolved once at startup, like the address: changing it requires a restart.
	resolvedClashSecret := resolveClashSecretFromConfig(cfgMgr)
	if resolvedClashAddr == "" {
		// Said once, here, rather than left to the samplers: they would report it
		// as a failed request to an empty URL, once a minute, in terms that say
		// nothing about what is unset or where to set it.
		log.Printf("Clash API address is not set (singbox.clash_api, or experimental.clash_api in the amnezia-box config): live connections and traffic history are off. It is read at startup, so restart RouteBox after setting it.")
	}

	// An inbound bound to loopback is reachable only through the front (ADR 0001),
	// and with no front port configured its client links cannot be built at all.
	// BuildSubscription drops those bindings WITHOUT logging — /sub is public and
	// unauthenticated, so it would log on every request — which makes an install
	// that is one setting away from working look like one that quietly works.
	// Said once, here, where it is a fact about the configuration.
	if cfg.Server.FrontPort == 0 {
		if tags := frontedInboundTags(cfgMgr.GetActive()); len(tags) > 0 {
			log.Printf("Inbounds %s listen on loopback and are reachable only through a front, but server.front_port is not set: their client links are skipped in subscriptions and share links. It is read at startup, so restart RouteBox after setting it.", strings.Join(tags, ", "))
		}
	}

	// Resolve GeoIP path: CLI flag > settings
	resolvedGeoipPath := *geoipPath
	if resolvedGeoipPath == "" {
		resolvedGeoipPath = cfg.GeoIP.Path
	}

	// Load GeoIP database if specified
	var geoipDB *geoip.DB
	if resolvedGeoipPath != "" {
		var err error
		geoipDB, err = geoip.Open(resolvedGeoipPath)
		if err != nil {
			log.Printf("Warning: Could not load GeoIP database from %s: %v", resolvedGeoipPath, err)
		} else {
			fmt.Printf("GeoIP database: %s\n", resolvedGeoipPath)
		}
	}

	// Resolve listen address: CLI flag > settings
	resolvedListenAddr := *listenAddr
	if resolvedListenAddr == "" {
		resolvedListenAddr = cfg.Network.Listen
	}

	// Initialize clients manager (LAN device names)
	var clientsPath string
	if sp := settingsMgr.GetPath(); sp != "" {
		clientsPath = filepath.Join(filepath.Dir(sp), "clients.toml")
	}
	clientsMgr := clients.New(clientsPath)
	if clientsPath != "" {
		if err := clientsMgr.Load(); err != nil {
			log.Printf("Warning: failed to load clients.toml: %v", err)
		}
	}
	stopClients := make(chan struct{})
	clientsDone := make(chan struct{})
	go clientsMgr.StartPersistLoop(30*time.Second, stopClients, clientsDone)

	// Auto-discover LAN clients from Clash /connections
	stopDiscovery := make(chan struct{})
	go runClientDiscovery(clientsMgr, resolvedClashAddr, resolvedClashSecret, stopDiscovery)

	// Open traffic history store (next to settings file)
	var trafficStore *traffic.Store
	if sp := settingsMgr.GetPath(); sp != "" {
		trafficPath := filepath.Join(filepath.Dir(sp), "traffic.db")
		if ts, err := traffic.OpenStore(trafficPath); err != nil {
			log.Printf("Warning: traffic store unavailable: %v", err)
		} else {
			trafficStore = ts
		}
	}

	// Start traffic sampler (no-op if trafficStore is nil)
	stopSampler := make(chan struct{})
	if trafficStore != nil {
		sampler := traffic.NewSampler(trafficStore)
		go sampler.Run(resolvedClashAddr, resolvedClashSecret, 35, stopSampler)
	}

	// Per-user StatsService sampler (v2ray_api). No-op without a store; graceful
	// (no crash / no log spam) if the running binary lacks with_v2ray_api or the
	// addr is unreachable — counters simply stay flat. The addr is resolved from
	// settings (loopback gRPC StatsService listen), defaulting to 127.0.0.1:8081.
	v2rayAPIAddr := cfg.Singbox.V2RayAPI
	if v2rayAPIAddr == "" {
		v2rayAPIAddr = "127.0.0.1:8081"
	}
	stopUserSampler := make(chan struct{})
	if trafficStore != nil {
		if client, err := v2stats.Dial(v2rayAPIAddr); err != nil {
			log.Printf("Warning: v2ray_api dial %s failed: %v", v2rayAPIAddr, err)
		} else {
			userSampler := traffic.NewUserSampler(trafficStore)
			go func() {
				userSampler.Run(client, 30, 35, stopUserSampler)
				client.Close()
			}()
		}
	}

	// Updates: GitHub release checker + daily auto-check (Task 4 adds API)
	updChecker := updates.NewChecker()
	updUpdater := updates.NewUpdater()
	updTargets := []updates.Target{
		updates.AmneziaTarget(procMgr.GetBinaryPath, procMgr.GetVersion, func() error {
			return procMgr.Restart(cfgMgr.GetPath())
		}, amneziaPreflight(cfgMgr, settingsMgr)),
		updates.RouteBoxTarget(Version),
	}
	stopUpdates := make(chan struct{})
	go updates.RunDailyChecks(updChecker, updTargets, func() bool {
		return settingsMgr.Get().Updates.AutoCheck
	}, 24*time.Hour, stopUpdates)

	// Subscriptions: TOML store + hourly auto-refresh of due subscriptions.
	var subsPath string
	if sp := settingsMgr.GetPath(); sp != "" {
		subsPath = filepath.Join(filepath.Dir(sp), "subscriptions.toml")
	}
	subsMgr := subscriptions.NewManager(subsPath)
	if subsPath != "" {
		if err := subsMgr.Load(); err != nil {
			log.Printf("Warning: failed to load subscriptions.toml: %v", err)
		}
	}
	subsRefresh := func(s subscriptions.Subscription) (int, int, error) {
		return subscriptions.Refresh(s, cfgMgr)
	}
	stopSubs := make(chan struct{})
	go subscriptions.RunRefreshLoop(subsMgr, cfgMgr, subscriptions.Refresh, time.Hour, stopSubs)

	// Panel users: sidecar registry mirroring the active config.
	var usersPath string
	if sp := settingsMgr.GetPath(); sp != "" {
		usersPath = filepath.Join(filepath.Dir(sp), "users.toml")
	}
	usersMgr := users.NewManager(usersPath)
	if usersPath != "" {
		if err := usersMgr.Load(); err != nil {
			log.Printf("Warning: failed to load users.toml: %v", err)
		}
	}
	// Startup reconcile against the active config (additive: empty active -> no-op).
	if _, err := usersMgr.Reconcile(cfgMgr.GetActive()); err != nil {
		log.Printf("Warning: startup users reconcile failed: %v", err)
	}

	// Initialize API handlers
	apiHandler := api.NewHandler(cfgMgr, procMgr, resolvedClashAddr, geoipDB, settingsMgr, clientsMgr, trafficStore)
	apiHandler.SetRouteBoxVersion(Version)
	apiHandler.SetUpdatesService(&updates.Service{
		Checker: updChecker,
		Updater: updUpdater,
		Targets: updTargets,
	})
	// ROUTEBOX_RUNTIME=docker is set by the official Dockerfile; it means
	// RouteBox's own binary lives in the image, not in a writable install
	// directory a self-update should touch.
	apiHandler.SetPanelMode(effectiveMode)
	apiHandler.SetDockerMode(dockerRuntime())
	apiHandler.SetSubscriptions(subsMgr, subsRefresh)
	apiHandler.SetUsers(usersMgr)

	// AmneziaWG server-interface manager: secrets + .conf live under
	// /etc/amnezia/amneziawg — this is where awg-tools' `awg-quick@<iface>` systemd
	// template reads its config from (ExecStart `awg-quick up %i` resolves the
	// bare iface name to /etc/amnezia/amneziawg/<iface>.conf). Writing elsewhere
	// makes the interface fail to come up. (It is NOT under /etc/amnezia-box, so
	// config.json backup pruning can never glob it.) Interface values from settings.awg.
	awgSettings := settingsMgr.Get().Awg
	awgMgr := awg.NewManager(awg.NewExecRunner(), "/etc/amnezia/amneziawg", awg.Config{
		Iface:      awgSettings.Interface,
		Subnet:     awgSettings.Subnet,
		ListenPort: awgSettings.ListenPort,
		MTU:        awgSettings.MTU,
		DNS:        awgSettings.DNS,
		WANIface:   awgSettings.WANIface,
		PublicHost: settingsMgr.Get().Server.PublicHost,
	})
	if err := awgMgr.Store().Load(); err != nil {
		log.Printf("Warning: failed to load amneziawg peers.toml: %v", err)
	}
	// awgDesired maps the persisted settings to the orchestrator input (used by both
	// ConfigDirty's live getter and the boot-time Rehydrate).
	awgDesired := func() awg.EnableInput {
		return awg.EnableInputFromSettings(settingsMgr.Get().Awg)
	}
	// Status.ConfigDirty compares the running config against the live saved settings.
	awgMgr.SetDesired(awgDesired)
	// Peer liveness on the singbox backend. There is no interface to ask for a
	// handshake there, so the roster reads "when did this tunnel IP last move
	// bytes" out of the same traffic history the per-peer byte counts come from.
	// Without a store (monitoring off) it stays nil and peers read offline, which
	// is what they did before.
	if trafficStore != nil {
		awgMgr.SetPeerLiveness(func(since int64) map[string]int64 {
			seen, err := trafficStore.LastSeenBySource(since)
			if err != nil {
				log.Printf("awg: peer liveness lookup failed: %v", err)
				return nil
			}
			return seen
		})
	}
	// Real per-peer handshake/tx/rx on the singbox backend, straight from the
	// WireGuard device's own UAPI state (amnezia-box's GET /awg/{tag}/peers) —
	// SetPeerLiveness above only runs as a fallback when this errors with
	// errAwgPeerStatsUnsupported (an amnezia-box binary older than that route).
	if resolvedClashAddr != "" {
		awgMgr.SetPeerStats(func() (map[string]awg.PeerStat, error) {
			return awg.FetchAwgPeerStats(resolvedClashAddr, resolvedClashSecret, config.ManagedAwgServerTag)
		})
	}
	// Resolve the AWG backend: explicit setting wins; otherwise default to singbox
	// (no kernel module required). Kernel is opt-in only — a router/VPS never runs
	// the kernel-module install path unless the operator explicitly selects it.
	// MUST run before the sweep ticker + HTTP server start so no goroutine ever
	// observes a half-wired Manager.
	// An unset setting keeps kernel when a rendered .conf shows this is a pre-0.23
	// install that predates the setting — flipping those to singbox tore down a
	// live tunnel and re-keyed the server (issue #43).
	awgBackend := awgMgr.ResolveBackend(settingsMgr.Get().Awg.Backend)
	// The API refuses to switch to a kernel backend this system cannot run; a
	// settings file written before the tools went missing (or edited by hand)
	// can still ask for it, and it would otherwise surface as a failing
	// systemctl at Enable rather than as a missing prerequisite.
	if awgBackend == "kernel" {
		if reason := awg.KernelBackendUnsupported(); reason != "" {
			log.Printf("WARNING: awg.backend is \"kernel\" but %s. The AmneziaWG server will fail to come up; set awg.backend = \"singbox\" in %s, or install the missing pieces.", reason, settingsMgr.GetPath())
		}
	}
	awgMgr.SetBackend(awgBackend)
	awgMgr.SetConfigSync(
		cfgMgr, // *config.Manager satisfies awg.ConfigSyncer
		func() error { return applyAwgReload(cfgMgr, procMgr) }, // reload after change
		procMgr.SupportsAWGServer,
	)
	// awg3 capability gate: cpa/rat/header_protection_key are emitted only when the
	// running binary accepts them (additivity — old binaries reject unknown fields).
	awgMgr.SetSupportsAWG3(procMgr.SupportsAWG3)
	// Kernel backend's own awg3 gate: independent of the sing-box binary above —
	// it checks the loaded amneziawg module + awg-quick/tools instead.
	awgMgr.SetKernelSupportsAWG3(awg.KernelSupportsAWG3)
	awgMgr.SetKernelSupportsAWG31(awg.KernelSupportsAWG31)
	// Warm the Manager so client-config rendering works after a restart without a
	// re-enable: singbox restores serverPriv/obf from settings + the store server key
	// (no awg-quick); kernel reads the persisted .conf (iface keeps running via systemd).
	if awgBackend == "singbox" {
		// enabled comes from the PERSISTED flag (Enable/Disable handlers write it),
		// not from key existence — Disable must survive a RouteBox restart (Bug C1).
		awgMgr.RehydrateSingbox(awgDesired(), settingsMgr.Get().Awg.Enabled)
	} else {
		awgMgr.Rehydrate(context.Background(), awgDesired())
	}
	// Heal leftovers of the INACTIVE backend: a switch on an older build (or a
	// crash mid-switch) can leave awg-quick@<iface> enabled/running while the
	// backend is singbox — a live, panel-invisible kernel tunnel — or an orphaned
	// managed endpoint in the active config while the backend is kernel.
	// Kernel backend without systemd (a container, or a host without it): no unit
	// holds boot persistence, so the interface has to be brought back here or the
	// panel comes up reporting a server that is enabled with nothing behind it.
	// A no-op wherever systemd owns the interface.
	if awgBackend == "kernel" {
		if err := awgMgr.RestoreKernelIface(context.Background(), settingsMgr.Get().Awg.Enabled); err != nil {
			log.Printf("WARNING: %v — the AmneziaWG server is enabled in settings but its interface is not up; the panel's Enable will report the same failure", err)
		}
	}
	awgMgr.ReconcileBackendResidue(context.Background())
	apiHandler.SetAWG(awgMgr)

	// AWG expiry sweep — tied to AWG's lifecycle, NOT the vps-only mode gate.
	// SweepExpired is a cheap no-op when the interface is down.
	stopAwgSweep := make(chan struct{})
	go awg.RunSweepLoop(func() { awgMgr.SweepExpired(context.Background()) }, 30*time.Second, stopAwgSweep)

	// Telegram MTProto proxy (vps mode). Client secrets sit beside users.toml,
	// the directory every other RouteBox-owned credential file lives in.
	var mtprotoPath string
	if sp := settingsMgr.GetPath(); sp != "" {
		mtprotoPath = filepath.Join(filepath.Dir(sp), "mtproto.toml")
	}
	mtprotoStore := mtproto.NewStore(mtprotoPath)
	if mtprotoPath != "" {
		if err := mtprotoStore.Load(); err != nil {
			log.Printf("Warning: failed to load mtproto.toml: %v", err)
		}
	}
	mtprotoMgr := mtproto.NewManager(mtprotoStore)
	apiHandler.SetMtproto(mtprotoMgr)

	mtprotoConfig := func() mtproto.Config {
		s := settingsMgr.Get().Mtproto
		return mtproto.Config{
			Listen:             s.Listen,
			MaskingDomain:      s.MaskingDomain,
			Concurrency:        uint(s.Concurrency),
			IdleTimeout:        time.Duration(s.IdleTimeoutSec) * time.Second,
			PreferIP:           s.PreferIP,
			DomainFrontingPort: uint(s.DomainFrontingPort),
			SocksProxy:         mtproto.SocksProxyAddr(s.Outbound, s.SocksPort),
		}
	}

	// Reconcile the managed SOCKS inbound before anything uses it. Ahead of the
	// amnezia-box auto-start below, so the process comes up on the synced config
	// rather than needing a reload it has not been asked for yet.
	apiHandler.SyncMtprotoSocksOnStart()

	if effectiveMode == "vps" && settingsMgr.Get().Mtproto.Enabled {
		// Not fatal: the panel has to come up for an operator to fix whatever
		// setting caused this — a busy port or a roster emptied by expiry.
		if err := mtprotoMgr.Start(mtprotoConfig()); err != nil {
			log.Printf("WARNING: the Telegram proxy did not start: %v", err)
		}
	}

	// Both loops run regardless of whether the proxy is up: the flusher reads a
	// stream that only exists after Start, and the sweep is a no-op on an empty
	// roster. Starting them here means a proxy enabled later is covered without
	// a second wiring path.
	stopMtproto := make(chan struct{})
	mtprotoFlushDone := make(chan struct{})
	go func() {
		defer close(mtprotoFlushDone)
		mtproto.RunFlushLoop(mtprotoMgr, trafficStore, time.Minute, stopMtproto)
	}()
	go mtproto.RunExpiryLoop(mtprotoStore, mtprotoMgr.Rebuild, 30*time.Second, stopMtproto)

	// Bring amnezia-box up when nothing else will. Done here rather than before
	// the AWG wiring above so the process starts on the config those steps may
	// have just synced, and before SyncRejectRuleAndReload so user lifecycle
	// enforcement lands on a running process instead of waiting for a reload.
	if shouldAutoStartAmneziaBox(settingsMgr.Get().Singbox.Autostart, procMgr.GetStatus(), fileExists(resolvedConfigPath), procMgr.IsBinaryInstalled()) {
		if err := procMgr.Start(resolvedConfigPath); err != nil {
			log.Printf("amnezia-box did not start automatically: %v — the panel's Start button reports the same failure with the logs", err)
		} else {
			fmt.Printf("Started amnezia-box with %s\n", resolvedConfigPath)
		}
	}

	// Phase 4: warn (don't block) when pre-existing panel users share a name.
	// auth_user matches by name, so duplicates over-block during lifecycle reject.
	if dups := users.DuplicateNames(usersMgr.List()); len(dups) > 0 {
		log.Printf("WARNING: duplicate panel user names detected (%v) — lifecycle reject matches by name, so disabling/expiring one of a duplicate pair over-blocks the other; rename to disambiguate", dups)
	}

	// Phase 4: establish lifecycle enforcement (disabled/expired users) in the
	// active config once at startup so a restart re-applies the reject rule. If the
	// process is not yet running, the reload is skipped (not-running) and it picks
	// up the synced config on its own start.
	apiHandler.SyncRejectRuleAndReload()

	// Expiry ticker (vps mode only — router mode has no panel users). 30s
	// granularity; cheap (one List + a deep-equal). Re-applies enforcement so users
	// expire with no admin action. Defers internally when a draft is pending.
	stopExpiry := make(chan struct{})
	if effectiveMode == "vps" {
		go users.RunExpiryLoop(apiHandler.SyncRejectRuleAndReload, 30*time.Second, stopExpiry)
	}

	// Dedicated per-IP rate-limiter for the PUBLIC /sub/{token} endpoint, kept
	// separate from the auth/lockout limiter. Keyed on clientIP(r) = RemoteAddr.
	subLimiter := auth.NewLimiter()
	apiHandler.SetSubLimiter(subLimiter)

	// Construct auth deps, wire onto handler, start cleanup ticker
	sessions := auth.NewSessionStore()
	limiter := auth.NewLimiter()
	verifier := auth.NewCachedVerifier()
	apiHandler.SetAuth(sessions, limiter, verifier)
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for range t.C {
			sessions.Cleanup()
			limiter.Cleanup()
			subLimiter.Cleanup()
		}
	}()

	// Setup router
	r := chi.NewRouter()

	// Middleware
	// Root access log with /sub/<token> scrubbing — the subscription token is a
	// credential and must NEVER reach the log (chi's middleware.Logger formats its
	// line from r.RequestURI, which would leak it). All other paths log verbatim.
	r.Use(api.SubTokenScrubber(log.Default()))
	r.Use(middleware.Recoverer)
	if cfg.Network.CompressionEnabled {
		r.Use(middleware.Compress(5))
	}
	if cfg.Security.CorsOrigins != "" {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   []string{cfg.Security.CorsOrigins},
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Content-Type"},
			AllowCredentials: false,
			MaxAge:           300,
		}))
	}

	// API routes: public group + protected group
	r.Route("/api", func(r chi.Router) {
		// PUBLIC: reachable without auth
		r.Group(func(r chi.Router) {
			r.Post("/auth/login", apiHandler.Login)
			r.Post("/auth/logout", apiHandler.Logout)
			r.Get("/auth/session", apiHandler.Session)
			r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status":"ok"}`))
			})
		})
		// PROTECTED: everything else
		r.Group(func(r chi.Router) {
			r.Use(api.AuthMiddleware(settingsMgr, sessions, limiter, verifier))

			// Config (full)
			r.Get("/config", apiHandler.GetConfig)
			r.Put("/config", apiHandler.SaveConfig)
			r.Post("/config/validate", apiHandler.ValidateConfig)
			r.Post("/config/diff", apiHandler.GetConfigDiff)
			r.Post("/config/apply", apiHandler.ApplyConfig)
			r.Post("/config/fix-unit", apiHandler.FixUnitConfigPath)
			// Обратная операция к fix-unit: снять drop-in, которым мы
			// перенацелили юнит. Без неё единственная запись RouteBox вне
			// своих файлов была односторонней.
			r.Delete("/config/unit-dropin", apiHandler.RemoveUnitConfigDropIn)
			r.Get("/config/export", apiHandler.ExportConfig)
			r.Post("/config/import", apiHandler.ImportConfig)

			// Config draft system
			r.Get("/config/status", apiHandler.GetConfigStatus)
			r.Post("/config/discard", apiHandler.DiscardConfig)
			r.Get("/config/draft-diff", apiHandler.GetDraftDiff)
			r.Post("/config/save", apiHandler.SaveConfigDraft)
			r.Post("/config/check", apiHandler.CheckConfig)
			r.Get("/config/active", apiHandler.GetActiveConfig)

			// Endpoints CRUD
			r.Route("/endpoints", func(r chi.Router) {
				r.Get("/", apiHandler.ListEndpoints)
				r.Post("/", apiHandler.CreateEndpoint)
				r.Get("/{tag}", apiHandler.GetEndpoint)
				r.Put("/{tag}", apiHandler.UpdateEndpoint)
				r.Delete("/{tag}", apiHandler.DeleteEndpoint)
				// Same measurement as an outbound's (#92): the tool resolves a tag
				// through the outbound manager, which falls back to endpoints, and
				// for AWG/WireGuard the endpoint IS the outbound.
				r.Post("/{tag}/speedtest", apiHandler.SpeedTestOutbound)
			})

			// Outbounds CRUD
			r.Route("/outbounds", func(r chi.Router) {
				r.Get("/", apiHandler.ListOutbounds)
				r.Post("/", apiHandler.CreateOutbound)
				r.Get("/{tag}", apiHandler.GetOutbound)
				r.Put("/{tag}", apiHandler.UpdateOutbound)
				r.Delete("/{tag}", apiHandler.DeleteOutbound)
				// Measured through the binary's own networkquality tool (#13).
				r.Post("/{tag}/speedtest", apiHandler.SpeedTestOutbound)
			})

			// Inbounds CRUD
			r.Route("/inbounds", func(r chi.Router) {
				r.Get("/", apiHandler.ListInbounds)
				r.Post("/", apiHandler.CreateInbound)
				r.Get("/{tag}", apiHandler.GetInbound)
				r.Put("/{tag}", apiHandler.UpdateInbound)
				r.Delete("/{tag}", apiHandler.DeleteInbound)
			})

			// Credential generators (server inbounds)
			r.Route("/generate", func(r chi.Router) {
				r.Post("/reality", apiHandler.GenerateReality)
				r.Post("/uuid", apiHandler.GenerateUUID)
				r.Post("/password", apiHandler.GeneratePassword)
			})

			// Route Rule Sets CRUD
			r.Route("/route/rule-sets", func(r chi.Router) {
				r.Get("/", apiHandler.ListRuleSets)
				r.Post("/", apiHandler.CreateRuleSet)
				r.Get("/usage", apiHandler.GetRuleSetsUsage)
				r.Put("/{tag}", apiHandler.UpdateRuleSet)
				r.Delete("/{tag}", apiHandler.DeleteRuleSet)
			})

			// Domain Sets (custom rule set sources)
			r.Route("/domains", func(r chi.Router) {
				r.Get("/", apiHandler.ListDomainSets)
				r.Post("/", apiHandler.CreateDomainSet)
				r.Route("/{tag}", func(r chi.Router) {
					r.Get("/", apiHandler.GetDomainSet)
					r.Put("/", apiHandler.SaveDomainSet)
					r.Delete("/", apiHandler.DeleteDomainSet)
					r.Post("/domain", apiHandler.AddDomain)
					r.Delete("/domain/{domain}", apiHandler.RemoveDomain)
					r.Post("/import", apiHandler.ImportDomains)
				})
			})

			// Clients (LAN device names)
			r.Route("/clients", func(r chi.Router) {
				r.Get("/", apiHandler.ListClients)
				r.Put("/{ip}", apiHandler.UpdateClient)
				r.Delete("/{ip}", apiHandler.DeleteClient)
			})

			// Subscriptions CRUD + refresh
			r.Route("/subscriptions", func(r chi.Router) {
				r.Get("/", apiHandler.ListSubscriptions)
				r.Post("/", apiHandler.CreateSubscription)
				r.Put("/{id}", apiHandler.UpdateSubscription)
				r.Delete("/{id}", apiHandler.DeleteSubscription)
				r.Post("/{id}/refresh", apiHandler.RefreshSubscription)
			})

			// The naive node dest serves in an out-of-the-box install. Not an
			// inbound, so it cannot come from /inbounds — and without it the
			// panel shows four protocols on a server that runs five.
			r.Get("/dest/naive", apiHandler.GetDestNaive)

			// Panel users CRUD + share-by-id
			r.Route("/users", func(r chi.Router) {
				r.Get("/", apiHandler.ListUsers)
				r.Post("/", apiHandler.CreateUser)
				r.Patch("/{id}", apiHandler.UpdateUser)
				r.Delete("/{id}", apiHandler.DeleteUser)
				r.Post("/{id}/bindings", apiHandler.AddBinding)
				r.Get("/{id}/link", apiHandler.GetUserLinkByID)
				r.Get("/{id}/traffic", apiHandler.GetUserTraffic)
				r.Post("/{id}/token/rotate", apiHandler.RotateUserToken)
				r.Delete("/{id}/token", apiHandler.RevokeUserToken)
			})

			// AmneziaWG server interface (vps mode). Inside the auth group: the
			// SameSite=Strict cookie + AuthMiddleware is the CSRF defense for the
			// mutating POST/DELETE routes.
			r.Route("/awg", func(r chi.Router) {
				r.Get("/status", apiHandler.GetAWGStatus)
				r.Post("/enable", apiHandler.EnableAWG)
				r.Post("/disable", apiHandler.DisableAWG)
				r.Get("/peers", apiHandler.ListAWGPeers)
				r.Post("/peers", apiHandler.CreateAWGPeer)
				// Static segment, so chi matches it before /peers/{publicKey}/...
				r.Get("/peers/traffic", apiHandler.GetAWGPeersTraffic)
				r.Delete("/peers/{publicKey}", apiHandler.DeleteAWGPeer)
				r.Get("/peers/{publicKey}/config", apiHandler.GetAWGPeerConfig)
				r.Get("/peers/{publicKey}/vpn-link", apiHandler.GetAWGPeerVPNLink)
				r.Get("/peers/{publicKey}/singbox", apiHandler.GetAWGPeerSingbox)
				r.Patch("/peers/{publicKey}/expiry", apiHandler.SetAWGPeerExpiry)
				// Server backup/restore (#97): the store + awg settings as one JSON.
				r.Get("/backup", apiHandler.GetAWGBackup)
				r.Post("/restore", apiHandler.RestoreAWGBackup)
			})

			// Telegram MTProto proxy (vps mode). Inside the auth group, like
			// /awg: the SameSite=Strict cookie plus AuthMiddleware is the CSRF
			// defense for the mutating routes.
			r.Route("/mtproto", func(r chi.Router) {
				r.Get("/", apiHandler.GetMtprotoStatus)
				r.Put("/", apiHandler.UpdateMtprotoSettings)
				r.Post("/enable", apiHandler.EnableMtproto)
				r.Post("/disable", apiHandler.DisableMtproto)
				r.Get("/connections", apiHandler.GetMtprotoConnections)
				// WebSocket: the proxy's own log. Not part of amnezia-box, so
				// the Clash log stream never carries it.
				r.Get("/logs", apiHandler.StreamMtprotoLogs)
				r.Get("/clients", apiHandler.ListMtprotoClients)
				r.Post("/clients", apiHandler.CreateMtprotoClient)
				// Static segment, so chi matches it before /clients/{name} —
				// "traffic" is a legal client name.
				r.Get("/clients/traffic", apiHandler.GetMtprotoClientsTraffic)
				r.Delete("/clients/{name}", apiHandler.DeleteMtprotoClient)
				r.Patch("/clients/{name}", apiHandler.UpdateMtprotoClient)
				r.Get("/clients/{name}/link", apiHandler.GetMtprotoClientLink)
				r.Post("/clients/{name}/rotate", apiHandler.RotateMtprotoClient)
			})

			// Route Rules CRUD
			r.Route("/route/rules", func(r chi.Router) {
				r.Get("/", apiHandler.ListRules)
				r.Post("/", apiHandler.CreateRule)
				r.Put("/{index}", apiHandler.UpdateRule)
				r.Delete("/{index}", apiHandler.DeleteRule)
				r.Put("/reorder", apiHandler.ReorderRules)
			})

			// Route Settings
			r.Get("/route/settings", apiHandler.GetRouteSettings)
			r.Put("/route/settings", apiHandler.UpdateRouteSettings)

			// Route Inspector
			r.Post("/route/test", apiHandler.TestRoute)

			// Connection Test (diagnostics)
			r.Post("/diagnostics/connect", apiHandler.ConnectTest)

			// Traffic history (SQLite-backed)
			r.Get("/traffic/history", apiHandler.GetTrafficHistory)
			r.Post("/traffic/reset", apiHandler.ResetTrafficHistory)

			// DNS Servers CRUD
			r.Route("/dns/servers", func(r chi.Router) {
				r.Get("/", apiHandler.ListDnsServers)
				r.Post("/", apiHandler.CreateDnsServer)
				r.Put("/{tag}", apiHandler.UpdateDnsServer)
				r.Delete("/{tag}", apiHandler.DeleteDnsServer)
			})

			// DNS Rules CRUD
			r.Route("/dns/rules", func(r chi.Router) {
				r.Get("/", apiHandler.ListDnsRules)
				r.Post("/", apiHandler.CreateDnsRule)
				r.Put("/{index}", apiHandler.UpdateDnsRule)
				r.Delete("/{index}", apiHandler.DeleteDnsRule)
				r.Put("/reorder", apiHandler.ReorderDnsRules)
			})

			// DNS Settings
			r.Get("/dns/settings", apiHandler.GetDnsSettings)
			r.Put("/dns/settings", apiHandler.UpdateDnsSettings)

			// Log Settings
			r.Get("/log", apiHandler.GetLogSettings)
			r.Put("/log", apiHandler.UpdateLogSettings)

			// Experimental Settings
			r.Get("/experimental", apiHandler.GetExperimental)
			r.Put("/experimental", apiHandler.UpdateExperimental)

			// Version & Feature Flags
			r.Get("/version", apiHandler.GetVersion)

			// Binary updates (amnezia-box + RouteBox self-update)
			r.Route("/updates", func(r chi.Router) {
				r.Get("/status", apiHandler.GetUpdatesStatus)
				r.Post("/check", apiHandler.CheckUpdates)
				r.Post("/apply", apiHandler.ApplyUpdate)
				r.Get("/progress", apiHandler.GetUpdatesProgress)
			})

			// Status & Control
			r.Get("/status", apiHandler.GetStatus)
			r.Post("/control/start", apiHandler.Start)
			r.Post("/control/stop", apiHandler.Stop)
			r.Post("/control/restart", apiHandler.Restart)
			r.Post("/control/reload", apiHandler.Reload)

			// Расхождение путей конфига целиком живёт в config_paths статуса,
			// поэтому отдельного «а какой путь обнаружен» больше нет — узнать
			// одно и то же двумя способами было ровно тем, из-за чего панель
			// показывала два разных предупреждения об одном и том же.
			r.Post("/config/adopt-unit-path", apiHandler.AdoptUnitConfigPath)

			// Systemd logs
			r.Get("/logs/journal", apiHandler.GetJournalLogs)

			// Clash API proxy - WebSocket routes MUST be registered before the wildcard
			r.HandleFunc("/clash/traffic", apiHandler.ProxyClashWebSocket)
			r.HandleFunc("/clash/logs", apiHandler.ProxyClashWebSocket)
			r.HandleFunc("/clash/connections", apiHandler.ProxyClashWebSocket)
			r.Get("/clash/*", apiHandler.ProxyClashAPI)
			r.Delete("/clash/*", apiHandler.ProxyClashAPI)

			// Panel auth — change password (session-protected; current pw verified)
			r.Post("/auth/change-password", apiHandler.ChangePassword)

			// RouteBox Settings API
			r.Get("/settings", apiHandler.GetSettings)
			r.Put("/settings", apiHandler.UpdateSettings)
			r.Post("/settings/reload", apiHandler.ReloadSettings)

			// Setup wizard
			r.Get("/needs-setup", apiHandler.NeedsSetup)
		})
	})

	// PUBLIC per-user subscription — registered OUTSIDE the /api auth group so
	// clients (which have no panel credentials) can fetch it. Anti-enumeration,
	// per-IP rate-limit, the 503-no-host policy, and log scrubbing are enforced in
	// the handler / root middleware. Sibling of /api and the SPA wildcard; chi's
	// longest-prefix match makes this win over /*.
	r.Get("/sub/{token}", apiHandler.GetSubscription)

	// Serve embedded frontend (SPA)
	r.Get("/*", embedded.Handler())

	// VPS-mode bootstrap (force-auth) + router-mode warning
	if effectiveMode == "vps" {
		if err := bootstrapVPSAuth(settingsMgr, os.Stdout); err != nil {
			log.Fatalf("%v", err)
		}
	} else if isNonLoopback(resolvedListenAddr) && !settingsMgr.Get().Security.AuthEnabled {
		log.Printf("WARNING: RouteBox is listening on a non-loopback address (%s) with auth DISABLED — the panel is open to the network. Enable security.auth_enabled or run with --mode=vps.", resolvedListenAddr)
	}

	// Resolve TLS strategy (acme → manual → off). A misconfigured acme
	// (enabled without a public_host) is fatal: we cannot issue a cert and
	// would otherwise loop on issuance under systemd Restart=on-failure.
	// cfg (captured before the VPS bootstrap, which mutates only [security])
	// still holds the current Network/Server values.
	tlsM, tlsErr := resolveTLSMode(cfg)
	if tlsErr != nil {
		log.Fatalf("TLS configuration: %v", tlsErr)
	}
	certPath := cfg.Network.TLSCertPath
	keyPath := cfg.Network.TLSKeyPath
	scheme := "http"
	tlsLabel := "off"
	switch tlsM {
	case tlsModeACME:
		scheme, tlsLabel = "https", "acme"
	case tlsModeManual:
		scheme, tlsLabel = "https", "manual"
	}

	// Start server
	fmt.Println()
	fmt.Printf("RouteBox %s starting on %s://%s (TLS: %s)\n", Version, scheme, resolvedListenAddr, tlsLabel)
	if cfg.Server.PublicHost != "" {
		publicURL := scheme + "://" + cfg.Server.PublicHost
		if cfg.Server.PublicPort != 0 && cfg.Server.PublicPort != 443 {
			publicURL = fmt.Sprintf("%s:%d", publicURL, cfg.Server.PublicPort)
		}
		fmt.Printf("Public URL: %s\n", publicURL)
	}
	fmt.Printf("Config: %s\n", resolvedConfigPath)
	if resolvedClashAddr != "" {
		fmt.Printf("Clash API: %s\n", resolvedClashAddr)
	}
	if !procMgr.IsBinaryInstalled() {
		// The wizard is a router-mode thing; a panel (and a container in
		// particular) has no wizard to send anyone to, and the useful fact there
		// is which path was looked at.
		if effectiveMode == "vps" {
			fmt.Printf("Note: no working amnezia-box binary at %s — the panel can manage the config but not the process\n", procMgr.GetBinaryPath())
		} else {
			fmt.Println("Note: amnezia-box not installed - run setup wizard to configure")
		}
	}
	fmt.Println()

	// WriteTimeout is intentionally left at 0 (unlimited) because the Clash
	// WebSocket proxy endpoints (/api/clash/traffic, /clash/logs, /clash/connections)
	// are long-lived streaming connections. A non-zero http.Server.WriteTimeout
	// applies to all handlers globally and would terminate those streams after the
	// deadline, breaking real-time monitoring. ReadTimeout is safe to set because
	// it only covers the time to read the initial request headers/body.
	srv := &http.Server{
		Addr:              resolvedListenAddr,
		Handler:           r,
		ReadTimeout:       time.Duration(cfg.Network.ReadTimeoutSec) * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	// errCh has TWO senders in acme mode (the :80 ACME-challenge listener
	// goroutine and the TLS serve), so size the buffer to 2 to avoid leaking a
	// blocked sender after the main select consumes the first error on shutdown.
	errCh := make(chan error, 2)
	switch tlsM {
	case tlsModeACME:
		// guaranteed non-empty by resolveTLSMode. Re-sanitize the loaded value:
		// SanitizePublicHost only runs on the write path, so a hand-edited TOML
		// could smuggle a scheme/port that would corrupt the HostWhitelist match.
		domain := cfg.Server.PublicHost
		if clean, err := settings.SanitizePublicHost(domain); err == nil && clean != "" {
			domain = clean
		}
		// Default an empty cache dir so autocert.DirCache("") never caches certs
		// (incl. the account key) in the process CWD.
		cacheDir := cfg.Network.ACMECacheDir
		if cacheDir == "" {
			cacheDir = "/etc/routebox/acme"
		}
		am := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(domain), // only our own domain (anti-abuse) — mandatory
			Cache:      autocert.DirCache(cacheDir),
			Email:      cfg.Network.ACMEEmail,
		}
		if url := acmeDirectoryURL(cfg.Network.ACMEStaging); url != "" {
			am.Client = &acme.Client{DirectoryURL: url}
		}
		// Keep the panel cert mirrored to the canonical PEM path so server inbounds can
		// reuse it; on renewal this SIGHUP-reloads amnezia-box to pick up the new cert.
		go panelcert.Refresh(context.Background(), cacheDir, domain, panelCertDir, procMgr.Reload)
		// HTTP-01 challenge listener, ":80" by default. TLS-ALPN-01 is impossible
		// (LE would connect on :443 = vless+Reality). Port 80 must stay reachable
		// from the internet for renewals too — in Docker that's normally done by
		// mapping the host's :80 to this listener's (possibly unprivileged) port,
		// rather than changing the listener's own bind address.
		acmeHTTPAddr := cfg.Network.ACMEHTTPAddr
		if acmeHTTPAddr == "" {
			acmeHTTPAddr = ":80"
		}
		go func() {
			errCh <- fmt.Errorf("acme http-01 listener (%s): %w", acmeHTTPAddr, http.ListenAndServe(acmeHTTPAddr, am.HTTPHandler(nil)))
		}()
		srv.TLSConfig = am.TLSConfig()
		go func() {
			errCh <- fmt.Errorf("https listener: %w", srv.ListenAndServeTLS("", "")) // certs from autocert cache
		}()
	case tlsModeManual:
		// Mirror the manual cert into the canonical path so the inbound "use panel cert"
		// option works uniformly. Best-effort.
		if err := panelcert.CopyManual(certPath, keyPath, panelCertDir); err != nil {
			log.Printf("panelcert: copy manual cert: %v", err)
		}
		go func() {
			errCh <- fmt.Errorf("https listener: %w", srv.ListenAndServeTLS(certPath, keyPath)) // manual (Phase 1)
		}()
	default:
		go func() {
			errCh <- fmt.Errorf("http listener: %w", srv.ListenAndServe())
		}()
	}

	select {
	case err := <-errCh:
		log.Fatalf("Server error: %v", err)
	case <-ctx.Done():
	}
	stop() // restore default signal behavior: second Ctrl-C kills immediately

	fmt.Println("\nShutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown: %v", err)
	}

	// Stop background loops, then close the store they write to
	close(stopClients)
	<-clientsDone // final clients.toml flush completes before exit
	close(stopDiscovery)
	close(stopSampler)
	close(stopUserSampler)
	close(stopUpdates)
	close(stopSubs)
	close(stopExpiry)
	close(stopAwgSweep)
	// The proxy stops first so no further traffic events arrive, then the loop
	// is told to make its final flush, and only once that has returned is the
	// store it writes to safe to close.
	if err := mtprotoMgr.Stop(); err != nil {
		log.Printf("mtproto: stop: %v", err)
	}
	close(stopMtproto)
	<-mtprotoFlushDone
	if trafficStore != nil {
		trafficStore.Close()
	}
	if geoipDB != nil {
		if err := geoipDB.Close(); err != nil {
			log.Printf("geoip close: %v", err)
		}
	}
}

// shouldAutoStartAmneziaBox reports whether RouteBox should start amnezia-box
// itself at boot.
//
// With a systemd unit present this is systemd's job — the installer enables the
// unit, and starting the process from here would race it. Without one, RouteBox
// is the only thing that runs at boot (a container, or a hand-rolled install),
// and the panel would otherwise come up with the proxy down until someone
// pressed Start — after every restart, not just the first.
//
// It was vps-only, on the reasoning that a router's config is written by the
// setup wizard, which starts the process as part of itself. That holds for the
// first run and for no reboot after it, which is what #87 asked about — so the
// mode check is now the singbox.autostart setting (default on, switchable per
// install). The guards that made the old rule safe are unchanged: a router that
// never finished the wizard has no config file, and one whose process is
// already running or owned by a unit is left alone.
func shouldAutoStartAmneziaBox(autostart bool, st process.Status, configExists, binaryInstalled bool) bool {
	if !autostart || !configExists || !binaryInstalled {
		return false
	}
	// ServiceName is the DETECTED unit, set even when this particular process
	// was launched by hand — which is exactly the case we must not start into.
	return !st.Running && st.ServiceName == ""
}

// dockerRuntime reports whether this process is the one the official image
// starts. Set by the Dockerfile, so it means the packaged container
// specifically — not "some container somewhere", which nothing here can tell.
func dockerRuntime() bool {
	return os.Getenv("ROUTEBOX_RUNTIME") == "docker"
}

// fileExists reports whether path names an existing file.
func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// generatePassword returns a 24-character URL-safe random password.
// applyAwgReload reloads sing-box after the managed AWG endpoint changed. SIGHUP
// first, restart fallback (mirrors ApplyConfig's mode==reload branch). No-op if
// the process is not running — it picks up the synced config on its own start.
func applyAwgReload(cfg *config.Manager, proc *process.Manager) error {
	if !proc.GetStatus().Running {
		return nil
	}
	if err := proc.Reload(); err != nil {
		return proc.Restart(cfg.GetPath())
	}
	return nil
}

// amneziaPreflight builds the amnezia-box update preflight: it validates the
// EXISTING config against the NEW (downloaded, verified, not-yet-installed)
// binary before the swap. Without it, a config the new build rejects — e.g.
// experimental.v2ray_api against a 1.14+ build that dropped with_v2ray_api —
// fails only at the post-swap restart, and the update rolls back forever with
// an opaque "service restarted but is not running" (a catch-22: the running
// old binary supports the block, so the panel keeps it in the config).
//
// Self-heal: when the check fails AND the new binary lacks with_v2ray_api, the
// RouteBox-owned v2ray_api block is stripped (change-gated; harmless for the
// still-running old binary — the block is optional) and the check retried.
// Anything else the new binary rejects aborts the update BEFORE the swap with
// sing-box's real error message, leaving the running service untouched.
func amneziaPreflight(cfg *config.Manager, sm *settings.Manager) func(string) error {
	return func(newBin string) error {
		cfgPath := cfg.GetPath()
		if cfgPath == "" {
			return nil
		}
		if ok, _ := config.CheckConfigWith(newBin, cfgPath); ok {
			return nil
		}
		if !process.BinarySupportsV2RayAPI(newBin) {
			listen := sm.Get().Singbox.V2RayAPI
			if listen == "" {
				listen = "127.0.0.1:8081"
			}
			changed, err := cfg.SyncV2RayAPI(listen, nil)
			if err != nil {
				return fmt.Errorf("strip experimental.v2ray_api: %w", err)
			}
			if changed {
				log.Printf("updates: stripped experimental.v2ray_api — the new amnezia-box build lacks with_v2ray_api (per-user traffic accounting is unavailable on it)")
			}
		}
		if ok, errs := config.CheckConfigWith(newBin, cfgPath); !ok {
			return fmt.Errorf("the new binary rejects the current config: %s", strings.Join(errs, "; "))
		}
		return nil
	}
}

// initialPasswordFile is the file the generated admin password is left in, next
// to routebox.toml — the place someone bringing a panel up looks, as opposed to
// the journal.
const initialPasswordFile = "routebox-initial-password"

// bootstrapVPSAuth turns panel auth on in vps mode when it is off, generating
// admin credentials and announcing them on out. A no-op when a password is
// already configured.
//
// Two things are written to the settings directory: the settings themselves and
// the password file. Neither is allowed to abort startup.
//
// Falling over would be the wrong trade: RouteBox supports a read-only
// installation as an operating mode — the panel still shows what is running and
// says what to make writable — and a VPS panel that refuses to boot because it
// could not write a file is strictly worse than one that boots with credentials
// it announces but cannot keep. Auth is still switched ON in memory: the mode
// exists because the panel is reachable from the internet, so failing open is
// not on the table.
//
// Silence would be the other wrong trade, and it is the one that used to happen
// for the password file: a bare errno in the middle of the banner. Both writes
// now go through the same classification the rest of RouteBox uses
// (util.ClassifyWriteErr), so an unwritable path is named as read-only with the
// path in it, and each failure is printed with its consequence — the password
// file failing means "this is the only copy", the settings failing means "this
// login lasts until the next start".
func bootstrapVPSAuth(sm *settings.Manager, out io.Writer) error {
	sec := sm.Get().Security
	if sec.AuthEnabled && sec.AuthPasswordHash != "" {
		return nil
	}

	pw, err := generatePassword()
	if err != nil {
		return fmt.Errorf("generate bootstrap password: %w", err)
	}
	user := orDefault(sec.AuthUsername, "admin")
	// In-memory failures stay fatal: they mean the panel would come up
	// unauthenticated on a public address, which is the one outcome vps mode
	// exists to prevent.
	if err := sm.Update(map[string]interface{}{
		"security.auth_enabled":  true,
		"security.auth_username": user,
		"security.auth_password": pw,
	}); err != nil {
		return fmt.Errorf("enable auth: %w", err)
	}

	saveErr := sm.Save()

	// The password file is rewritten on every bootstrap, so it can never hold a
	// password that is no longer the current one: if the settings did not
	// persist, the next start generates a new password and overwrites the file
	// with it.
	pwFile := ""
	var writeErr error
	if settingsPath := sm.GetPath(); settingsPath != "" {
		pwFile = filepath.Join(filepath.Dir(settingsPath), initialPasswordFile)
		writeErr = util.ClassifyWriteErr(pwFile, os.WriteFile(pwFile, []byte(pw+"\n"), 0600))
	}

	fmt.Fprintln(out, "==================================================================")
	fmt.Fprintln(out, " VPS MODE: panel auth was OFF — generated admin credentials")
	fmt.Fprintf(out, "   username: %s\n", user)
	fmt.Fprintf(out, "   password: %s\n", pw)
	switch {
	case pwFile == "":
		fmt.Fprintln(out, "   WARNING: there is no settings file, so the password is not stored anywhere.")
		fmt.Fprintln(out, "            copy it now — it is not printed again.")
	case writeErr != nil:
		fmt.Fprintf(out, "   WARNING: %s could not be written: %v\n", pwFile, writeErr)
		fmt.Fprintln(out, "            copy the password now — this line is the only copy.")
	default:
		fmt.Fprintf(out, "   (also written to %s)\n", pwFile)
	}
	if saveErr != nil {
		fmt.Fprintf(out, "   WARNING: the credentials could not be saved to %s: %v\n", sm.GetPath(), saveErr)
		fmt.Fprintln(out, "            auth is ON for this run only; the next start generates a new password.")
	}
	fmt.Fprintln(out, "==================================================================")
	return nil
}

func generatePassword() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// tlsMode is the resolved TLS termination strategy for the panel.
type tlsMode int

const (
	tlsModeOff    tlsMode = iota // plain HTTP
	tlsModeManual                // ListenAndServeTLS with cert/key paths (Phase 1)
	tlsModeACME                  // embedded Let's Encrypt via autocert
)

// resolveTLSMode decides the TLS strategy from settings, with priority
// acme → manual → off. ACME requires Server.PublicHost (the cert domain);
// enabling it without one would make autocert attempt issuance for an empty
// SNI on every handshake — a guaranteed failure loop that can hit Let's
// Encrypt rate limits — so this returns an error and the caller fails fast.
func resolveTLSMode(cfg settings.Settings) (tlsMode, error) {
	if cfg.Network.ACMEEnabled {
		if cfg.Server.PublicHost == "" {
			return tlsModeOff, errors.New("network.acme_enabled requires server.public_host (the certificate domain) to be set")
		}
		return tlsModeACME, nil
	}
	if cfg.Network.TLSCertPath != "" && cfg.Network.TLSKeyPath != "" {
		return tlsModeManual, nil
	}
	return tlsModeOff, nil
}

// acmeDirectoryURL returns the ACME directory endpoint. Empty string means the
// autocert default (Let's Encrypt production). Staging has lax rate limits and
// an UNTRUSTED chain — dev/e2e only.
func acmeDirectoryURL(staging bool) string {
	if staging {
		return "https://acme-staging-v02.api.letsencrypt.org/directory"
	}
	return ""
}

// orDefault returns s if non-empty, otherwise def.
func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// isNonLoopback reports whether addr resolves to a non-loopback interface.
func isNonLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	return host != "" && host != "127.0.0.1" && host != "::1" && host != "localhost"
}

// runClientDiscovery polls the Clash /connections endpoint every 60s and feeds
// observed source IPs into the clients manager. Exits cleanly when stop closes.
func runClientDiscovery(mgr *clients.Manager, clashAddr, secret string, stop <-chan struct{}) {
	if clashAddr == "" {
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	// lastErr dedupes the non-200 log: only transitions (ok→error, or a
	// different status) are logged, so a persistent 401 (Clash secret changed
	// without a RouteBox restart) produces one line, not one per tick.
	lastErr := ""
	tick := func() {
		req, err := http.NewRequest("GET", "http://"+clashAddr+"/connections", nil)
		if err != nil {
			return
		}
		if secret != "" {
			req.Header.Set("Authorization", "Bearer "+secret)
		}
		resp, err := client.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			// A 401 body must not decode as "zero connections" — skip the tick.
			if msg := fmt.Sprintf("clash /connections: status %d", resp.StatusCode); msg != lastErr {
				log.Printf("client discovery: %s", msg)
				lastErr = msg
			}
			return
		}
		lastErr = ""
		var data struct {
			Connections []struct {
				Metadata struct {
					SourceIP string `json:"sourceIP"`
				} `json:"metadata"`
			} `json:"connections"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			return
		}
		now := time.Now()
		for _, c := range data.Connections {
			// Canonical, not raw: a dual-stack inbound reports an IPv4 client as
			// "::ffff:x", which would become a second client entry for one device (#71).
			if ip := util.CanonicalClientIP(c.Metadata.SourceIP); ip != "" {
				mgr.Observe(ip, now)
			}
		}
	}
	tick()
	for {
		select {
		case <-ticker.C:
			tick()
		case <-stop:
			return
		}
	}
}

// resolveClashSecretFromConfig extracts the Clash API secret from sing-box
// config (experimental.clash_api.secret); "" when absent.
func resolveClashSecretFromConfig(cfgMgr *config.Manager) string {
	cfg := cfgMgr.Get()
	if cfg == nil {
		return ""
	}

	if exp, ok := cfg["experimental"].(map[string]interface{}); ok {
		if clashApi, ok := exp["clash_api"].(map[string]interface{}); ok {
			if secret, ok := clashApi["secret"].(string); ok {
				return secret
			}
		}
	}

	return ""
}

// resolveClashAddrFromConfig extracts Clash API address from sing-box config
func resolveClashAddrFromConfig(cfgMgr *config.Manager) string {
	cfg := cfgMgr.Get()
	if cfg == nil {
		return ""
	}

	// Look for experimental.clash_api.external_controller
	if exp, ok := cfg["experimental"].(map[string]interface{}); ok {
		if clashApi, ok := exp["clash_api"].(map[string]interface{}); ok {
			if controller, ok := clashApi["external_controller"].(string); ok && controller != "" {
				return controller
			}
		}
	}

	return ""
}

// frontedInboundTags lists the inbounds that only a front can reach. Loopback
// binding is the whole marker — serverlinks reads the same fact to decide which
// port a client link carries, so the two agree by construction.
func frontedInboundTags(cfg map[string]interface{}) []string {
	var tags []string
	inbounds, _ := cfg["inbounds"].([]interface{})
	for _, ib := range inbounds {
		obj, ok := ib.(map[string]interface{})
		if !ok {
			continue
		}
		listen, _ := obj["listen"].(string)
		if tag, _ := obj["tag"].(string); tag != "" && util.IsLoopbackListen(listen) {
			tags = append(tags, tag)
		}
	}
	return tags
}
