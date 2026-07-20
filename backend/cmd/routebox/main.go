package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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
	"routebox/backend/internal/panelcert"
	"routebox/backend/internal/process"
	"routebox/backend/internal/settings"
	"routebox/backend/internal/subscriptions"
	"routebox/backend/internal/traffic"
	"routebox/backend/internal/updates"
	"routebox/backend/internal/users"
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
	modeFlag   = flag.String("mode", "", "panel mode: router (default) or vps")
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("routebox version %s\n", Version)
		return
	}
	flag.Parse()

	// Require root privileges (needed for TUN interface)
	if os.Geteuid() != 0 {
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

	// Print settings info
	if settingsMgr.GetPath() != "" {
		fmt.Printf("Settings: %s\n", settingsMgr.GetPath())
	}

	// Initialize process manager
	procMgr := process.NewManager()

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
		}),
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
		s := settingsMgr.Get().Awg
		return awg.EnableInput{
			Subnet: s.Subnet, ListenPort: s.ListenPort, MTU: s.MTU,
			DNS: s.DNS, WANIface: s.WANIface, ObfPreset: s.ObfPreset,
			HeaderProtection: s.HeaderProtection,
			Obf: awg.Obfuscation{
				Jc: s.Obf.Jc, Jmin: s.Obf.Jmin, Jmax: s.Obf.Jmax,
				S1: s.Obf.S1, S2: s.Obf.S2, S3: s.Obf.S3, S4: s.Obf.S4,
				H1: s.Obf.H1, H2: s.Obf.H2, H3: s.Obf.H3, H4: s.Obf.H4,
				CPA: s.Obf.ContentPaddingAddition, RAT: s.Obf.RekeyAfterTime,
			},
		}
	}
	// Status.ConfigDirty compares the running config against the live saved settings.
	awgMgr.SetDesired(awgDesired)
	// Resolve the AWG backend: explicit setting wins; otherwise default to singbox
	// (no kernel module required). Kernel is opt-in only — a router/VPS never runs
	// the kernel-module install path unless the operator explicitly selects it.
	// MUST run before the sweep ticker + HTTP server start so no goroutine ever
	// observes a half-wired Manager.
	awgBackend := settingsMgr.Get().Awg.Backend
	if awgBackend == "" {
		awgBackend = "singbox"
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
	apiHandler.SetAWG(awgMgr)

	// AWG expiry sweep — tied to AWG's lifecycle, NOT the vps-only mode gate.
	// SweepExpired is a cheap no-op when the interface is down.
	stopAwgSweep := make(chan struct{})
	go awg.RunSweepLoop(func() { awgMgr.SweepExpired(context.Background()) }, 30*time.Second, stopAwgSweep)

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
			})

			// Outbounds CRUD
			r.Route("/outbounds", func(r chi.Router) {
				r.Get("/", apiHandler.ListOutbounds)
				r.Post("/", apiHandler.CreateOutbound)
				r.Get("/{tag}", apiHandler.GetOutbound)
				r.Put("/{tag}", apiHandler.UpdateOutbound)
				r.Delete("/{tag}", apiHandler.DeleteOutbound)
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
				r.Delete("/peers/{publicKey}", apiHandler.DeleteAWGPeer)
				r.Get("/peers/{publicKey}/config", apiHandler.GetAWGPeerConfig)
				r.Get("/peers/{publicKey}/singbox", apiHandler.GetAWGPeerSingbox)
				r.Patch("/peers/{publicKey}/expiry", apiHandler.SetAWGPeerExpiry)
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

			// Config detection (kept for compatibility)
			r.Get("/config/detected", apiHandler.GetDetectedConfig)
			r.Post("/config/use-detected", apiHandler.UseDetectedConfig)

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
		sec := settingsMgr.Get().Security
		if !sec.AuthEnabled || sec.AuthPasswordHash == "" {
			pw, err := generatePassword()
			if err != nil {
				log.Fatalf("generate bootstrap password: %v", err)
			}
			if err := settingsMgr.Update(map[string]interface{}{
				"security.auth_enabled":  true,
				"security.auth_username": orDefault(sec.AuthUsername, "admin"),
				"security.auth_password": pw,
			}); err != nil {
				log.Fatalf("enable auth: %v", err)
			}
			if err := settingsMgr.Save(); err != nil {
				log.Fatalf("persist auth: %v", err)
			}
			pwFile := filepath.Join(filepath.Dir(settingsMgr.GetPath()), "routebox-initial-password")
			writeErr := os.WriteFile(pwFile, []byte(pw+"\n"), 0600)
			fmt.Println("==================================================================")
			fmt.Println(" VPS MODE: panel auth was OFF — generated admin credentials")
			fmt.Printf("   username: %s\n", orDefault(sec.AuthUsername, "admin"))
			fmt.Printf("   password: %s\n", pw)
			if writeErr == nil {
				fmt.Printf("   (also written to %s)\n", pwFile)
			} else {
				fmt.Printf("   (WARNING: could not write password file: %v)\n", writeErr)
			}
			fmt.Println("==================================================================")
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
		fmt.Println("Note: amnezia-box not installed - run setup wizard to configure")
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
		// HTTP-01 challenge listener on :80. TLS-ALPN-01 is impossible (LE would
		// connect on :443 = vless+Reality). :80 must stay open for renewals too.
		go func() {
			errCh <- fmt.Errorf("acme http-01 listener (:80): %w", http.ListenAndServe(":80", am.HTTPHandler(nil)))
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
	if trafficStore != nil {
		trafficStore.Close()
	}
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
			if c.Metadata.SourceIP != "" {
				mgr.Observe(c.Metadata.SourceIP, now)
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
